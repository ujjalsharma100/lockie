package disk_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ujjalsharma100/lockie/internal/store"
	"github.com/ujjalsharma100/lockie/internal/store/disk"
)

func TestOpen_PersistAndReload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases.json")

	s1, err := disk.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	id, deduped, err := s1.PutValue([]byte("phase1-literal-xyz"))
	if err != nil || deduped {
		t.Fatalf("PutValue: id=%s deduped=%v err=%v", id, deduped, err)
	}
	if err := s1.PutAlias(store.Alias{Name: "MYKEY", ValueID: id}); err != nil {
		t.Fatalf("PutAlias: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o; want 0600", info.Mode().Perm())
	}

	s2, err := disk.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.GetAlias("", "MYKEY")
	if err != nil {
		t.Fatalf("GetAlias: %v", err)
	}
	if got.ValueID != id {
		t.Errorf("value_id = %s; want %s", got.ValueID, id)
	}
	val, err := s2.GetValue(id)
	if err != nil {
		t.Fatalf("GetValue: %v", err)
	}
	if string(val) != "phase1-literal-xyz" {
		t.Errorf("literal = %q", val)
	}
}

func TestForget_GCsValue(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases.json")
	s, err := disk.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	id, _, err := s.PutValue([]byte("gc-me-please-12345"))
	if err != nil {
		t.Fatalf("PutValue: %v", err)
	}
	if err := s.PutAlias(store.Alias{Name: "N", ValueID: id}); err != nil {
		t.Fatalf("PutAlias: %v", err)
	}
	if err := s.DeleteAlias("", "N"); err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}
	if err := s.DeleteValue(id); err != nil {
		t.Fatalf("DeleteValue: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc struct {
		Values  map[string]string `json:"values"`
		Aliases []any             `json:"aliases"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(doc.Values) != 0 || len(doc.Aliases) != 0 {
		t.Errorf("expected empty store after GC, got values=%d aliases=%d", len(doc.Values), len(doc.Aliases))
	}
}
