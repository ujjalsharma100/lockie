// Package audit records substitution events in an append-only log.
// Events carry placeholder and rule identity only — never literals
// (PLAN.md §7).
package audit

import "time"

// Event is one redaction recorded when a literal is replaced by a
// session placeholder. SessionID and Tool are filled in by the daemon
// when the event is persisted; the substituter emits Timestamp,
// RuleID, and Placeholder only.
type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	SessionID   string    `json:"session_id,omitempty"`
	Tool        string    `json:"tool,omitempty"`
	RuleID      string    `json:"rule"`
	Placeholder string    `json:"placeholder"`
}
