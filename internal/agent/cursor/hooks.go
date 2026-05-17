package cursor

import (
	"github.com/ujjalsharma100/lockie/internal/agent"
)

const hooksFileVersion = 1

// hookSpec links one canonical Lockie hook to one or more Cursor
// native hook keys. Cursor splits Claude's PostToolUse into success
// (`postToolUse`) and failure (`postToolUseFailure`) variants; both
// route to the same Lockie subcommand (§6.0 / §6.2).
type hookSpec struct {
	canonical  agent.HookType
	nativeKeys []string
	matcher    string
	command    string
}

var hookSpecs = []hookSpec{
	{
		canonical:  agent.HookSessionStart,
		nativeKeys: []string{"sessionStart"},
		command:    "lockie hook session-start",
	},
	{
		canonical:  agent.HookPromptSubmit,
		nativeKeys: []string{"beforeSubmitPrompt"},
		command:    "lockie hook prompt",
	},
	{
		canonical:  agent.HookPreToolUse,
		nativeKeys: []string{"preToolUse"},
		matcher:    ".*",
		command:    "lockie hook pre-tool",
	},
	{
		canonical:  agent.HookPostToolUse,
		nativeKeys: []string{"postToolUse", "postToolUseFailure"},
		matcher:    ".*",
		command:    "lockie hook post-tool",
	},
	{
		canonical:  agent.HookSessionStop,
		nativeKeys: []string{"sessionEnd"},
		command:    "lockie hook session-stop",
	},
}

// lockieEntry returns one Lockie-managed entry as map[string]any so the
// merger can mix it with user-authored entries that come back from the
// JSON parser as the same type.
func (s hookSpec) lockieEntry() map[string]any {
	entry := map[string]any{
		"_lockie_managed": true,
		"command":         s.command,
	}
	if s.matcher != "" {
		entry["matcher"] = s.matcher
	}
	return entry
}

// nativeKeysOwned returns every Cursor native hook key Lockie may
// touch. Used by the uninstaller and Status.
func nativeKeysOwned() []string {
	out := make([]string, 0)
	for _, s := range hookSpecs {
		out = append(out, s.nativeKeys...)
	}
	return out
}

// canonicalFor returns the canonical HookType for a Cursor native hook
// key. Empty HookType + false means Lockie does not own this key.
func canonicalFor(nativeKey string) (agent.HookType, bool) {
	for _, s := range hookSpecs {
		for _, k := range s.nativeKeys {
			if k == nativeKey {
				return s.canonical, true
			}
		}
	}
	return "", false
}

// buildLockiePlan returns the Lockie-managed hook entries for the
// enabled canonical hooks, keyed by Cursor native hook name. A single
// canonical HookPostToolUse fans out to both postToolUse and
// postToolUseFailure (IMPLEMENTATION.md §6.0).
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
		entry := s.lockieEntry()
		for _, key := range s.nativeKeys {
			out[key] = []any{entry}
		}
	}
	return out
}

// isLockieManaged reports whether an entry carries
// `"_lockie_managed": true`.
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
