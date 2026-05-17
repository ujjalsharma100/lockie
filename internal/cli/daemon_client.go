package cli

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/daemon"
)

type daemonSocketKey struct{}

func daemonSocket(cmd *cobra.Command) string {
	if v, ok := cmd.Context().Value(daemonSocketKey{}).(string); ok && v != "" {
		return v
	}
	path, _ := resolveSocket("")
	return path
}

func daemonClient(cmd *cobra.Command) (*daemon.Client, error) {
	socket := daemonSocket(cmd)
	opts := daemon.DefaultLaunchOptions(socket)
	opts.Binary = os.Args[0]
	opts.Args = []string{"daemon", "start", "--foreground", "--socket", socket}
	ctx, cancel := context.WithTimeout(cmdContext(cmd), 5*time.Second)
	defer cancel()
	return daemon.EnsureRunning(ctx, opts)
}

func bindDaemonSocketFlag(cmd *cobra.Command, socket *string) {
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		path, err := resolveSocket(*socket)
		if err != nil {
			return err
		}
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		cmd.SetContext(context.WithValue(ctx, daemonSocketKey{}, path))
		return nil
	}
}
