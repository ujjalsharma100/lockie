package substitute

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/ujjalsharma100/lockie/internal/detect"
	"github.com/ujjalsharma100/lockie/internal/placeholder"
)

// Redact-path tests expect vendor-shaped sample secrets in inputs/fixtures.
// Committed placeholders (SAMPLE_*_REPLACE_ME) keep push scanning happy;
// paste sample test keys locally before running those tests — see
// test/fixtures/envfiles/*.env headers for shapes. Do not commit real keys.

// newSubstituter wires a real default detector and a fresh session.
// Tests that want a custom detector instantiate Substituter directly.
func newSubstituter(t *testing.T) *Substituter {
	t.Helper()
	eng, err := detect.NewDefaultEngine()
	if err != nil {
		t.Fatalf("NewDefaultEngine: %v", err)
	}
	return &Substituter{
		Detector: eng,
		Session:  placeholder.NewSession(),
	}
}

// TestSubstitution_RoundTripPreservesLiteral checks invariant #4:
// Redact then Rehydrate returns the exact original bytes (modulo any
// non-detected substrings, which trivially survive unchanged).
func TestSubstitution_RoundTripPreservesLiteral(t *testing.T) {
	s := newSubstituter(t)
	// e.g. STRIPE_LIVE=sk_test_… tail
	input := []byte("STRIPE_LIVE=SAMPLE_STRIPE_SECRET_KEY_REPLACE_ME tail\n")
	redacted, events, err := s.Redact(input)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 redaction event, got %d", len(events))
	}
	if bytes.Contains(redacted, []byte("sk_test_")) {
		t.Errorf("redacted output still contains literal: %s", redacted)
	}
	if !bytes.Contains(redacted, []byte("STRIPE_KEY_1")) {
		t.Errorf("redacted output missing expected placeholder STRIPE_KEY_1: %s", redacted)
	}
	rehydrated, err := s.Rehydrate(redacted)
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	if !bytes.Equal(rehydrated, input) {
		t.Errorf("round-trip mismatch:\n  want: %s\n  got:  %s", input, rehydrated)
	}
}

// TestSubstitution_IdempotentOnAlreadyRedacted checks that re-running
// Redact on previously-redacted output produces zero new events and
// byte-identical output (invariant #4 in its strict form).
func TestSubstitution_IdempotentOnAlreadyRedacted(t *testing.T) {
	s := newSubstituter(t)
	input := []byte("API=SAMPLE_STRIPE_SECRET_KEY_REPLACE_ME\n")
	first, _, err := s.Redact(input)
	if err != nil {
		t.Fatalf("Redact 1: %v", err)
	}
	second, events, err := s.Redact(first)
	if err != nil {
		t.Fatalf("Redact 2: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("redaction is not idempotent:\n  first:  %s\n  second: %s", first, second)
	}
	if len(events) != 0 {
		t.Errorf("re-redacting already-redacted text produced %d events; want 0", len(events))
	}
}

// TestSubstitution_PlaceholderNotInMapPassesThrough checks invariant
// #2: a placeholder-shaped identifier that the session doesn't know
// is left untouched.
func TestSubstitution_PlaceholderNotInMapPassesThrough(t *testing.T) {
	s := newSubstituter(t)
	input := []byte("noise UNKNOWN_PLACEHOLDER_42 lives here NOPE_99 tail\n")
	out, err := s.Rehydrate(input)
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	if !bytes.Equal(out, input) {
		t.Errorf("unknown placeholders must pass through:\n  want: %s\n  got:  %s", input, out)
	}
}

// TestSubstitution_LongestMatchWins checks invariant #3: STRIPE_KEY_10
// rehydrates to the literal bound to STRIPE_KEY_10, not to the literal
// bound to STRIPE_KEY_1 followed by "0".
func TestSubstitution_LongestMatchWins(t *testing.T) {
	sess := placeholder.NewSession()
	for i := 1; i <= 10; i++ {
		if _, err := sess.PlaceholderFor([]byte(fmt.Sprintf("LITERAL_%d", i)), "STRIPE_KEY"); err != nil {
			t.Fatalf("mint %d: %v", i, err)
		}
	}
	s := &Substituter{Session: sess}
	in := []byte("token=STRIPE_KEY_10 next=STRIPE_KEY_1 done\n")
	out, err := s.Rehydrate(in)
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	want := []byte("token=LITERAL_10 next=LITERAL_1 done\n")
	if !bytes.Equal(out, want) {
		t.Errorf("longest match resolution failed:\n  want: %s\n  got:  %s", want, out)
	}
}

// TestSubstitution_LongerUnknownDoesNotFallBackToShorter pins down the
// "no back-off" rule on findRegisteredPlaceholders: if RE2 matches a
// longer identifier the session doesn't know, we pass through; we do
// NOT split it into a known short prefix + trailing bytes.
func TestSubstitution_LongerUnknownDoesNotFallBackToShorter(t *testing.T) {
	sess := placeholder.NewSession()
	if _, err := sess.PlaceholderFor([]byte("short"), "STRIPE_KEY"); err != nil {
		t.Fatalf("mint: %v", err)
	}
	s := &Substituter{Session: sess}
	// STRIPE_KEY_10 is unknown (only STRIPE_KEY_1 is registered, value
	// "short"). The substituter must leave STRIPE_KEY_10 alone, not
	// rewrite it as "short0".
	in := []byte("call STRIPE_KEY_10 done\n")
	out, err := s.Rehydrate(in)
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("longer unknown placeholder must pass through:\n  want: %s\n  got:  %s", in, out)
	}
}

// TestSubstitution_RedactRehydrateRoundTrip_DeterministicSweep is the
// deterministic stand-in for the property test §8.5 calls for. We
// generate inputs by interleaving padding with rotations of a fixed
// secret list across 1..50 lines. Property-test library (rapid) is
// deferred to Phase 3 alongside the rest of the perf/property gate;
// the §8.4 idempotency test uses the same pattern.
func TestSubstitution_RedactRehydrateRoundTrip_DeterministicSweep(t *testing.T) {
	secrets := []string{
		"SAMPLE_STRIPE_SECRET_KEY_REPLACE_ME",
		"SAMPLE_AWS_ACCESS_KEY_ID_REPLACE_ME",
		"SAMPLE_GITHUB_TOKEN_REPLACE_ME",
		"SAMPLE_GOOGLE_API_KEY_REPLACE_ME",
		"SAMPLE_SLACK_BOT_TOKEN_REPLACE_ME",
	}
	for n := 1; n <= 50; n++ {
		s := newSubstituter(t)
		var buf bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&buf, "line%d=padding ", i)
			buf.WriteString(secrets[i%len(secrets)])
			buf.WriteByte('\n')
		}
		input := buf.Bytes()
		redacted, _, err := s.Redact(input)
		if err != nil {
			t.Fatalf("n=%d Redact: %v", n, err)
		}
		// Detected literals must not survive into redacted output.
		for _, secret := range secrets {
			if bytes.Contains(redacted, []byte(secret)) {
				t.Errorf("n=%d redacted output still contains %q", n, secret)
				break
			}
		}
		rehydrated, err := s.Rehydrate(redacted)
		if err != nil {
			t.Fatalf("n=%d Rehydrate: %v", n, err)
		}
		if !bytes.Equal(rehydrated, input) {
			t.Errorf("n=%d round-trip failed", n)
		}
	}
}

// TestSubstitution_RedactStableAcrossCalls pins that the substituter's
// determinism is a function of *session state*, not call ordering: the
// same input redacted twice in the same session produces the same
// placeholders (because the literal was already minted on the first
// call). Across distinct sessions, placeholders restart at _1.
func TestSubstitution_RedactStableAcrossCalls(t *testing.T) {
	s := newSubstituter(t)
	input := []byte("API=SAMPLE_STRIPE_SECRET_KEY_REPLACE_ME\n")
	first, _, err := s.Redact(input)
	if err != nil {
		t.Fatalf("Redact 1: %v", err)
	}
	second, _, err := s.Redact(input)
	if err != nil {
		t.Fatalf("Redact 2: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("same input + same session must yield identical redaction:\n  1: %s\n  2: %s", first, second)
	}
}

// TestSubstitution_RedactMultipleSecretsOneInput exercises the
// multi-finding rewrite path (sorted findings, non-overlapping spans).
func TestSubstitution_RedactMultipleSecretsOneInput(t *testing.T) {
	s := newSubstituter(t)
	input := []byte(strings.Join([]string{
		"STRIPE=SAMPLE_STRIPE_SECRET_KEY_REPLACE_ME",
		"AWS_ACCESS_KEY=SAMPLE_AWS_ACCESS_KEY_ID_REPLACE_ME",
		"GITHUB=SAMPLE_GITHUB_TOKEN_REPLACE_ME",
	}, "\n") + "\n")
	out, events, err := s.Redact(input)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d", len(events))
	}
	for _, lit := range []string{"sk_test_", "AKIA", "ghp_"} {
		if bytes.Contains(out, []byte(lit)) {
			t.Errorf("redacted output still contains %q: %s", lit, out)
		}
	}
	for _, ph := range []string{"STRIPE_KEY_1", "AWS_ACCESS_KEY_ID_1", "GITHUB_TOKEN_1"} {
		if !bytes.Contains(out, []byte(ph)) {
			t.Errorf("redacted output missing placeholder %q: %s", ph, out)
		}
	}
}

// TestSubstitution_RedactSkippedWithoutDetector and friends pin the
// constructor errors so a misconfigured Substituter fails fast.
func TestSubstitution_RedactMissingDeps(t *testing.T) {
	if _, _, err := (&Substituter{}).Redact([]byte("hello")); err == nil {
		t.Errorf("Redact without Detector succeeded; want error")
	}
	eng, _ := detect.NewDefaultEngine()
	if _, _, err := (&Substituter{Detector: eng}).Redact([]byte("hello")); err == nil {
		t.Errorf("Redact without Session succeeded; want error")
	}
}

func TestSubstitution_RehydrateMissingSession(t *testing.T) {
	if _, err := (&Substituter{}).Rehydrate([]byte("STRIPE_KEY_1")); err == nil {
		t.Errorf("Rehydrate without Session succeeded; want error")
	}
}

// TestSubstitution_ConcurrentRedactSafe stresses the session map under
// concurrent rewrites. The race detector (make test -race) catches any
// missing locks.
func TestSubstitution_ConcurrentRedactSafe(t *testing.T) {
	s := newSubstituter(t)
	input := []byte("STRIPE_LIVE=SAMPLE_STRIPE_SECRET_KEY_REPLACE_ME\n")

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if _, _, err := s.Redact(input); err != nil {
					t.Errorf("concurrent Redact: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
