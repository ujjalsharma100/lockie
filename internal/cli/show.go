package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/daemon"
)

func newShowCmd() *cobra.Command {
	var (
		project string
		socket  string
	)
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show alias metadata (never the literal value)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemonClient(cmd)
			if err != nil {
				return err
			}
			defer client.Close()
			a, err := client.AliasGet(cmdContext(cmd), daemon.AliasGetParams{
				Project: project,
				Name:    args[0],
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "name:        %s\n", a.Name)
			if a.Project != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "project:     %s\n", a.Project)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "value_id:    %s\n", a.ValueID)
			fmt.Fprintf(cmd.OutOrStdout(), "hash:        %s\n", a.Hash)
			fmt.Fprintf(cmd.OutOrStdout(), "created_at:  %s\n", a.CreatedAt)
			fmt.Fprintf(cmd.OutOrStdout(), "last_used:   %s\n", a.LastUsedAt)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project scope (empty = user-global)")
	cmd.Flags().StringVar(&socket, "socket", "", "Override the daemon socket path")
	bindDaemonSocketFlag(cmd, &socket)
	return cmd
}
