package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/daemon"
)

func newForgetCmd() *cobra.Command {
	var (
		project string
		socket  string
	)
	cmd := &cobra.Command{
		Use:   "forget <name>",
		Short: "Remove a durable alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemonClient(cmd)
			if err != nil {
				return err
			}
			defer client.Close()
			if err := client.AliasForget(cmdContext(cmd), daemon.AliasForgetParams{
				Project: project,
				Name:    args[0],
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "forgot %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project scope (empty = user-global)")
	cmd.Flags().StringVar(&socket, "socket", "", "Override the daemon socket path")
	bindDaemonSocketFlag(cmd, &socket)
	return cmd
}
