// Package daemon implements Lockie's background process and the IPC
// protocol the CLI's `lockie hook ...` subcommands use to talk to it.
//
// Wire shape (IMPLEMENTATION.md §4): JSON-lines over a Unix domain
// socket. One request per line, one response per line; client and
// server each terminate every frame with a single '\n'.
//
//	{"id":"<uuid>","method":"hook.post_tool","params":{...}}
//	{"id":"<uuid>","result":{...}}
//	{"id":"<uuid>","error":{"code":N,"message":"..."}}
//
// `params` and `result` are method-specific (see the *Params / *Result
// types in this file). They are exchanged as json.RawMessage on the
// wire so the dispatcher can hand the raw bytes to per-method
// handlers without an intermediate any-typed decode.
package daemon

import (
	"encoding/json"
	"fmt"
)

// Method names recognised by the daemon. Centralised so handlers and
// clients reference the same string.
const (
	MethodSessionStart = "session.start"
	MethodSessionStop  = "session.stop"
	MethodHookPrompt   = "hook.prompt"
	MethodHookPreTool  = "hook.pre_tool"
	MethodHookPostTool = "hook.post_tool"
	MethodAliasAdd     = "alias.add"
	MethodAliasList    = "alias.list"
	MethodAliasGet     = "alias.get"
	MethodAliasForget  = "alias.forget"
	MethodHealth       = "health"
)

// Error codes returned in Response.Error.Code. Stable across releases;
// the CLI may special-case some of them (e.g. ErrCodeUnknownSession to
// auto-recreate the session).
const (
	ErrCodeInvalidRequest = 1 // malformed framing or missing fields
	ErrCodeUnknownMethod  = 2 // method name not in the table above
	ErrCodeInvalidParams  = 3 // params decode failed
	ErrCodeUnknownSession = 4 // session_id not registered
	ErrCodeInternal       = 5 // handler-side failure
	ErrCodeNotFound       = 6 // alias or value not found
)

// Request is the wire envelope sent from client → daemon.
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is the wire envelope sent from daemon → client. Exactly one
// of Result / Error is set; both being zero is allowed for methods
// whose result type is the empty struct (encoded as `{}`).
type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *ErrorObject    `json:"error,omitempty"`
}

// ErrorObject is the failure payload carried in Response.Error.
type ErrorObject struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorObject) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("daemon error %d: %s", e.Code, e.Message)
}

// ---- method param / result shapes ------------------------------------------

// SessionStartParams optionally carries a SessionID supplied by the
// hosting agent (Claude Code / Cursor pass their own session-id in the
// event payload). When empty the daemon mints one.
type SessionStartParams struct {
	SessionID string `json:"session_id,omitempty"`
	Agent     string `json:"agent,omitempty"`
	CWD       string `json:"cwd,omitempty"`
}

// SessionStartResult returns the canonical session-id, either the one
// the caller passed in or a freshly minted ValueID-shaped string.
type SessionStartResult struct {
	SessionID string `json:"session_id"`
}

// SessionStopParams identifies the session to flush.
type SessionStopParams struct {
	SessionID string `json:"session_id"`
}

// SessionStopResult is intentionally empty; success is signalled by the
// absence of an Error in the response envelope.
type SessionStopResult struct{}

// HookPromptParams carries the raw user prompt to redact.
type HookPromptParams struct {
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt"`
}

// HookPromptResult carries the (possibly) rewritten prompt. Modified is
// true iff Redact found at least one secret.
type HookPromptResult struct {
	Modified bool   `json:"modified"`
	Prompt   string `json:"prompt"`
}

// HookPreToolParams carries the tool input map; the daemon rehydrates
// any registered placeholder strings nested inside it.
type HookPreToolParams struct {
	SessionID string         `json:"session_id"`
	Tool      string         `json:"tool"`
	Input     map[string]any `json:"input"`
}

// HookPreToolResult returns the rewritten input map.
type HookPreToolResult struct {
	Modified bool           `json:"modified"`
	Input    map[string]any `json:"input"`
}

// HookPostToolOutput mirrors agent.ToolOutput on the wire. The JSON
// field names use snake_case for consistency with the rest of the
// protocol; the daemon translates to/from the canonical struct.
type HookPostToolOutput struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Content  string `json:"content,omitempty"`
	Diff     string `json:"diff,omitempty"`
}

// HookPostToolParams carries the tool output for redaction.
type HookPostToolParams struct {
	SessionID string             `json:"session_id"`
	Tool      string             `json:"tool"`
	Output    HookPostToolOutput `json:"output"`
}

// HookPostToolResult returns the rewritten tool output.
type HookPostToolResult struct {
	Modified bool               `json:"modified"`
	Output   HookPostToolOutput `json:"output"`
}

// AliasAddParams registers a durable alias for a literal value.
type AliasAddParams struct {
	Project string `json:"project,omitempty"`
	Name    string `json:"name"`
	Value   string `json:"value"`
}

// AliasAddResult reports the stored value-id and whether dedup reused
// an existing value entry.
type AliasAddResult struct {
	ValueID string `json:"value_id"`
	Deduped bool   `json:"deduped"`
}

// AliasListParams scopes the listing to one project (empty = user-global).
type AliasListParams struct {
	Project string `json:"project,omitempty"`
}

// AliasInfo is alias metadata returned by list/get — never the literal.
type AliasInfo struct {
	Project    string `json:"project"`
	Name       string `json:"name"`
	ValueID    string `json:"value_id"`
	Hash       string `json:"hash"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

// AliasListResult carries every alias in the requested scope.
type AliasListResult struct {
	Aliases []AliasInfo `json:"aliases"`
}

// AliasGetParams looks up one alias by project + name.
type AliasGetParams struct {
	Project string `json:"project,omitempty"`
	Name    string `json:"name"`
}

// AliasForgetParams removes an alias and GCs the value when unreferenced.
type AliasForgetParams struct {
	Project string `json:"project,omitempty"`
	Name    string `json:"name"`
}

// HealthResult is the body of a `health` response: daemon identity plus
// uptime for liveness checks.
type HealthResult struct {
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	PID           int    `json:"pid"`
	Sessions      int    `json:"sessions"`
}
