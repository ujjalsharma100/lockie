package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ujjalsharma100/lockie/internal/store"
)

// DefaultDialTimeout caps a single Dial attempt. The hook command
// path needs to fail fast when the daemon socket is unreachable so
// the launcher's auto-start can take over.
const DefaultDialTimeout = 500 * time.Millisecond

// DefaultCallTimeout bounds a single Call's request → response
// round-trip. Hooks are on the agent's critical path; > a second is
// already too slow.
const DefaultCallTimeout = 2 * time.Second

// Client is a one-connection-per-instance daemon client. It is safe
// for sequential use from a single goroutine; the hook CLI hands a
// fresh Client off per process so concurrent use is not a goal.
// Tests share a single Client across goroutines via a mutex — see
// `mu` below.
type Client struct {
	socketPath string

	mu   sync.Mutex
	conn net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer

	dialTimeout time.Duration
	callTimeout time.Duration

	newRequestID func() (string, error)
}

// NewClient constructs a Client bound to the given socket path. The
// connection is lazy — Dial happens on the first Call so callers
// that only want to *check* whether the daemon is alive can use
// CheckHealth and not pay the cost of a connection up-front.
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath:  socketPath,
		dialTimeout: DefaultDialTimeout,
		callTimeout: DefaultCallTimeout,
		newRequestID: func() (string, error) {
			id, err := store.NewValueID()
			return string(id), err
		},
	}
}

// SetDialTimeout / SetCallTimeout adjust the timeouts; tests use them
// to keep the suite fast.
func (c *Client) SetDialTimeout(d time.Duration) { c.dialTimeout = d }
func (c *Client) SetCallTimeout(d time.Duration) { c.callTimeout = d }

// Close releases the underlying connection. Safe to call multiple times.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	c.br = nil
	c.bw = nil
	return err
}

// Call performs one request/response round-trip. It marshals params,
// sends the framed request, reads one response line, and unmarshals
// the result into out. A protocol-level error in the response is
// returned as a typed *ErrorObject so callers can branch on code.
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureConn(ctx); err != nil {
		return err
	}
	id, err := c.newRequestID()
	if err != nil {
		return fmt.Errorf("daemon client: mint request id: %w", err)
	}
	req := Request{ID: id, Method: method}
	if params != nil {
		body, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("daemon client: encode params: %w", err)
		}
		req.Params = body
	}
	body, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("daemon client: encode request: %w", err)
	}
	body = append(body, '\n')

	deadline, ok := callDeadline(ctx, c.callTimeout)
	if ok {
		_ = c.conn.SetDeadline(deadline)
		defer c.conn.SetDeadline(time.Time{}) //nolint:errcheck
	}
	if _, err := c.bw.Write(body); err != nil {
		c.dropConn()
		return fmt.Errorf("daemon client: write request: %w", err)
	}
	if err := c.bw.Flush(); err != nil {
		c.dropConn()
		return fmt.Errorf("daemon client: flush request: %w", err)
	}
	line, err := readLineNoLimit(c.br)
	if err != nil {
		c.dropConn()
		return fmt.Errorf("daemon client: read response: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		c.dropConn()
		return fmt.Errorf("daemon client: decode response: %w", err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out == nil || len(resp.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		return fmt.Errorf("daemon client: decode result: %w", err)
	}
	return nil
}

func (c *Client) ensureConn(ctx context.Context) error {
	if c.conn != nil {
		return nil
	}
	d := net.Dialer{Timeout: c.dialTimeout}
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("daemon client: dial %s: %w", c.socketPath, err)
	}
	c.conn = conn
	c.br = bufio.NewReaderSize(conn, 64<<10)
	c.bw = bufio.NewWriterSize(conn, 64<<10)
	return nil
}

func (c *Client) dropConn() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.br = nil
		c.bw = nil
	}
}

func callDeadline(ctx context.Context, fallback time.Duration) (time.Time, bool) {
	if dl, ok := ctx.Deadline(); ok {
		return dl, true
	}
	if fallback <= 0 {
		return time.Time{}, false
	}
	return time.Now().Add(fallback), true
}

// readLineNoLimit reads one '\n'-terminated frame. Unlike the
// server-side readLine, the client trusts the daemon and skips the
// per-frame ceiling.
func readLineNoLimit(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) && len(buf)+len(chunk) > 0 {
				return append(buf, chunk...), nil
			}
			return nil, err
		}
		buf = append(buf, chunk...)
		if !isPrefix {
			return buf, nil
		}
	}
}

// ---- typed wrappers --------------------------------------------------------

// Health calls the `health` method and returns the parsed result.
func (c *Client) Health(ctx context.Context) (*HealthResult, error) {
	var out HealthResult
	if err := c.Call(ctx, MethodHealth, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SessionStart opens (or registers) a session. If sessionID is empty
// the daemon mints one and returns it.
func (c *Client) SessionStart(ctx context.Context, p SessionStartParams) (*SessionStartResult, error) {
	var out SessionStartResult
	if err := c.Call(ctx, MethodSessionStart, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SessionStop drops a session's placeholder map.
func (c *Client) SessionStop(ctx context.Context, p SessionStopParams) error {
	return c.Call(ctx, MethodSessionStop, p, nil)
}

// HookPrompt asks the daemon to redact a user prompt.
func (c *Client) HookPrompt(ctx context.Context, p HookPromptParams) (*HookPromptResult, error) {
	var out HookPromptResult
	if err := c.Call(ctx, MethodHookPrompt, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HookPreTool asks the daemon to rehydrate placeholders in tool input.
func (c *Client) HookPreTool(ctx context.Context, p HookPreToolParams) (*HookPreToolResult, error) {
	var out HookPreToolResult
	if err := c.Call(ctx, MethodHookPreTool, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HookPostTool asks the daemon to redact secrets in tool output.
func (c *Client) HookPostTool(ctx context.Context, p HookPostToolParams) (*HookPostToolResult, error) {
	var out HookPostToolResult
	if err := c.Call(ctx, MethodHookPostTool, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AliasAdd registers a durable alias via the daemon store.
func (c *Client) AliasAdd(ctx context.Context, p AliasAddParams) (*AliasAddResult, error) {
	var out AliasAddResult
	if err := c.Call(ctx, MethodAliasAdd, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AliasList returns alias metadata for a project scope.
func (c *Client) AliasList(ctx context.Context, p AliasListParams) (*AliasListResult, error) {
	var out AliasListResult
	if err := c.Call(ctx, MethodAliasList, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AliasGet returns metadata for one alias.
func (c *Client) AliasGet(ctx context.Context, p AliasGetParams) (*AliasInfo, error) {
	var out AliasInfo
	if err := c.Call(ctx, MethodAliasGet, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AliasForget removes an alias and GCs the value when unreferenced.
func (c *Client) AliasForget(ctx context.Context, p AliasForgetParams) error {
	return c.Call(ctx, MethodAliasForget, p, nil)
}
