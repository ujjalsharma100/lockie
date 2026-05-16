package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

func newInstallCmd() *cobra.Command {
	var (
		scopeFlag string
		dryRun    bool
	)
	cmd := &cobra.Command{
		Use:   "install <agent>",
		Short: "Wire Lockie hooks into a coding agent's config",
		Long: "Install Lockie's hook entries into the named agent's config file.\n\n" +
			"Available agents: " + listAgentNames() + ".\n" +
			"Use --dry-run to preview the JSON that would be written without touching disk.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := parseScope(scopeFlag)
			if err != nil {
				return err
			}
			a, err := agent.Get(args[0])
			if err != nil {
				if errors.Is(err, agent.ErrUnknownAgent) {
					return fmt.Errorf("%w (available: %s)", err, listAgentNames())
				}
				return err
			}
			opts := agent.InstallOptions{
				Scope:        scope,
				DryRun:       dryRun,
				DryRunOutput: cmd.OutOrStdout(),
			}
			return a.Install(opts)
		},
	}
	cmd.Flags().StringVar(&scopeFlag, "scope", "user", "Config scope: user|project|project-local")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the JSON that would be written; do not touch disk")
	return cmd
}

func listAgentNames() string {
	names := agent.Names()
	if len(names) == 0 {
		return "(none registered)"
	}
	out := names[0]
	for _, n := range names[1:] {
		out += ", " + n
	}
	return out
}
