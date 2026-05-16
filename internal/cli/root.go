package cli

import "github.com/spf13/cobra"

// NewRoot returns the top-level lockie CLI command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "lockie",
		Short: "Secret management for AI coding agents",
	}
	root.AddCommand(newVersionCmd())
	return root
}
