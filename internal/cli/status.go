package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

func newStatusCmd() *cobra.Command {
	var scopeFlag string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show installed agents, hook coverage, and detected config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, err := parseScope(scopeFlag)
			if err != nil {
				return err
			}
			return writeStatus(cmd.OutOrStdout(), scope)
		},
	}
	cmd.Flags().StringVar(&scopeFlag, "scope", "user", "Config scope to inspect: user|project|project-local")
	return cmd
}

func writeStatus(w io.Writer, scope agent.Scope) error {
	agents := agent.All()
	if len(agents) == 0 {
		_, err := fmt.Fprintln(w, "no agents registered")
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tDETECTED\tCONFIG DIR\tSETTINGS PATH\tWARNINGS")
	for _, a := range agents {
		det, err := a.Detect()
		if err != nil {
			fmt.Fprintf(tw, "%s\terror\t-\t-\t%v\n", a.Name(), err)
			continue
		}
		st, err := a.Status(scope)
		if err != nil {
			fmt.Fprintf(tw, "%s\t%v\t%s\t-\t%v\n", a.Name(), det.Installed, det.ConfigDir, err)
			continue
		}
		warn := "-"
		if len(st.Warnings) > 0 {
			warn = st.Warnings[0]
		}
		fmt.Fprintf(tw, "%s\t%v\t%s\t%s\t%s\n",
			a.Name(), det.Installed, fallback(det.ConfigDir, "-"),
			fallback(st.SettingsPath, "-"), warn,
		)
	}
	return tw.Flush()
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
