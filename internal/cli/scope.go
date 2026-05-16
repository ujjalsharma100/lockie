package cli

import (
	"fmt"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

// parseScope maps the --scope CLI flag value onto the agent.Scope
// enum. Returns a usage error for any unrecognized value so cobra
// surfaces the help text.
func parseScope(s string) (agent.Scope, error) {
	switch s {
	case "user", "":
		return agent.ScopeUser, nil
	case "project":
		return agent.ScopeProject, nil
	case "project-local":
		return agent.ScopeProjectLocal, nil
	default:
		return 0, fmt.Errorf("invalid --scope %q (want user|project|project-local)", s)
	}
}
