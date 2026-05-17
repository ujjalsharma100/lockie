package claudecode

import (
	"github.com/ujjalsharma100/lockie/internal/agent"
)

// hookSpec describes one Lockie-managed hook entry as defined in
// IMPLEMENTATION.md §6.1. canonical maps to a Claude Code hook key,
// and the entry carries Lockie's matcher + priority metadata.
type hookSpec struct {
	canonical agent.HookType
	nativeKey string
	priority  string
	matcher   string
	command   string
}

// hookSpecs are listed in canonical priority order for readability;
// the on-disk JSON encodes its map keys alphabetically which is fine —
// Claude Code treats the dict as unordered.
var hookSpecs = []hookSpec{
	{
		canonical: agent.HookSessionStart,
		nativeKey: "SessionStart",
		command:   "lockie hook session-start",
	},
	{
		canonical: agent.HookPromptSubmit,
		nativeKey: "UserPromptSubmit",
		priority:  "first",
		command:   "lockie hook prompt",
	},
	{
		canonical: agent.HookPreToolUse,
		nativeKey: "PreToolUse",
		priority:  "last",
		matcher:   "Bash|Write|Edit|NotebookEdit",
		command:   "lockie hook pre-tool",
	},
	{
		canonical: agent.HookPostToolUse,
		nativeKey: "PostToolUse",
		priority:  "first",
		matcher:   "Read|Bash|Grep|Glob|Edit|Write|NotebookEdit|WebFetch",
		command:   "lockie hook post-tool",
	},
	{
		canonical: agent.HookSessionStop,
		nativeKey: "Stop",
		command:   "lockie hook session-stop",
	},
}

// lockieEntry is the Go representation of one Lockie-managed entry under
// a Claude Code hook key. It is constructed as map[string]any so the
// merge layer can mix it with user-authored entries that come back from
// the JSON parser as the same type.
func (s hookSpec) lockieEntry() map[string]any {
	entry := map[string]any{
		"_lockie_managed": true,
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": s.command,
			},
		},
	}
	if s.priority != "" {
		entry["_lockie_priority"] = s.priority
	}
	if s.matcher != "" {
		entry["matcher"] = s.matcher
	}
	return entry
}

// nativeKeysOwned returns the Claude Code hook keys Lockie may write to.
// Used by the uninstaller and Status to know which entries to inspect.
func nativeKeysOwned() []string {
	keys := make([]string, 0, len(hookSpecs))
	for _, s := range hookSpecs {
		keys = append(keys, s.nativeKey)
	}
	return keys
}

// canonicalFor returns the canonical HookType for a Claude Code native
// hook key. Empty HookType + false means Lockie does not own this key.
func canonicalFor(nativeKey string) (agent.HookType, bool) {
	for _, s := range hookSpecs {
		if s.nativeKey == nativeKey {
			return s.canonical, true
		}
	}
	return "", false
}

// buildLockiePlan returns the Lockie-managed hook entries for the
// enabled canonical hooks, keyed by Claude Code native hook name.
func buildLockiePlan(enabled []agent.HookType) map[string][]any {
	enabledSet := make(map[agent.HookType]bool, len(enabled))
	for _, h := range enabled {
		enabledSet[h] = true
	}
	out := map[string][]any{}
	for _, s := range hookSpecs {
		if !enabledSet[s.canonical] {
			continue
		}
		out[s.nativeKey] = []any{s.lockieEntry()}
	}
	return out
}

// isLockieManaged reports whether an entry — as parsed from JSON or
// freshly built by buildLockiePlan — carries `"_lockie_managed": true`.
// Anything else (user-authored entries, malformed data, etc.) is left
// untouched by install/uninstall.
func isLockieManaged(entry any) bool {
	m, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	v, ok := m["_lockie_managed"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
