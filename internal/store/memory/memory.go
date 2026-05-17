// Package memory provides an in-process implementation of
// store.Store. It backs unit/integration tests and stands in for the
// durable backend until step 8.8 wires the on-disk aliases file and
// Phase 2 wires the OS keychain.
//
// Concurrency model: a single RWMutex guards every map. The maps are
// small (per-user secret counts in the dozens to low hundreds) so the
// global lock is the simpler choice over per-bucket locking.
package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/ujjalsharma100/lockie/internal/store"
)

// Store implements store.Store using in-memory maps.
type Store struct {
	mu      sync.RWMutex
	values  map[store.ValueID][]byte
	hashes  map[string]store.ValueID
	aliases map[aliasKey]store.Alias

	// now is overridable for tests; defaults to time.Now().UTC().
	now func() time.Time
	// newID is overridable for tests; defaults to store.NewValueID.
	newID func() (store.ValueID, error)
}

type aliasKey struct{ project, name string }

// Option configures a Store. Tests override the clock or id generator
// for determinism; production code passes nothing.
type Option func(*Store)

// WithClock replaces the internal clock used to stamp CreatedAt /
// LastUsedAt on aliases. Production callers should not need this.
func WithClock(fn func() time.Time) Option {
	return func(s *Store) {
		if fn != nil {
			s.now = fn
		}
	}
}

// WithIDFunc replaces the ValueID generator. Production callers should
// not need this.
func WithIDFunc(fn func() (store.ValueID, error)) Option {
	return func(s *Store) {
		if fn != nil {
			s.newID = fn
		}
	}
}

// New returns a fresh, empty in-memory store.
func New(opts ...Option) *Store {
	s := &Store{
		values:  make(map[store.ValueID][]byte),
		hashes:  make(map[string]store.ValueID),
		aliases: make(map[aliasKey]store.Alias),
		now:     func() time.Time { return time.Now().UTC() },
		newID:   store.NewValueID,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// PutValue stores literal and returns its ValueID. If a value with
// the same sha256 hash already exists, the existing ValueID is
// returned and alreadyExisted is true — content-addressed dedup.
func (s *Store) PutValue(literal []byte) (store.ValueID, bool, error) {
	if len(literal) == 0 {
		return "", false, fmt.Errorf("store/memory: refusing to store empty literal")
	}
	hash := hashOf(literal)
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.hashes[hash]; ok {
		return id, true, nil
	}
	id, err := s.newID()
	if err != nil {
		return "", false, err
	}
	s.values[id] = append([]byte(nil), literal...)
	s.hashes[hash] = id
	return id, false, nil
}

// GetValue returns a copy of the stored literal. Callers may mutate
// the returned slice freely.
func (s *Store) GetValue(id store.ValueID) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.values[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return append([]byte(nil), v...), nil
}

// DeleteValue removes a value. It refuses with ErrValueInUse if any
// alias still references it.
func (s *Store) DeleteValue(id store.ValueID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[id]
	if !ok {
		return store.ErrNotFound
	}
	for _, a := range s.aliases {
		if a.ValueID == id {
			return store.ErrValueInUse
		}
	}
	delete(s.values, id)
	delete(s.hashes, hashOf(v))
	return nil
}

// PutAlias creates or overwrites an alias entry. Timestamps default to
// the store's clock when zero; Hash defaults to sha256 of the
// referenced value when empty.
func (s *Store) PutAlias(a store.Alias) error {
	if a.Name == "" {
		return fmt.Errorf("store/memory: alias name is empty")
	}
	if a.ValueID == "" {
		return fmt.Errorf("store/memory: alias value-id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	literal, ok := s.values[a.ValueID]
	if !ok {
		return store.ErrNotFound
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = s.now()
	}
	if a.LastUsedAt.IsZero() {
		a.LastUsedAt = a.CreatedAt
	}
	if a.Hash == "" {
		a.Hash = hashOf(literal)
	}
	s.aliases[aliasKey{a.Project, a.Name}] = a
	return nil
}

// GetAlias returns a copy of the alias entry.
func (s *Store) GetAlias(project, name string) (*store.Alias, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.aliases[aliasKey{project, name}]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := a
	return &cp, nil
}

// DeleteAlias removes an alias entry. The underlying value is left
// intact; the caller decides when to GC unreferenced values.
func (s *Store) DeleteAlias(project, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := aliasKey{project, name}
	if _, ok := s.aliases[key]; !ok {
		return store.ErrNotFound
	}
	delete(s.aliases, key)
	return nil
}

// ListAliases returns every alias for project (or every "user-global"
// alias when project == ""). Order is unspecified.
func (s *Store) ListAliases(project string) ([]store.Alias, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []store.Alias
	for k, a := range s.aliases {
		if k.project == project {
			out = append(out, a)
		}
	}
	return out, nil
}

// ListAliasesByValue returns every alias pointing at id.
func (s *Store) ListAliasesByValue(id store.ValueID) ([]store.Alias, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []store.Alias
	for _, a := range s.aliases {
		if a.ValueID == id {
			out = append(out, a)
		}
	}
	return out, nil
}

// FindValueByHash returns the ValueID whose stored literal has the
// given sha256 hex, or ErrNotFound.
func (s *Store) FindValueByHash(hash string) (store.ValueID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.hashes[hash]
	if !ok {
		return "", store.ErrNotFound
	}
	return id, nil
}

// Close is a no-op for the in-memory store.
func (s *Store) Close() error { return nil }

func hashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
