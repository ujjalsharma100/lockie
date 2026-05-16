package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ujjalsharma100/lockie/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println(version.String())
			return nil
		},
	}
}
