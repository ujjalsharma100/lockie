// Package store defines the storage backend Lockie uses to hold
// content-addressed secret values and the per-project aliases that
// point at them. The interface is split into two planes:
//
//   - Value plane: PutValue / GetValue / DeleteValue / FindValueByHash.
//     Values are content-addressed: one entry per unique literal,
//     regardless of how many aliases reference it.
//   - Alias plane: PutAlias / GetAlias / DeleteAlias / List*. Aliases
//     are project-scoped names that point at a ValueID; they carry
//     metadata (created/used timestamps, sha256 hash) but never the
//     literal itself.
//
// Phase 1 ships an in-process implementation (store/memory) that backs
// tests and stands in for durable storage until step 8.8 wires
// ~/.lockie/aliases.json. Phase 2 swaps in the OS keychain backend
// (PLAN.md §7); the interface here is the swap boundary.
//
// Cross-reference: IMPLEMENTATION.md §3.2 and PLAN.md §7.
package store

import (
	"errors"
	"time"
)

// ValueID is an opaque identifier for a stored literal. The current
// format is UUID v7 (see value_id.go) but callers must treat it as
// opaque — the only valid operation is string equality.
type ValueID string

// Alias is a project-scoped name pointing at a ValueID. The Hash field
// is sha256(literal) hex, useful for dedup lookups without reading
// every stored value.
type Alias struct {
	Project    string
	Name       string
	ValueID    ValueID
	Hash       string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// ErrNotFound is returned when a value or alias does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrValueInUse is returned by DeleteValue when at least one alias
// still references the value. Callers must DeleteAlias for every
// alias first (or rely on the higher-level "forget" path to GC).
var ErrValueInUse = errors.New("store: value still referenced by aliases")

// Store is the backend abstraction. Implementations must be safe for
// concurrent use from multiple goroutines (the daemon dispatches
// hooks from a goroutine per connection).
type Store interface {
	// Value plane (content-addressed).
	PutValue(literal []byte) (id ValueID, alreadyExisted bool, err error)
	GetValue(id ValueID) ([]byte, error)
	DeleteValue(id ValueID) error // refuses with ErrValueInUse if any alias still references it.

	// Alias plane.
	PutAlias(a Alias) error
	GetAlias(project, name string) (*Alias, error)
	DeleteAlias(project, name string) error
	ListAliases(project string) ([]Alias, error)
	ListAliasesByValue(id ValueID) ([]Alias, error)

	// Dedup lookup. Returns ErrNotFound when no value has that hash.
	FindValueByHash(hash string) (ValueID, error)

	// Lifecycle.
	Close() error
}
