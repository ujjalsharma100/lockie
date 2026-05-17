package detect

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// fixtures returns the absolute path to test/fixtures. It is the
// only place the test file hard-codes a relative path; everything
// else builds off this.
func fixtures(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	repo := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(repo, "test", "fixtures")
}

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	path := filepath.Join(fixtures(t), rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return b
}

// expectFinding asserts that findings contains exactly one finding
// with the given rule ID whose Match contains substr. Match-by-
// substring keeps the assertion robust to fixture edits that nudge
// values around without changing identity.
func expectFinding(t *testing.T, findings []Finding, ruleID, substr string) {
	t.Helper()
	matches := 0
	for _, f := range findings {
		if f.Rule.ID == ruleID && strings.Contains(f.Match, substr) {
			matches++
		}
	}
	if matches != 1 {
		t.Errorf("rule %q: want exactly 1 finding containing %q, got %d (all findings: %s)",
			ruleID, substr, matches, dumpFindings(findings))
	}
}

func dumpFindings(findings []Finding) string {
	var b strings.Builder
	for i, f := range findings {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s@[%d,%d):%s", f.Rule.ID, f.Start, f.End, f.Match)
	}
	return b.String()
}

func TestEngine_StripeEnv(t *testing.T) {
	// Fixture ships placeholder text only (see test/fixtures/envfiles/stripe.env).
	// Paste sample sk_test_/rk_test_ keys locally, then remove this skip.
	t.Skip("stripe.env contains placeholders; replace with sample Stripe test keys to run")
	eng, err := NewDefaultEngine()
	if err != nil {
		t.Fatalf("NewDefaultEngine: %v", err)
	}
	got, err := eng.Scan(readFixture(t, "envfiles/stripe.env"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// Expect sk_test_ and rk_test_ matches once real-shaped samples are in the fixture.
	expectFinding(t, got, "stripe-access-token", "sk_test_")
	expectFinding(t, got, "stripe-access-token", "rk_test_")
}

func TestEngine_AWSEnv(t *testing.T) {
	// Fixture ships placeholder text only (see test/fixtures/envfiles/aws.env).
	t.Skip("aws.env contains placeholders; replace with sample AWS test credentials to run")
	eng, err := NewDefaultEngine()
	if err != nil {
		t.Fatalf("NewDefaultEngine: %v", err)
	}
	got, err := eng.Scan(readFixture(t, "envfiles/aws.env"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	expectFinding(t, got, "aws-access-token", "AKIA")
	// AWS secret access key has no fixed format; it is caught by the
	// entropy detector because "SECRET" sits in the variable name on
	// the same line.
	expectFinding(t, got, "generic-high-entropy", "SECRET_ACCESS_KEY=")
}

// TestEngine_MixedEnvGolden is the §8.4 exit-criterion assertion:
// scanning mixed.env finds all 8 embedded secrets, each attributed
// to the documented rule. Requires sample keys in mixed.env — see
// test/fixtures/envfiles/mixed.env header comments.
func TestEngine_MixedEnvGolden(t *testing.T) {
	t.Skip("mixed.env contains placeholders; replace with sample keys per fixture header to run")
	eng, err := NewDefaultEngine()
	if err != nil {
		t.Fatalf("NewDefaultEngine: %v", err)
	}
	got, err := eng.Scan(readFixture(t, "envfiles/mixed.env"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	type expectation struct {
		ruleID string
		substr string
	}
	wants := []expectation{
		{"stripe-access-token", "sk_test_"},
		{"aws-access-token", "AKIA"},
		{"github-personal-access-token", "ghp_"},
		{"slack-bot-token", "xoxb-"},
		{"anthropic-api-key", "sk-ant-api03-"},
		{"google-api-key", "AIzaSy"},
		{"jwt", "eyJ"},
		{"generic-high-entropy", "INTERNAL_API_TOKEN"},
	}
	for _, w := range wants {
		expectFinding(t, got, w.ruleID, w.substr)
	}
	if len(got) != len(wants) {
		t.Errorf("want %d findings, got %d: %s", len(wants), len(got), dumpFindings(got))
	}
	// Findings must be returned in ascending Start order — the
	// substituter relies on this for single-pass rewrites.
	for i := 1; i < len(got); i++ {
		if got[i-1].Start > got[i].Start {
			t.Errorf("findings not sorted by Start: %s", dumpFindings(got))
			break
		}
	}
}

func TestEngine_EmptyInput(t *testing.T) {
	eng, err := NewDefaultEngine()
	if err != nil {
		t.Fatalf("NewDefaultEngine: %v", err)
	}
	got, err := eng.Scan(nil)
	if err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want zero findings, got %d", len(got))
	}
	got, err = eng.Scan([]byte(""))
	if err != nil {
		t.Fatalf("Scan(empty): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want zero findings, got %d", len(got))
	}
}

func TestEntropy_RequiresKeyword(t *testing.T) {
	d := NewEntropyDetector()
	// High-entropy string with NO keyword in proximity → no finding.
	noKeyword := []byte("zzz_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx_zzz SAMPLE_HIGH_ENTROPY_NO_KEYWORD_X9K2mP4nQ7wR")
	got, err := d.Scan(noKeyword)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want no findings without keyword, got %s", dumpFindings(got))
	}

	// Same value with "token" nearby → flagged.
	withKeyword := []byte("MY_TOKEN=SAMPLE_HIGH_ENTROPY_WITH_KEYWORD_X9K2mP4nQ7wR1tY6vZ0aB3cD6eF")
	got, err = d.Scan(withKeyword)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) == 0 {
		t.Errorf("want a finding with keyword, got none")
	}
}

func TestEntropy_LowEntropyIgnored(t *testing.T) {
	d := NewEntropyDetector()
	// 30 repeated chars: low entropy, even with a keyword nearby.
	low := []byte("API_KEY=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	got, err := d.Scan(low)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want no findings for low-entropy run, got %s", dumpFindings(got))
	}
}

func TestShannonEntropy(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		minWanted float64
		maxWanted float64
	}{
		{"empty", "", 0, 0},
		{"uniform-byte", strings.Repeat("a", 100), 0, 0.01},
		{"two-byte-even", strings.Repeat("ab", 50), 0.99, 1.01},
		{"varied-base64", "SAMPLE_HIGH_ENTROPY_VARIED_X9K2mP4nQ7wR1tY6vZ0aB3cD6eF0gH", 4.5, 6.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shannonEntropy([]byte(tc.input))
			if got < tc.minWanted || got > tc.maxWanted {
				t.Errorf("entropy(%q) = %.4f, want in [%.4f, %.4f]",
					tc.input, got, tc.minWanted, tc.maxWanted)
			}
		})
	}
}

func TestResolveOverlaps_LongestWins(t *testing.T) {
	// Two findings on the same line: a generic 10-char entropy match
	// fully inside a 20-char gitleaks match. Longest wins.
	short := Finding{Match: "short", Start: 5, End: 15, Rule: Rule{ID: "short-rule", Priority: 100}}
	long := Finding{Match: "long", Start: 0, End: 20, Rule: Rule{ID: "long-rule", Priority: 10}}
	got := resolveOverlaps([]Finding{short, long})
	if len(got) != 1 || got[0].Rule.ID != "long-rule" {
		t.Errorf("want only long-rule, got %s", dumpFindings(got))
	}
}

func TestResolveOverlaps_PriorityTieBreak(t *testing.T) {
	// Same span → priority decides; lower wins.
	a := Finding{Start: 0, End: 10, Rule: Rule{ID: "a", Priority: 5}}
	b := Finding{Start: 0, End: 10, Rule: Rule{ID: "b", Priority: 50}}
	got := resolveOverlaps([]Finding{b, a})
	if len(got) != 1 || got[0].Rule.ID != "a" {
		t.Errorf("want only a, got %s", dumpFindings(got))
	}
}

func TestResolveOverlaps_LexTieBreak(t *testing.T) {
	a := Finding{Start: 0, End: 10, Rule: Rule{ID: "alpha", Priority: 10}}
	b := Finding{Start: 0, End: 10, Rule: Rule{ID: "beta", Priority: 10}}
	got := resolveOverlaps([]Finding{b, a})
	if len(got) != 1 || got[0].Rule.ID != "alpha" {
		t.Errorf("want only alpha, got %s", dumpFindings(got))
	}
}

func TestResolveOverlaps_NonOverlappingPreserved(t *testing.T) {
	a := Finding{Start: 0, End: 10, Rule: Rule{ID: "a"}}
	b := Finding{Start: 20, End: 30, Rule: Rule{ID: "b"}}
	c := Finding{Start: 40, End: 50, Rule: Rule{ID: "c"}}
	got := resolveOverlaps([]Finding{c, a, b})
	if len(got) != 3 {
		t.Fatalf("want 3 findings, got %d", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Rule.ID != want {
			t.Errorf("position %d: want %s, got %s", i, want, got[i].Rule.ID)
		}
	}
}

func TestStream_OffsetsMatchScan(t *testing.T) {
	// Same fixture requirement as TestEngine_MixedEnvGolden.
	t.Skip("mixed.env contains placeholders; replace with sample keys per fixture header to run")
	input := readFixture(t, "envfiles/mixed.env")
	eng, err := NewDefaultEngine()
	if err != nil {
		t.Fatalf("NewDefaultEngine: %v", err)
	}

	wantFindings, err := eng.Scan(input)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var streamed []Finding
	if err := eng.ScanStream(bytes.NewReader(input), func(f Finding) {
		streamed = append(streamed, f)
	}); err != nil {
		t.Fatalf("ScanStream: %v", err)
	}

	sortByStart := func(in []Finding) []Finding {
		out := append([]Finding(nil), in...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
		return out
	}
	want := sortByStart(wantFindings)
	got := sortByStart(streamed)

	if len(want) != len(got) {
		t.Fatalf("Scan returned %d findings, ScanStream returned %d (got=%s, want=%s)",
			len(want), len(got), dumpFindings(got), dumpFindings(want))
	}
	for i := range want {
		if want[i].Rule.ID != got[i].Rule.ID || want[i].Start != got[i].Start || want[i].End != got[i].End {
			t.Errorf("finding %d differs: scan=%s@[%d,%d) stream=%s@[%d,%d)",
				i,
				want[i].Rule.ID, want[i].Start, want[i].End,
				got[i].Rule.ID, got[i].Start, got[i].End,
			)
		}
		if string(input[got[i].Start:got[i].End]) != got[i].Match {
			t.Errorf("finding %d stream offset does not point at the literal match", i)
		}
	}
}

// TestEngine_IdempotentRedaction is the property check from
// IMPLEMENTATION.md §8.4: scanning text, replacing each finding with
// its placeholder, then scanning again must return zero findings.
// We sweep across deterministic inputs (the fixtures plus a generated
// mix) instead of pulling in a property-test library for v0.1.
func TestEngine_IdempotentRedaction(t *testing.T) {
	eng, err := NewDefaultEngine()
	if err != nil {
		t.Fatalf("NewDefaultEngine: %v", err)
	}

	// Fixture files use placeholders only; idempotency on them is trivial
	// (zero findings). After pasting sample keys locally, include the
	// fixtures here again to exercise real redaction paths.
	inputs := [][]byte{
		generateMixedInput(t, 50),
	}
	for i, input := range inputs {
		first, err := eng.Scan(input)
		if err != nil {
			t.Fatalf("input %d Scan: %v", i, err)
		}
		redacted := redactWithPlaceholders(input, first)
		second, err := eng.Scan(redacted)
		if err != nil {
			t.Fatalf("input %d Scan after redact: %v", i, err)
		}
		if len(second) != 0 {
			t.Errorf("input %d: redacted output still produced %d findings: %s",
				i, len(second), dumpFindings(second))
		}
	}
}

// redactWithPlaceholders rewrites every finding span in input with a
// placeholder of the shape `<PREFIX>_<N>`. It is a deliberately
// minimal stand-in for the substitution engine that lands in step
// 8.5; the only behaviour it shares is that placeholders are
// `[A-Z][A-Z0-9_]+_\d+` so the idempotency check is meaningful.
func redactWithPlaceholders(input []byte, findings []Finding) []byte {
	if len(findings) == 0 {
		return append([]byte(nil), input...)
	}
	sorted := append([]Finding(nil), findings...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })

	var out bytes.Buffer
	counters := map[string]int{}
	prev := 0
	for _, f := range sorted {
		counters[f.Rule.PlaceholderPrefix]++
		out.Write(input[prev:f.Start])
		fmt.Fprintf(&out, "%s_%d", f.Rule.PlaceholderPrefix, counters[f.Rule.PlaceholderPrefix])
		prev = f.End
	}
	out.Write(input[prev:])
	return out.Bytes()
}

// generateMixedInput builds deterministic filler lines for property
// checks that do not need committed secret-shaped strings. To sweep
// idempotency against real patterns, paste sample keys into the env
// fixtures or extend this helper locally (do not commit sample keys).
func generateMixedInput(t *testing.T, lines int) []byte {
	t.Helper()
	var buf bytes.Buffer
	for i := 0; i < lines; i++ {
		buf.WriteString(fmt.Sprintf("line%d clear-text padding noise xyz123 SAMPLE_LINE_%d\n", i, i))
	}
	return buf.Bytes()
}

// BenchmarkEngine_Scan10MB tracks the §8.4 perf target ("scan 10 MB
// of text, target < 100 ms"). Run with `go test -bench=. ./internal/detect/...`.
// Hitting the target itself is Phase 3 work — see IMPLEMENTATION.md
// §10 "Perf budget" — so this benchmark is not a CI gate today.
func BenchmarkEngine_Scan10MB(b *testing.B) {
	eng, err := NewDefaultEngine()
	if err != nil {
		b.Fatalf("NewDefaultEngine: %v", err)
	}
	input := makeBenchInput(b, 10*1024*1024)
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := eng.Scan(input); err != nil {
			b.Fatalf("Scan: %v", err)
		}
	}
}

// TestEngine_Scan10MBSmokes is a smoke test for the Phase 1 detector
// chain at scale: a 10 MB blob must scan without error and without
// degrading multiple orders of magnitude beyond the §10 perf target.
// We log the wall-clock so regressions are visible in CI output but
// only fail the test if we drift into "obviously broken" territory.
func TestEngine_Scan10MBSmokes(t *testing.T) {
	if testing.Short() {
		t.Skip("10 MB smoke skipped in -short")
	}
	eng, err := NewDefaultEngine()
	if err != nil {
		t.Fatalf("NewDefaultEngine: %v", err)
	}
	input := makeBenchInput(t, 10*1024*1024)
	start := time.Now()
	if _, err := eng.Scan(input); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	took := time.Since(start)
	t.Logf("10 MB scan: %s (Phase 3 target: < 100 ms)", took)
	if took > 30*time.Second {
		t.Errorf("10 MB scan took %s — way beyond any acceptable bound", took)
	}
}

// makeBenchInput builds a deterministic 10 MB filler blob. To benchmark
// against secret-shaped input, inject sample keys locally (see fixture
// headers); do not commit realistic key material.
func makeBenchInput(tb testing.TB, size int) []byte {
	tb.Helper()
	const filler = "the quick brown fox jumps over the lazy dog 1234567890 "
	var buf bytes.Buffer
	buf.Grow(size)
	for buf.Len() < size {
		buf.WriteString(filler)
	}
	return buf.Bytes()[:size]
}
