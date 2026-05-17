package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
)

// Server is a Lockie daemon listening on a Unix domain socket. One
// goroutine accepts connections, one goroutine per accepted connection
// drives the JSON-lines request/response loop. Shutdown is cooperative:
// Stop closes the listener (unblocking Accept), waits for in-flight
// connections to drain, then unlinks the socket file.
type Server struct {
	socketPath string
	handler    *Handler
	listener   net.Listener

	wg sync.WaitGroup

	// closed is set once Stop has been called; subsequent Accept
	// errors caused by the close are then suppressed.
	closed atomic.Bool

	// conns tracks live connections so Stop can break their read
	// loops by closing them. Read-side bufio buffers are flushed
	// implicitly when the underlying conn is closed.
	connsMu sync.Mutex
	conns   map[net.Conn]struct{}

	// maxLineBytes caps the per-frame line length to avoid an
	// attacker (or buggy client) OOM-ing the daemon with a single
	// unterminated 1 GB write. Default 4 MiB — comfortably above any
	// tool-output payload Lockie will plausibly redact.
	maxLineBytes int
}

// DefaultMaxLineBytes is the per-frame line ceiling applied to every
// connection unless Server.MaxLineBytes is set before Start.
const DefaultMaxLineBytes = 4 << 20 // 4 MiB

// NewServer wires a handler to a socket path. The socket file is not
// created until Start runs.
func NewServer(socketPath string, h *Handler) *Server {
	return &Server{
		socketPath:   socketPath,
		handler:      h,
		conns:        make(map[net.Conn]struct{}),
		maxLineBytes: DefaultMaxLineBytes,
	}
}

// SetMaxLineBytes overrides the per-frame line ceiling. Call before Start.
func (s *Server) SetMaxLineBytes(n int) {
	if n > 0 {
		s.maxLineBytes = n
	}
}

// SocketPath returns the path the server is bound (or will bind) to.
func (s *Server) SocketPath() string { return s.socketPath }

// Start opens the listener and spawns the accept loop. It returns once
// the socket is bound (so callers can race-free dial it). Use Stop to
// shut down.
//
// If a stale socket file exists at the path, Start removes it. Race
// against a live daemon is handled by trying to dial it first — if a
// connection succeeds the existing daemon is treated as authoritative
// and Start returns ErrAlreadyRunning.
func (s *Server) Start() error {
	if s.handler == nil {
		return fmt.Errorf("daemon: Start requires a Handler")
	}
	if err := ensureSocketDir(s.socketPath); err != nil {
		return err
	}
	if err := removeStaleSocket(s.socketPath); err != nil {
		return err
	}
	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("daemon: listen on %s: %w", s.socketPath, err)
	}
	// Tighten perms so other users on the box can't talk to the
	// daemon. Unix sockets respect filesystem perms on Linux; macOS
	// has historically been loose here, but a 0o600 chmod is still
	// the right hardening default.
	if err := os.Chmod(s.socketPath, 0o600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("daemon: chmod socket %s: %w", s.socketPath, err)
	}
	s.listener = ln
	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}

// ErrAlreadyRunning is returned by removeStaleSocket when a live
// daemon already owns the socket path.
var ErrAlreadyRunning = errors.New("daemon: another lockie daemon is already running")

func removeStaleSocket(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("daemon: stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("daemon: %s exists and is not a socket", path)
	}
	// Try to talk to it; if the dial succeeds something is alive.
	conn, dialErr := net.Dial("unix", path)
	if dialErr == nil {
		_ = conn.Close()
		return ErrAlreadyRunning
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("daemon: remove stale socket %s: %w", path, err)
	}
	return nil
}

// Stop closes the listener, waits for all in-flight connections to
// finish, and removes the socket file. Safe to call from any goroutine
// and idempotent.
func (s *Server) Stop(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.connsMu.Lock()
	for c := range s.conns {
		_ = c.Close()
	}
	s.connsMu.Unlock()

	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		// Best-effort: the accept loop and per-conn handlers may
		// linger for a few ms past the context deadline, but the
		// socket file is already removed below so new clients can't
		// arrive.
	}
	if s.socketPath != "" {
		_ = os.Remove(s.socketPath)
	}
	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			// Transient accept errors (e.g. EMFILE) — log via
			// stderr; production code may upgrade to a structured
			// logger later, but Phase 1 doesn't have one wired yet.
			fmt.Fprintf(os.Stderr, "lockie daemon: accept: %v\n", err)
			return
		}
		s.trackConn(conn)
		s.wg.Add(1)
		go s.serveConn(conn)
	}
}

func (s *Server) trackConn(c net.Conn) {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	s.conns[c] = struct{}{}
}

func (s *Server) untrackConn(c net.Conn) {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	delete(s.conns, c)
}

func (s *Server) serveConn(c net.Conn) {
	defer s.wg.Done()
	defer s.untrackConn(c)
	defer c.Close()

	br := bufio.NewReaderSize(c, 64<<10)
	bw := bufio.NewWriterSize(c, 64<<10)
	for {
		line, err := readLine(br, s.maxLineBytes)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			if s.closed.Load() {
				return
			}
			// Surface a final protocol-level error to the client
			// before disconnecting, so a single garbled frame
			// doesn't look like an opaque connection drop.
			_ = writeProtocolError(bw, "", ErrCodeInvalidRequest, err.Error())
			return
		}
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			if werr := writeProtocolError(bw, "", ErrCodeInvalidRequest,
				fmt.Sprintf("decode request: %v", err)); werr != nil {
				return
			}
			continue
		}
		resp := s.handler.Dispatch(&req)
		if err := writeResponse(bw, resp); err != nil {
			return
		}
	}
}

func readLine(br *bufio.Reader, maxBytes int) ([]byte, error) {
	var buf []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		if err != nil {
			return nil, err
		}
		if buf == nil && !isPrefix {
			// Hot path: most frames fit in the bufio buffer.
			return append([]byte(nil), chunk...), nil
		}
		if len(buf)+len(chunk) > maxBytes {
			// Drain the rest of the line so the connection
			// resyncs on the next newline rather than wedging.
			for isPrefix {
				_, isPrefix, _ = br.ReadLine()
			}
			return nil, fmt.Errorf("request exceeds max line size (%d bytes)", maxBytes)
		}
		buf = append(buf, chunk...)
		if !isPrefix {
			return buf, nil
		}
	}
}

func writeResponse(bw *bufio.Writer, resp *Response) error {
	body, err := json.Marshal(resp)
	if err != nil {
		// Marshalling a Response we built ourselves shouldn't fail;
		// if it somehow does, fall back to a fixed error frame so
		// the client doesn't hang waiting for bytes.
		body = []byte(`{"error":{"code":5,"message":"encode response failed"}}`)
	}
	body = append(body, '\n')
	if _, err := bw.Write(body); err != nil {
		return err
	}
	return bw.Flush()
}

func writeProtocolError(bw *bufio.Writer, id string, code int, msg string) error {
	resp := &Response{ID: id, Error: &ErrorObject{Code: code, Message: msg}}
	return writeResponse(bw, resp)
}
