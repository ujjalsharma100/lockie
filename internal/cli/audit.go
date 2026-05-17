package cli

import (
	"fmt"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/audit"
	"github.com/ujjalsharma100/lockie/internal/config"
)

func newAuditCmd() *cobra.Command {
	var (
		since string
		name  string
	)
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Read the substitution audit log",
		Long:  "Show redaction history from ~/.lockie/audit.log. Entries list placeholder names and rules only — never literals.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.AuditPath()
			if err != nil {
				return err
			}
			var filter audit.Filter
			if since != "" {
				t, err := parseSince(since)
				if err != nil {
					return fmt.Errorf("audit: --since: %w", err)
				}
				filter.Since = t
			}
			filter.Name = name
			events, err := audit.Read(path, filter)
			if err != nil {
				return err
			}
			if len(events) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no audit events")
				return nil
			}
			sort.Slice(events, func(i, j int) bool {
				return events[i].Timestamp.Before(events[j].Timestamp)
			})
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TIME\tSESSION\tTOOL\tRULE\tPLACEHOLDER")
			for _, ev := range events {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					ev.Timestamp.UTC().Format(time.RFC3339),
					ev.SessionID,
					ev.Tool,
					ev.RuleID,
					ev.Placeholder,
				)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only events at or after this time (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&name, "name", "", "Filter by exact placeholder name")
	return cmd
}

func parseSince(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 or YYYY-MM-DD, got %q", s)
}
