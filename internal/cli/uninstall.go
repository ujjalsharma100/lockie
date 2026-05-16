package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

func newUninstallCmd() *cobra.Command {
	var scopeFlag string
	cmd := &cobra.Command{
		Use:   "uninstall <agent>",
		Short: "Remove Lockie hooks from a coding agent's config",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
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
			return a.Uninstall(scope)
		},
	}
	cmd.Flags().StringVar(&scopeFlag, "scope", "user", "Config scope: user|project|project-local")
	return cmd
}
