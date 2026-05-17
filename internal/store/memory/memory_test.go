package memory

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/ujjalsharma100/lockie/internal/store"
)

func TestPutValue_DeduplicatesByHash(t *testing.T) {
	s := New()
	literal := []byte("SAMPLE_VALUE_DUPECHECK_AAAA_BBBB_CCCC_DDDD")
	id1, existed, err := s.PutValue(literal)
	if err != nil {
		t.Fatalf("PutValue 1: %v", err)
	}
	if existed {
		t.Errorf("first PutValue reported existed=true")
	}
	id2, existed, err := s.PutValue(literal)
	if err != nil {
		t.Fatalf("PutValue 2: %v", err)
	}
	if !existed || id1 != id2 {
		t.Errorf("second PutValue did not dedup: id1=%s id2=%s existed=%v", id1, id2, existed)
	}
}

func TestGetValue_ReturnsCopy(t *testing.T) {
	s := New()
	literal := []byte("hunter2-secret-XYZ-12345")
	id, _, err := s.PutValue(literal)
	if err != nil {
		t.Fatalf("PutValue: %v", err)
	}
	got, err := s.GetValue(id)
	if err != nil {
		t.Fatalf("GetValue: %v", err)
	}
	if !bytes.Equal(got, literal) {
		t.Errorf("GetValue returned %q, want %q", got, literal)
	}
	// Mutate the returned slice; the store must not see the change.
	got[0] = 'X'
	got2, _ := s.GetValue(id)
	if !bytes.Equal(got2, literal) {
		t.Errorf("Store leaked internal slice: caller mutation observed downstream")
	}
}

func TestPutValue_RefusesEmpty(t *testing.T) {
	s := New()
	if _, _, err := s.PutValue(nil); err == nil {
		t.Errorf("PutValue(nil) succeeded; want error")
	}
	if _, _, err := s.PutValue([]byte{}); err == nil {
		t.Errorf("PutValue(empty) succeeded; want error")
	}
}

func TestAlias_LifecycleAndScope(t *testing.T) {
	clock := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	s := New(WithClock(func() time.Time { return clock }))

	id, _, err := s.PutValue([]byte("real-secret-VALUE-12345-ABCDE"))
	if err != nil {
		t.Fatalf("PutValue: %v", err)
	}
	want := store.Alias{
		Project: "proj-a",
		Name:    "STRIPE_KEY_PROD",
		ValueID: id,
	}
	if err := s.PutAlias(want); err != nil {
		t.Fatalf("PutAlias: %v", err)
	}
	got, err := s.GetAlias("proj-a", "STRIPE_KEY_PROD")
	if err != nil {
		t.Fatalf("GetAlias: %v", err)
	}
	if got.ValueID != id {
		t.Errorf("alias value-id mismatch: got %s, want %s", got.ValueID, id)
	}
	if !got.CreatedAt.Equal(clock) || !got.LastUsedAt.Equal(clock) {
		t.Errorf("alias timestamps not stamped from clock: got created=%s used=%s", got.CreatedAt, got.LastUsedAt)
	}
	if got.Hash == "" {
		t.Errorf("alias hash empty; expected sha256 of value")
	}

	// Project isolation: a different project must not see proj-a's alias.
	list, err := s.ListAliases("proj-b")
	if err != nil {
		t.Fatalf("ListAliases proj-b: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("proj-b sees proj-a's alias: %#v", list)
	}

	// ListAliasesByValue covers the cross-project rotation surface.
	if err := s.PutAlias(store.Alias{Project: "proj-b", Name: "STRIPE_KEY_DEV", ValueID: id}); err != nil {
		t.Fatalf("PutAlias proj-b: %v", err)
	}
	all, err := s.ListAliasesByValue(id)
	if err != nil {
		t.Fatalf("ListAliasesByValue: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("ListAliasesByValue returned %d entries; want 2", len(all))
	}
}

func TestDeleteValue_RefusesWhileAliased(t *testing.T) {
	s := New()
	id, _, err := s.PutValue([]byte("secret-VALUE-67890-FGHIJ-KLMNO"))
	if err != nil {
		t.Fatalf("PutValue: %v", err)
	}
	if err := s.PutAlias(store.Alias{Project: "p", Name: "N", ValueID: id}); err != nil {
		t.Fatalf("PutAlias: %v", err)
	}
	if err := s.DeleteValue(id); !errors.Is(err, store.ErrValueInUse) {
		t.Errorf("DeleteValue while aliased returned %v; want ErrValueInUse", err)
	}
	if err := s.DeleteAlias("p", "N"); err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}
	if err := s.DeleteValue(id); err != nil {
		t.Errorf("DeleteValue after alias removed: %v", err)
	}
	if _, err := s.GetValue(id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetValue after DeleteValue returned %v; want ErrNotFound", err)
	}
}

func TestFindValueByHash(t *testing.T) {
	s := New()
	literal := []byte("look-up-by-hash-XXXX-YYYY-ZZZZ")
	id, _, err := s.PutValue(literal)
	if err != nil {
		t.Fatalf("PutValue: %v", err)
	}
	got, err := s.FindValueByHash(hashOf(literal))
	if err != nil {
		t.Fatalf("FindValueByHash: %v", err)
	}
	if got != id {
		t.Errorf("FindValueByHash returned %s; want %s", got, id)
	}
	if _, err := s.FindValueByHash("0123456789abcdef"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("FindValueByHash(unknown) returned %v; want ErrNotFound", err)
	}
}

func TestPutAlias_RejectsUnknownValueID(t *testing.T) {
	s := New()
	err := s.PutAlias(store.Alias{Project: "p", Name: "N", ValueID: "ghost"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("PutAlias against unknown value returned %v; want ErrNotFound", err)
	}
}
