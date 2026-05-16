// Package agent defines the coding-tool adapter abstraction. Each
// supported agent (Claude Code, Cursor, …) implements Agent and is
// registered via the global registry in registry.go.
//
// Cross-reference: IMPLEMENTATION.md §3.1.
package agent

import "io"

// Scope describes where an agent's config should be written.
type Scope int

const (
	// ScopeUser writes to user-global config (e.g. ~/.claude/settings.json).
	ScopeUser Scope = iota
	// ScopeProject writes to project config (intended to be committed).
	ScopeProject
	// ScopeProjectLocal writes to project-local config (gitignored).
	ScopeProjectLocal
)

// String returns the canonical scope label used in CLI flags and logs.
func (s Scope) String() string {
	switch s {
	case ScopeUser:
		return "user"
	case ScopeProject:
		return "project"
	case ScopeProjectLocal:
		return "project-local"
	default:
		return "unknown"
	}
}

// HookType is the canonical (agent-neutral) hook name. Each Agent impl
// translates these to/from its native names — see IMPLEMENTATION.md §6.0.
type HookType string

const (
	HookPromptSubmit HookType = "PromptSubmit"
	HookPreToolUse   HookType = "PreToolUse"
	HookPostToolUse  HookType = "PostToolUse"
	HookSessionStart HookType = "SessionStart"
	HookSessionStop  HookType = "SessionStop"
)

// AllHooks is the canonical set Lockie installs by default.
func AllHooks() []HookType {
	return []HookType{
		HookSessionStart,
		HookPromptSubmit,
		HookPreToolUse,
		HookPostToolUse,
		HookSessionStop,
	}
}

// InstallOptions controls Install(). When DryRun is true the adapter
// MUST NOT touch disk; instead it writes the JSON it would have written
// to DryRunOutput (defaulting to os.Stdout if nil) and returns nil.
type InstallOptions struct {
	Scope         Scope
	EnabledHooks  []HookType
	DaemonAddress string
	DryRun        bool
	DryRunOutput  io.Writer
}

// DetectResult reports whether an agent is installed on this machine
// and where its config lives.
type DetectResult struct {
	Installed  bool
	BinaryPath string
	ConfigDir  string
	Version    string
}

// Status reports the current Lockie integration state for one scope of
// one agent.
type Status struct {
	Installed    bool
	InstalledFor []HookType
	AgentVersion string
	SettingsPath string
	Warnings     []string
}

// Agent is the interface every coding-tool adapter implements.
//
// Adding a new agent means: (1) create internal/agent/<name>/ with an
// Agent impl, (2) register it in registry.go (or via init() in the
// adapter package). Nothing else changes.
type Agent interface {
	// Identity
	Name() string
	DisplayName() string
	SupportedHooks() []HookType

	// Discovery
	Detect() (DetectResult, error)

	// Lifecycle
	Install(opts InstallOptions) error
	Uninstall(scope Scope) error
	Status(scope Scope) (Status, error)

	// Translation: canonical Event ↔ agent-specific wire format.
	DecodeEvent(raw []byte, hook HookType) (*Event, error)
	EncodeResponse(resp *Response, hook HookType) ([]byte, error)
}
