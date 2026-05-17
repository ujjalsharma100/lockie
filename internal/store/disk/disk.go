// Package disk implements store.Store with persistence to
// ~/.lockie/aliases.json. Phase 1 stores literals in plaintext with
// 0600 file permissions; Phase 2 swaps this backend for the OS
// keychain without changing the Store interface.
package disk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ujjalsharma100/lockie/internal/config"
	"github.com/ujjalsharma100/lockie/internal/store"
	"github.com/ujjalsharma100/lockie/internal/store/memory"
)

const fileVersion = 1

// onDisk is the JSON shape written to aliases.json.
type onDisk struct {
	Version int           `json:"version"`
	Values  map[string]string `json:"values"`
	Aliases []aliasRecord `json:"aliases"`
}

type aliasRecord struct {
	Project    string    `json:"project"`
	Name       string    `json:"name"`
	ValueID    string    `json:"value_id"`
	Hash       string    `json:"hash"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// Store wraps the in-memory implementation and fsyncs aliases.json
// after every mutating operation.
type Store struct {
	path  string
	inner *memory.Store
	mu    sync.Mutex // serialises persist with Replace on load
}

// OpenDefault opens (or creates) the store at config.AliasesPath().
func OpenDefault() (*Store, error) {
	path, err := config.AliasesPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

// Open loads or creates a store at path.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("store/disk: mkdir %s: %w", dir, err)
		}
	}
	s := &Store{path: path, inner: memory.New()}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("store/disk: read %s: %w", s.path, err)
	}
	var doc onDisk
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("store/disk: decode %s: %w", s.path, err)
	}
	if doc.Version != 0 && doc.Version != fileVersion {
		return fmt.Errorf("store/disk: unsupported aliases.json version %d", doc.Version)
	}
	snap := memory.ExportedState{
		Values:  make(map[store.ValueID][]byte, len(doc.Values)),
		Aliases: make([]store.Alias, 0, len(doc.Aliases)),
	}
	for id, literal := range doc.Values {
		snap.Values[store.ValueID(id)] = []byte(literal)
	}
	for _, rec := range doc.Aliases {
		snap.Aliases = append(snap.Aliases, store.Alias{
			Project:    rec.Project,
			Name:       rec.Name,
			ValueID:    store.ValueID(rec.ValueID),
			Hash:       rec.Hash,
			CreatedAt:  rec.CreatedAt,
			LastUsedAt: rec.LastUsedAt,
		})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Replace(snap)
}

func (s *Store) persist() error {
	snap := s.inner.Export()
	doc := onDisk{
		Version: fileVersion,
		Values:  make(map[string]string, len(snap.Values)),
		Aliases: make([]aliasRecord, 0, len(snap.Aliases)),
	}
	for id, literal := range snap.Values {
		doc.Values[string(id)] = string(literal)
	}
	for _, a := range snap.Aliases {
		doc.Aliases = append(doc.Aliases, aliasRecord{
			Project:    a.Project,
			Name:       a.Name,
			ValueID:    string(a.ValueID),
			Hash:       a.Hash,
			CreatedAt:  a.CreatedAt,
			LastUsedAt: a.LastUsedAt,
		})
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("store/disk: encode: %w", err)
	}
	body = append(body, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("store/disk: mkdir %s: %w", dir, err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("store/disk: write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("store/disk: rename: %w", err)
	}
	return nil
}

func (s *Store) afterMutate(err error) error {
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persist()
}

func (s *Store) PutValue(literal []byte) (store.ValueID, bool, error) {
	id, existed, err := s.inner.PutValue(literal)
	return id, existed, s.afterMutate(err)
}

func (s *Store) GetValue(id store.ValueID) ([]byte, error) {
	return s.inner.GetValue(id)
}

func (s *Store) DeleteValue(id store.ValueID) error {
	return s.afterMutate(s.inner.DeleteValue(id))
}

func (s *Store) PutAlias(a store.Alias) error {
	return s.afterMutate(s.inner.PutAlias(a))
}

func (s *Store) GetAlias(project, name string) (*store.Alias, error) {
	return s.inner.GetAlias(project, name)
}

func (s *Store) DeleteAlias(project, name string) error {
	return s.afterMutate(s.inner.DeleteAlias(project, name))
}

func (s *Store) ListAliases(project string) ([]store.Alias, error) {
	return s.inner.ListAliases(project)
}

func (s *Store) ListAliasesByValue(id store.ValueID) ([]store.Alias, error) {
	return s.inner.ListAliasesByValue(id)
}

func (s *Store) FindValueByHash(hash string) (store.ValueID, error) {
	return s.inner.FindValueByHash(hash)
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persist()
}
