package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SocketDirEnv lets tests and one-off scripts override the directory
// the daemon listens in. When set, it replaces the entire $XDG /
// $TMPDIR resolution chain.
const SocketDirEnv = "LOCKIE_SOCKET_DIR"

// DefaultSocketPath returns the canonical Unix-domain socket path for
// the current platform, per IMPLEMENTATION.md §4.1:
//
//   - Linux:   $XDG_RUNTIME_DIR/lockie/lockied.sock (falls back to $TMPDIR)
//   - macOS:   $TMPDIR/lockie/lockied.sock
//
// Phase 1 is Unix-only; Windows named-pipe support is tracked alongside
// the Phase 1 dogfooding exit criterion (§8.10).
//
// $LOCKIE_SOCKET_DIR (see SocketDirEnv) takes precedence on every
// platform — tests use it to point at a t.TempDir(), avoiding the
// 104-byte sun_path ceiling on macOS.
func DefaultSocketPath() (string, error) {
	dir, err := defaultSocketDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lockied.sock"), nil
}

func defaultSocketDir() (string, error) {
	if override := os.Getenv(SocketDirEnv); override != "" {
		return override, nil
	}
	switch runtime.GOOS {
	case "linux":
		if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
			return filepath.Join(xdg, "lockie"), nil
		}
		// XDG_RUNTIME_DIR is unset in CI containers and on headless
		// systems — fall through to TMPDIR so the daemon still has
		// somewhere to bind. Permissions on the bind dir are tightened
		// in ensureSocketDir below.
		return filepath.Join(os.TempDir(), "lockie"), nil
	case "darwin":
		return filepath.Join(os.TempDir(), "lockie"), nil
	default:
		return "", fmt.Errorf("daemon: unsupported platform %q (Phase 1 supports linux/darwin)", runtime.GOOS)
	}
}

// ensureSocketDir mkdirs the parent directory of socketPath with 0o700
// perms. Returning a typed error here keeps the server-startup path
// readable.
func ensureSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("daemon: create socket dir %s: %w", dir, err)
	}
	// MkdirAll skips chmod when the directory already exists. Tighten
	// the perms in case it was created earlier with a different umask.
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("daemon: chmod socket dir %s: %w", dir, err)
	}
	return nil
}
