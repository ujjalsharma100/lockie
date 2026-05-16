package cursor

import (
	"github.com/ujjalsharma100/lockie/internal/agent"
)

// hookEntry is one Lockie-managed entry under a Cursor hook key.
// Field order matches the wire format documented in
// IMPLEMENTATION.md §6.2 (command, matcher, _lockie_managed); the
// json:"omitempty" on matcher drops it for hooks that do not need one
// (e.g. sessionStart).
type hookEntry struct {
	Command       string `json:"command"`
	Matcher       string `json:"matcher,omitempty"`
	LockieManaged bool   `json:"_lockie_managed"`
}

// hooksFile is the on-disk shape of ~/.cursor/hooks.json.
type hooksFile struct {
	Version int                    `json:"version"`
	Hooks   map[string][]hookEntry `json:"hooks"`
}

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

// buildHooks produces the Lockie-managed slice of hooks.json covering
// exactly the hooks in `enabled`. The real install path (step 8.3b)
// will merge this into the user's existing file; for dry-run we emit
// the standalone slice so callers can preview it.
func buildHooks(enabled []agent.HookType) hooksFile {
	enabledSet := make(map[agent.HookType]bool, len(enabled))
	for _, h := range enabled {
		enabledSet[h] = true
	}

	hooks := make(map[string][]hookEntry)
	for _, spec := range hookSpecs {
		if !enabledSet[spec.canonical] {
			continue
		}
		entry := hookEntry{
			Command:       spec.command,
			Matcher:       spec.matcher,
			LockieManaged: true,
		}
		for _, key := range spec.nativeKeys {
			hooks[key] = []hookEntry{entry}
		}
	}
	return hooksFile{Version: 1, Hooks: hooks}
}
