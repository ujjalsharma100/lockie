package agent

import "time"

// Event is the agent-neutral representation of a hook invocation. Each
// Agent.DecodeEvent translates its native wire format into this type;
// the daemon and substitution engine operate on Event exclusively.
//
// Cross-reference: IMPLEMENTATION.md §3.1.
type Event struct {
	Hook      HookType
	SessionID string
	AgentName string
	Tool      string         // canonical tool name ("Bash", "Read", …)
	Input     map[string]any // tool input (for Pre*)
	Output    *ToolOutput    // tool output (for Post*)
	Prompt    string         // raw prompt (for PromptSubmit)
	CWD       string
	Timestamp time.Time
}

// ToolOutput is the canonical shape of a tool result. Fields are
// populated according to which tool produced them; unused fields are
// left zero.
type ToolOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Content  string // file content for Read
	Diff     string // for Edit
}

// Response is the canonical shape an Agent emits back to its host
// after Lockie has redacted / rehydrated an Event. Each Agent encodes
// this into the wire format the host expects.
type Response struct {
	Modified      bool
	ModifiedInput map[string]any // for Pre*
	ModifiedText  string         // for prompt / post*
	Block         bool
	BlockReason   string
}
