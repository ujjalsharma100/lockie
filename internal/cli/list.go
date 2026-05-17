package cli

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/daemon"
)

func newListCmd() *cobra.Command {
	var (
		project string
		socket  string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List durable secret aliases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := daemonClient(cmd)
			if err != nil {
				return err
			}
			defer client.Close()
			r, err := client.AliasList(cmdContext(cmd), daemon.AliasListParams{Project: project})
			if err != nil {
				return err
			}
			aliases := r.Aliases
			sort.Slice(aliases, func(i, j int) bool {
				return aliases[i].Name < aliases[j].Name
			})
			if len(aliases) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no aliases")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tVALUE_ID\tCREATED")
			for _, a := range aliases {
				fmt.Fprintf(w, "%s\t%s\t%s\n", a.Name, a.ValueID, a.CreatedAt)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project scope (empty = user-global)")
	cmd.Flags().StringVar(&socket, "socket", "", "Override the daemon socket path")
	bindDaemonSocketFlag(cmd, &socket)
	return cmd
}
