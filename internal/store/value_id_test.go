package store

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"
)

var uuidV7RE = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewValueID_Shape(t *testing.T) {
	id, err := NewValueID()
	if err != nil {
		t.Fatalf("NewValueID: %v", err)
	}
	if !uuidV7RE.MatchString(string(id)) {
		t.Errorf("ValueID %q does not match UUID-v7 shape", id)
	}
}

func TestNewValueID_Uniqueness(t *testing.T) {
	seen := make(map[ValueID]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id, err := NewValueID()
		if err != nil {
			t.Fatalf("NewValueID: %v", err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("ValueID collision after %d iterations: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewValueID_TimestampPrefixOrdering(t *testing.T) {
	// Inject zero entropy so the only varying source is the timestamp.
	// Later wall-clock → lexicographically larger prefix (UUID v7's
	// big-endian millisecond field).
	zeros := bytes.NewReader(bytes.Repeat([]byte{0}, 16))
	t0 := time.Unix(1_700_000_000, 0)
	id0, err := newValueID(zeros, t0)
	if err != nil {
		t.Fatalf("newValueID t0: %v", err)
	}

	zeros = bytes.NewReader(bytes.Repeat([]byte{0}, 16))
	t1 := t0.Add(time.Second)
	id1, err := newValueID(zeros, t1)
	if err != nil {
		t.Fatalf("newValueID t1: %v", err)
	}
	if strings.Compare(string(id0), string(id1)) >= 0 {
		t.Errorf("UUID v7 not monotonic by timestamp: %q !< %q", id0, id1)
	}
}
