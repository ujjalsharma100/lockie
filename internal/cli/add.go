package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/daemon"
)

func newAddCmd() *cobra.Command {
	var (
		project string
		socket  string
	)
	cmd := &cobra.Command{
		Use:   "add <name> <value>",
		Short: "Register a durable secret alias",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemonClient(cmd)
			if err != nil {
				return err
			}
			defer client.Close()
			ctx := cmdContext(cmd)
			r, err := client.AliasAdd(ctx, daemon.AliasAddParams{
				Project: project,
				Name:    args[0],
				Value:   args[1],
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s (value_id=%s", args[0], r.ValueID)
			if r.Deduped {
				fmt.Fprint(cmd.OutOrStdout(), ", deduped")
			}
			fmt.Fprintln(cmd.OutOrStdout(), ")")
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project scope (empty = user-global)")
	cmd.Flags().StringVar(&socket, "socket", "", "Override the daemon socket path")
	bindDaemonSocketFlag(cmd, &socket)
	return cmd
}
