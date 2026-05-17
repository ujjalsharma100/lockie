package memory

import (
	"fmt"

	"github.com/ujjalsharma100/lockie/internal/store"
)

// ExportedState is a deep copy of an in-memory store suitable for
// persistence or test fixtures.
type ExportedState struct {
	Values  map[store.ValueID][]byte
	Aliases []store.Alias
}

// Export returns a snapshot of the store. Value bytes are copied so
// callers may mutate the returned slices freely.
func (s *Store) Export() ExportedState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := make(map[store.ValueID][]byte, len(s.values))
	for id, v := range s.values {
		values[id] = append([]byte(nil), v...)
	}
	aliases := make([]store.Alias, 0, len(s.aliases))
	for _, a := range s.aliases {
		aliases = append(aliases, a)
	}
	return ExportedState{Values: values, Aliases: aliases}
}

// Replace clears the store and loads state from snap. Used when opening
// a disk-backed store from aliases.json.
func (s *Store) Replace(snap ExportedState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = make(map[store.ValueID][]byte)
	s.hashes = make(map[string]store.ValueID)
	s.aliases = make(map[aliasKey]store.Alias)
	for id, literal := range snap.Values {
		if len(literal) == 0 {
			return fmt.Errorf("store/memory: refusing empty value %q", id)
		}
		cp := append([]byte(nil), literal...)
		s.values[id] = cp
		s.hashes[hashOf(cp)] = id
	}
	for _, a := range snap.Aliases {
		if a.Name == "" || a.ValueID == "" {
			return fmt.Errorf("store/memory: invalid alias %#v", a)
		}
		if _, ok := s.values[a.ValueID]; !ok {
			return store.ErrNotFound
		}
		if a.Hash == "" {
			a.Hash = hashOf(s.values[a.ValueID])
		}
		if a.CreatedAt.IsZero() {
			a.CreatedAt = s.now()
		}
		if a.LastUsedAt.IsZero() {
			a.LastUsedAt = a.CreatedAt
		}
		s.aliases[aliasKey{a.Project, a.Name}] = a
	}
	return nil
}
