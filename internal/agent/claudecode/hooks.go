package claudecode

import (
	"github.com/ujjalsharma100/lockie/internal/agent"
)

// hookCommand is one entry in the Claude Code "hooks" array — the
// shape Claude Code expects on disk.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// hookEntry is one Lockie-managed entry under a Claude Code hook key
// (e.g. "PreToolUse"). The leading-underscore fields are Lockie
// metadata; Claude Code ignores unknown keys (IMPLEMENTATION.md §6.1).
type hookEntry struct {
	LockieManaged  bool          `json:"_lockie_managed"`
	LockiePriority string        `json:"_lockie_priority,omitempty"`
	Matcher        string        `json:"matcher,omitempty"`
	Hooks          []hookCommand `json:"hooks"`
}

// settingsFile is the subset of settings.json Lockie owns. It is
// deliberately minimal: the real merger (step 8.3) preserves all
// user-authored keys; the dry-run path emits only the Lockie block so
// callers can see what would change.
type settingsFile struct {
	Hooks map[string][]hookEntry `json:"hooks"`
}

// hookSpec describes a single Lockie-managed hook entry as defined in
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
// JSON encoding sorts the map keys alphabetically when written, which
// is fine — Claude Code treats the dict as unordered.
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

// buildSettings produces the Lockie-managed slice of settings.json
// covering exactly the hooks in `enabled`. The result is a complete
// settings file body — `{"hooks": {...}}` — suitable for dry-run
// output. The real install path (step 8.3) will merge this into the
// user's existing settings.json instead of replacing it.
func buildSettings(enabled []agent.HookType) settingsFile {
	enabledSet := make(map[agent.HookType]bool, len(enabled))
	for _, h := range enabled {
		enabledSet[h] = true
	}

	hooks := make(map[string][]hookEntry, len(hookSpecs))
	for _, spec := range hookSpecs {
		if !enabledSet[spec.canonical] {
			continue
		}
		hooks[spec.nativeKey] = []hookEntry{{
			LockieManaged:  true,
			LockiePriority: spec.priority,
			Matcher:        spec.matcher,
			Hooks:          []hookCommand{{Type: "command", Command: spec.command}},
		}}
	}
	return settingsFile{Hooks: hooks}
}
