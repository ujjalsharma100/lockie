package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// LaunchOptions controls how the launcher boots a background daemon.
// Defaults are aimed at the `lockie hook ...` call site, where the
// hook command must talk to a live daemon within a tight latency
// budget; tests override the binary path and timing to keep
// integration runs fast.
type LaunchOptions struct {
	// SocketPath — where to look for / wait on the socket.
	SocketPath string
	// Binary — the lockie executable used to fork a daemon when the
	// socket is absent. Defaults to os.Args[0].
	Binary string
	// Args — argv passed to Binary. Defaults to ["daemon", "start", "--foreground"].
	Args []string
	// Env — process environment. Defaults to os.Environ().
	Env []string
	// WaitTimeout — total time to wait for the socket to come up.
	WaitTimeout time.Duration
	// PollInterval — how often to poll for the socket while waiting.
	PollInterval time.Duration
}

// DefaultLaunchOptions returns the standard launcher settings.
func DefaultLaunchOptions(socketPath string) LaunchOptions {
	return LaunchOptions{
		SocketPath:   socketPath,
		Binary:       os.Args[0],
		Args:         []string{"daemon", "start", "--foreground"},
		Env:          os.Environ(),
		WaitTimeout:  500 * time.Millisecond,
		PollInterval: 20 * time.Millisecond,
	}
}

// EnsureRunning makes sure a daemon is reachable at opts.SocketPath.
// If the socket already accepts connections, it returns immediately.
// Otherwise it forks a background daemon process (detached from the
// caller) and polls until the socket is reachable or opts.WaitTimeout
// expires. The returned client is connected and ready for Call.
func EnsureRunning(ctx context.Context, opts LaunchOptions) (*Client, error) {
	if opts.SocketPath == "" {
		return nil, fmt.Errorf("daemon launcher: SocketPath is required")
	}
	// Fast path: socket already alive.
	if c, err := tryDial(opts.SocketPath); err == nil {
		_ = c.Close()
		return clientReady(opts.SocketPath), nil
	}
	if err := spawnDaemon(opts); err != nil {
		return nil, err
	}
	if err := waitForSocket(ctx, opts); err != nil {
		return nil, err
	}
	return clientReady(opts.SocketPath), nil
}

func clientReady(socketPath string) *Client { return NewClient(socketPath) }

func tryDial(socketPath string) (net.Conn, error) {
	return net.DialTimeout("unix", socketPath, 100*time.Millisecond)
}

func spawnDaemon(opts LaunchOptions) error {
	if opts.Binary == "" {
		return fmt.Errorf("daemon launcher: Binary is required")
	}
	args := opts.Args
	if len(args) == 0 {
		args = []string{"daemon", "start", "--foreground"}
	}
	// Both Binary and Args are constructed by the lockie CLI itself
	// (DefaultLaunchOptions) or by callers inside our own packages —
	// not by untrusted user input. gosec G204 fires on the *shape*
	// of exec.Command with variables, so suppress with cause.
	cmd := exec.Command(opts.Binary, args...) //nolint:gosec // args are internal, not user-tainted
	if opts.Env != nil {
		cmd.Env = opts.Env
	}
	// Detach from the calling terminal: redirect stdio to /dev/null
	// so the daemon doesn't hold the hook command's tty open. We
	// intentionally do not pipe stdout/stderr — daemon logs land on
	// the system log via the foreground process's own writes when
	// step 8.9 wires the audit log, not via the hook invocation.
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("daemon launcher: open /dev/null: %w", err)
	}
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	if err := cmd.Start(); err != nil {
		_ = devNull.Close()
		return fmt.Errorf("daemon launcher: start %s: %w", opts.Binary, err)
	}
	// Release file handles in the parent — the child has its own.
	_ = devNull.Close()
	// Reap the child if it dies fast (e.g. ErrAlreadyRunning during a
	// race). Doing this in a goroutine keeps EnsureRunning's hot path
	// non-blocking when the daemon stays up, which is the common case.
	go func() { _ = cmd.Process.Release() }()
	return nil
}

func waitForSocket(ctx context.Context, opts LaunchOptions) error {
	deadline := time.Now().Add(opts.WaitTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	poll := opts.PollInterval
	if poll <= 0 {
		poll = 20 * time.Millisecond
	}
	for {
		c, err := tryDial(opts.SocketPath)
		if err == nil {
			_ = c.Close()
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) && !isConnRefused(err) {
			// Surface anything that isn't "socket not there yet";
			// the caller probably wants to know about EPERM.
			return fmt.Errorf("daemon launcher: dial: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("daemon launcher: socket %s did not come up within %s",
				opts.SocketPath, opts.WaitTimeout)
		}
		select {
		case <-time.After(poll):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func isConnRefused(err error) bool {
	if err == nil {
		return false
	}
	// On macOS / Linux the dial against a stale or missing socket
	// typically surfaces as syscall.ECONNREFUSED or syscall.ENOENT.
	// Match by string to avoid pulling in syscall just for this
	// one check on platforms where the error tree shape varies.
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such file or directory")
}
