// Package substitute implements the literal ↔ placeholder rewriting
// pass Lockie applies to every hook payload:
//
//   - Redact   replaces detected secrets with stable session-scoped
//              placeholders (used on PromptSubmit + PostToolUse output).
//   - Rehydrate replaces registered placeholders with their original
//              literals (used on PreToolUse input for the localSinks
//              allowlist only — egress paths are deliberately excluded
//              upstream).
//
// The four invariants the rewriter must preserve are listed in
// IMPLEMENTATION.md §3.4:
//
//  1. Substitution is a Map lookup, never fuzzy.
//  2. Only registered placeholders are substituted.
//  3. Longest-match wins on overlapping placeholders.
//  4. Same input → same output, given the same session state.
//
// SessionMap is the only seam between the substituter and the placeholder
// package — keeping the interface in this package avoids importing
// internal/placeholder from internal/substitute (and the cycle that
// would create when the daemon wires both together).
package substitute

import (
	"bytes"
	"fmt"
	"time"

	"github.com/ujjalsharma100/lockie/internal/audit"
	"github.com/ujjalsharma100/lockie/internal/detect"
)

// SessionMap is the per-session placeholder registry the substituter
// talks to. internal/placeholder.Session satisfies it by structure.
type SessionMap interface {
	// PlaceholderFor returns the placeholder bound to literal under
	// prefix, minting one if needed. Same literal → same placeholder
	// for the lifetime of the session.
	PlaceholderFor(literal []byte, prefix string) (string, error)
	// ResolveAlias returns the literal for placeholder, or an error
	// describing why it could not be resolved. The substituter treats
	// any error as "pass through unchanged" — only known placeholders
	// are rehydrated.
	ResolveAlias(placeholder string) ([]byte, error)
	// KnownPlaceholders returns all live placeholders. Currently used
	// by tests; the rehydrate path uses Pattern() + ResolveAlias()
	// directly so it stays O(input) rather than O(registry size).
	KnownPlaceholders() []string
}

// Substituter wraps a Detector and a SessionMap. A zero-value
// Substituter is unusable; populate at least Session before calling
// Rehydrate. Redact additionally needs Detector.
type Substituter struct {
	Detector detect.Detector
	Session  SessionMap

	// Now is overridable for tests; defaults to time.Now().UTC().
	Now func() time.Time
}

func (s *Substituter) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// Redact runs the detector over in, mints a placeholder for every
// finding via Session.PlaceholderFor, and returns the rewritten bytes
// plus one audit.Event per substitution (placeholder + rule only).
//
// Idempotency (invariant #4): redacted output passed back through
// Redact produces zero further findings because every rule's pattern
// is disjoint from the placeholder pattern (PLAN.md §13.8). This is
// asserted by TestSubstitution_IdempotentOnAlreadyRedacted.
func (s *Substituter) Redact(in []byte) ([]byte, []audit.Event, error) {
	if len(in) == 0 {
		return append([]byte(nil), in...), nil, nil
	}
	if s.Detector == nil {
		return nil, nil, fmt.Errorf("substitute: Redact requires a Detector")
	}
	if s.Session == nil {
		return nil, nil, fmt.Errorf("substitute: Redact requires a SessionMap")
	}
	findings, err := s.Detector.Scan(in)
	if err != nil {
		return nil, nil, fmt.Errorf("substitute: detect: %w", err)
	}
	if len(findings) == 0 {
		return append([]byte(nil), in...), nil, nil
	}

	out := bytes.NewBuffer(make([]byte, 0, len(in)))
	events := make([]audit.Event, 0, len(findings))
	prev := 0
	now := s.now()
	for _, f := range findings {
		if f.Start < prev {
			// Detector contract (priority.go) guarantees ascending Start
			// and non-overlapping spans. If that ever breaks, fail loud
			// rather than emit garbled output.
			return nil, nil, fmt.Errorf("substitute: overlapping/unsorted findings at %d", f.Start)
		}
		out.Write(in[prev:f.Start])
		ph, err := s.Session.PlaceholderFor(in[f.Start:f.End], f.Rule.PlaceholderPrefix)
		if err != nil {
			return nil, nil, fmt.Errorf("substitute: mint placeholder for %s: %w", f.Rule.ID, err)
		}
		out.WriteString(ph)
		prev = f.End
		events = append(events, audit.Event{
			Timestamp:   now,
			RuleID:      f.Rule.ID,
			Placeholder: ph,
		})
	}
	out.Write(in[prev:])
	return out.Bytes(), events, nil
}

// Rehydrate walks in for placeholder identifiers and replaces every
// registered one with its literal. Unknown placeholders pass through
// verbatim (invariant #2). The function operates on raw bytes; the
// hook layer is responsible for invoking it on the right string
// fields of a tool-input JSON.
func (s *Substituter) Rehydrate(in []byte) ([]byte, error) {
	if len(in) == 0 {
		return append([]byte(nil), in...), nil
	}
	if s.Session == nil {
		return nil, fmt.Errorf("substitute: Rehydrate requires a SessionMap")
	}
	spans := findRegisteredPlaceholders(in, s.Session)
	if len(spans) == 0 {
		return append([]byte(nil), in...), nil
	}
	out := bytes.NewBuffer(make([]byte, 0, len(in)))
	prev := 0
	for _, sp := range spans {
		out.Write(in[prev:sp.start])
		literal, err := s.Session.ResolveAlias(sp.name)
		if err != nil {
			// Should not happen — findRegisteredPlaceholders only
			// emits spans the resolver already accepted. Treat as
			// pass-through rather than dropping bytes.
			out.Write(in[sp.start:sp.end])
		} else {
			out.Write(literal)
		}
		prev = sp.end
	}
	out.Write(in[prev:])
	return out.Bytes(), nil
}
