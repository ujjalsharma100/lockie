package placeholder

import (
	"errors"
	"fmt"
	"sync"
)

// ErrUnknownPlaceholder is returned by Session.ResolveAlias when a
// placeholder identifier is not registered in this session. The
// Substituter's Rehydrate path treats this as "pass through unchanged"
// (PLAN.md §13: only registered placeholders are substituted), so
// unknown placeholders flow downstream verbatim.
var ErrUnknownPlaceholder = errors.New("placeholder: unknown placeholder")

// Session is the per-session map: literal ↔ placeholder mintings that
// live from SessionStart to SessionStop and vanish on Stop. It
// satisfies the substitute.SessionMap interface by structure — no
// import edge, so the cycle stays broken.
//
// Operations are safe for concurrent use. The single mutex is enough
// at session scale (low-hundreds of placeholders) and keeps the
// invariants ("same literal → same placeholder") trivially correct.
type Session struct {
	mu        sync.Mutex
	byLiteral map[string]string // literal bytes (as string key) → placeholder
	byName    map[string][]byte // placeholder → literal copy
	counters  map[string]int    // prefix → next counter
}

// NewSession returns an empty session map.
func NewSession() *Session {
	return &Session{
		byLiteral: make(map[string]string),
		byName:    make(map[string][]byte),
		counters:  make(map[string]int),
	}
}

// PlaceholderFor returns the placeholder bound to literal under prefix,
// minting a fresh one if necessary. Same literal always returns the
// same placeholder for the life of the session — even across different
// prefixes the first call wins. This guarantees Redact is idempotent
// (PLAN.md §6).
func (s *Session) PlaceholderFor(literal []byte, prefix string) (string, error) {
	if len(literal) == 0 {
		return "", fmt.Errorf("placeholder: empty literal")
	}
	if !ValidPrefix(prefix) {
		return "", fmt.Errorf("placeholder: invalid prefix %q (need [A-Z][A-Z0-9_]+)", prefix)
	}
	key := string(literal)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.byLiteral[key]; ok {
		return existing, nil
	}
	s.counters[prefix]++
	name := fmt.Sprintf("%s_%d", prefix, s.counters[prefix])
	// Defensive: a freshly minted name must not collide with a
	// previously minted one under a different counter path. Counters
	// are monotonic per prefix, so the only way to hit this branch is
	// programmer error elsewhere — surface loudly.
	if _, exists := s.byName[name]; exists {
		return "", fmt.Errorf("placeholder: minted name %s already in use", name)
	}
	s.byLiteral[key] = name
	s.byName[name] = append([]byte(nil), literal...)
	return name, nil
}

// ResolveAlias returns the literal bound to name, or ErrUnknownPlaceholder.
// The returned slice is a fresh copy — callers may mutate it.
func (s *Session) ResolveAlias(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lit, ok := s.byName[name]
	if !ok {
		return nil, ErrUnknownPlaceholder
	}
	return append([]byte(nil), lit...), nil
}

// KnownPlaceholders returns every placeholder currently registered in
// the session. Order is unspecified.
func (s *Session) KnownPlaceholders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.byName))
	for k := range s.byName {
		out = append(out, k)
	}
	return out
}

// Len reports the number of placeholders currently registered.
func (s *Session) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byName)
}
