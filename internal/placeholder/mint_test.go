package placeholder

import (
	"errors"
	"sort"
	"sync"
	"testing"
)

func TestSession_MintsAndCounters(t *testing.T) {
	s := NewSession()
	// Opaque sample literals only — do not commit vendor-shaped key material.
	a, err := s.PlaceholderFor([]byte("SAMPLE_STRIPE_LITERAL_ONE"), "STRIPE_KEY")
	if err != nil {
		t.Fatalf("PlaceholderFor a: %v", err)
	}
	if a != "STRIPE_KEY_1" {
		t.Errorf("first mint = %q; want STRIPE_KEY_1", a)
	}
	b, err := s.PlaceholderFor([]byte("SAMPLE_STRIPE_LITERAL_TWO"), "STRIPE_KEY")
	if err != nil {
		t.Fatalf("PlaceholderFor b: %v", err)
	}
	if b != "STRIPE_KEY_2" {
		t.Errorf("second mint = %q; want STRIPE_KEY_2", b)
	}
	// Different prefix → independent counter.
	c, err := s.PlaceholderFor([]byte("SAMPLE_AWS_ACCESS_KEY_LITERAL"), "AWS_ACCESS_KEY_ID")
	if err != nil {
		t.Fatalf("PlaceholderFor c: %v", err)
	}
	if c != "AWS_ACCESS_KEY_ID_1" {
		t.Errorf("aws mint = %q; want AWS_ACCESS_KEY_ID_1", c)
	}
}

func TestSession_SameLiteralReturnsSamePlaceholder(t *testing.T) {
	s := NewSession()
	first, _ := s.PlaceholderFor([]byte("the-secret"), "STRIPE_KEY")
	second, _ := s.PlaceholderFor([]byte("the-secret"), "STRIPE_KEY")
	if first != second {
		t.Errorf("idempotency broken: first=%q second=%q", first, second)
	}
	if s.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", s.Len())
	}
}

func TestSession_FirstPrefixWinsAcrossCalls(t *testing.T) {
	// Idempotency dominates re-classification: once a literal is bound
	// to a placeholder under prefix A, asking for the same literal
	// under prefix B returns the same placeholder. This is the right
	// trade-off — we'd rather have stable identifiers than ones that
	// flip prefix mid-session.
	s := NewSession()
	first, _ := s.PlaceholderFor([]byte("the-secret"), "STRIPE_KEY")
	second, err := s.PlaceholderFor([]byte("the-secret"), "AWS_ACCESS_KEY_ID")
	if err != nil {
		t.Fatalf("PlaceholderFor 2: %v", err)
	}
	if second != first {
		t.Errorf("expected stable placeholder; got first=%q second=%q", first, second)
	}
}

func TestSession_ResolveAlias(t *testing.T) {
	s := NewSession()
	literal := []byte("real-bytes")
	name, _ := s.PlaceholderFor(literal, "STRIPE_KEY")
	got, err := s.ResolveAlias(name)
	if err != nil {
		t.Fatalf("ResolveAlias: %v", err)
	}
	if string(got) != string(literal) {
		t.Errorf("ResolveAlias = %q; want %q", got, literal)
	}
	// Returned slice is a copy.
	got[0] = 'X'
	got2, _ := s.ResolveAlias(name)
	if string(got2) != string(literal) {
		t.Errorf("ResolveAlias leaked internal slice")
	}
}

func TestSession_ResolveAlias_Unknown(t *testing.T) {
	s := NewSession()
	if _, err := s.ResolveAlias("STRIPE_KEY_999"); !errors.Is(err, ErrUnknownPlaceholder) {
		t.Errorf("unknown ResolveAlias returned %v; want ErrUnknownPlaceholder", err)
	}
}

func TestSession_KnownPlaceholders(t *testing.T) {
	s := NewSession()
	_, _ = s.PlaceholderFor([]byte("one"), "STRIPE_KEY")
	_, _ = s.PlaceholderFor([]byte("two"), "STRIPE_KEY")
	_, _ = s.PlaceholderFor([]byte("three"), "JWT")
	got := s.KnownPlaceholders()
	sort.Strings(got)
	want := []string{"JWT_1", "STRIPE_KEY_1", "STRIPE_KEY_2"}
	if len(got) != len(want) {
		t.Fatalf("KnownPlaceholders length = %d; want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("KnownPlaceholders[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestSession_RejectsInvalidInput(t *testing.T) {
	s := NewSession()
	if _, err := s.PlaceholderFor(nil, "STRIPE_KEY"); err == nil {
		t.Errorf("empty literal accepted")
	}
	if _, err := s.PlaceholderFor([]byte("x"), ""); err == nil {
		t.Errorf("empty prefix accepted")
	}
	if _, err := s.PlaceholderFor([]byte("x"), "lowercase"); err == nil {
		t.Errorf("lowercase prefix accepted")
	}
	if _, err := s.PlaceholderFor([]byte("x"), "X"); err == nil {
		t.Errorf("single-char prefix accepted (Pattern needs >= 2)")
	}
}

func TestSession_ConcurrentSafe(t *testing.T) {
	s := NewSession()
	literals := make([][]byte, 64)
	for i := range literals {
		literals[i] = []byte("literal-bytes-")
		literals[i] = append(literals[i], byte('A'+(i%26)), byte('0'+(i%10)))
	}

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := s.PlaceholderFor(literals[i], "STRIPE_KEY"); err != nil {
					t.Errorf("concurrent PlaceholderFor: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	if s.Len() != 64 {
		t.Errorf("expected 64 unique placeholders, got %d", s.Len())
	}
}
