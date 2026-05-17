package cli

import (
	"github.com/spf13/cobra"

	// Side-effect imports: each adapter registers itself in
	// internal/agent's global registry via init().
	_ "github.com/ujjalsharma100/lockie/internal/agent/claudecode"
	_ "github.com/ujjalsharma100/lockie/internal/agent/cursor"
)

// NewRoot returns the top-level lockie CLI command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "lockie",
		Short: "Secret management for AI coding agents",
	}
	root.AddCommand(
		newVersionCmd(),
		newInstallCmd(),
		newUninstallCmd(),
		newStatusCmd(),
		newDaemonCmd(),
	)
	return root
}
