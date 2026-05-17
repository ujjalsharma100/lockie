package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/daemon"
	"github.com/ujjalsharma100/lockie/internal/store/memory"
)

// newDaemonCmd builds the `lockie daemon` command group:
//
//	lockie daemon start [--foreground] [--socket PATH]
//	lockie daemon stop  [--socket PATH]
//	lockie daemon status [--socket PATH]
//
// Phase 1 ships an in-memory store as the daemon's backing — the
// keychain swap-in is Phase 2 (§9.1) and won't change this wiring
// because the Store interface is the swap boundary.
func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the Lockie background daemon",
	}
	cmd.AddCommand(newDaemonStartCmd(), newDaemonStopCmd(), newDaemonStatusCmd())
	return cmd
}

func newDaemonStartCmd() *cobra.Command {
	var (
		foreground bool
		socket     string
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon (foreground by default)",
		Long: "Start the Lockie daemon. With --foreground the process stays attached\n" +
			"to the current terminal; without it, the daemon detaches itself.\n\n" +
			"`lockie hook ...` invocations auto-start the daemon when needed, so most\n" +
			"users will not call this directly.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketPath, err := resolveSocket(socket)
			if err != nil {
				return err
			}
			if foreground {
				return runDaemonForeground(cmd, socketPath)
			}
			return runDaemonDetached(cmd, socketPath)
		},
	}
	cmd.Flags().BoolVar(&foreground, "foreground", false, "Stay attached to this terminal; otherwise fork a detached daemon")
	cmd.Flags().StringVar(&socket, "socket", "", "Override the daemon socket path")
	return cmd
}

func newDaemonStopCmd() *cobra.Command {
	var socket string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketPath, err := resolveSocket(socket)
			if err != nil {
				return err
			}
			return stopDaemon(cmd, socketPath)
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Override the daemon socket path")
	return cmd
}

func newDaemonStatusCmd() *cobra.Command {
	var socket string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon uptime and active session count",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			socketPath, err := resolveSocket(socket)
			if err != nil {
				return err
			}
			return printDaemonStatus(cmd, socketPath)
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Override the daemon socket path")
	return cmd
}

func resolveSocket(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return daemon.DefaultSocketPath()
}

func runDaemonForeground(cmd *cobra.Command, socketPath string) error {
	st := memory.New()
	h, err := daemon.NewHandler(st)
	if err != nil {
		return err
	}
	srv := daemon.NewServer(socketPath, h)
	if err := srv.Start(); err != nil {
		if errors.Is(err, daemon.ErrAlreadyRunning) {
			// Already running — treat as success so `lockie daemon
			// start` is idempotent (matches the launcher's
			// EnsureRunning contract).
			fmt.Fprintf(cmd.OutOrStdout(), "lockie daemon already running at %s\n", socketPath)
			return nil
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "lockie daemon listening on %s\n", socketPath)

	// Block until a termination signal. SIGTERM is the standard, SIGINT
	// covers Ctrl-C in foreground mode; both trigger a graceful shutdown.
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	<-stopCh
	signal.Stop(stopCh)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Stop(ctx)
}

func runDaemonDetached(cmd *cobra.Command, socketPath string) error {
	opts := daemon.DefaultLaunchOptions(socketPath)
	// Propagate the resolved socket path to the child so an override
	// here (typically a test) survives the fork. Without this the
	// child would re-resolve via DefaultSocketPath and bind a
	// different socket, leaving the parent's wait loop to time out.
	opts.Args = []string{"daemon", "start", "--foreground", "--socket", socketPath}
	opts.WaitTimeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(cmdContext(cmd), 5*time.Second)
	defer cancel()
	client, err := daemon.EnsureRunning(ctx, opts)
	if err != nil {
		return err
	}
	defer client.Close()
	fmt.Fprintf(cmd.OutOrStdout(), "lockie daemon listening on %s\n", socketPath)
	return nil
}

// cmdContext returns the cobra command's context, falling back to
// context.Background() when none has been attached (the case under
// `Execute()` without an explicit ExecuteContext).
func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func stopDaemon(cmd *cobra.Command, socketPath string) error {
	client := daemon.NewClient(socketPath)
	defer client.Close()
	ctx, cancel := context.WithTimeout(cmdContext(cmd), 1*time.Second)
	defer cancel()
	health, err := client.Health(ctx)
	if err != nil {
		// Daemon not reachable — nothing to stop. Treat as success
		// so a stop after a crash doesn't fail the script.
		fmt.Fprintf(cmd.OutOrStdout(), "no lockie daemon reachable at %s\n", socketPath)
		return nil
	}
	if health.PID > 0 {
		// Foreground daemon owns its socket; sending SIGTERM lets it
		// run its graceful Stop(). The launcher's detached path
		// keeps the same PID so this works for both.
		if err := signalPID(health.PID, syscall.SIGTERM); err != nil {
			return fmt.Errorf("signal daemon pid %d: %w", health.PID, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "sent SIGTERM to lockie daemon pid %d\n", health.PID)
		return nil
	}
	return fmt.Errorf("daemon reported no pid; cannot stop")
}

func printDaemonStatus(cmd *cobra.Command, socketPath string) error {
	client := daemon.NewClient(socketPath)
	defer client.Close()
	ctx, cancel := context.WithTimeout(cmdContext(cmd), 1*time.Second)
	defer cancel()
	health, err := client.Health(ctx)
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "lockie daemon: not running (socket: %s)\n", socketPath)
		return nil
	}
	uptime := time.Duration(health.UptimeSeconds) * time.Second
	fmt.Fprintf(cmd.OutOrStdout(),
		"lockie daemon: running\n  socket:   %s\n  version:  %s\n  pid:      %d\n  uptime:   %s\n  sessions: %d\n",
		socketPath, health.Version, health.PID, uptime, health.Sessions,
	)
	return nil
}

func signalPID(pid int, sig os.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}
