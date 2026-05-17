package detect

import (
	"io"
	"math"
	"regexp"
	"strings"
)

// Default knobs for the entropy detector. They are tuned to bias
// toward false positives over false negatives (PLAN.md §4): a placeholder
// the agent didn't need is recoverable; a missed secret reaching the
// model is not.
const (
	defaultEntropyMinLen   = 20
	defaultEntropyThreshold = 4.0
	// proximityWindow is how many bytes ahead of a keyword we look for
	// a high-entropy candidate. Sized to comfortably span common
	// shapes like `API_TOKEN="..."` or `password: <value>` on the same
	// line.
	proximityWindow = 200
)

// entropyKeywords are the tokens that, when seen near a high-entropy
// run, lift that run from "random noise" to "probable secret." Match
// is case-insensitive (entropyDetector lowercases input).
var entropyKeywords = []string{
	"secret", "token", "key", "password", "passwd", "pwd",
	"api", "auth", "credential", "bearer",
}

// candidatePattern picks out runs that *could* be high-entropy
// secrets: contiguous base64-ish characters of at least
// defaultEntropyMinLen. `=` is deliberately excluded so an env-style
// `FOO_KEY=value` line splits into separate candidates for the
// variable name and the value — otherwise the variable name pads the
// candidate span and the longest-wins overlap resolver beats the
// specific gitleaks match for the value (PLAN.md §13.8).
var candidatePattern = regexp.MustCompile(`[A-Za-z0-9+/_-]{20,}`)

// entropyDetector emits findings for high-Shannon-entropy strings
// that appear near a known credential-ish keyword. It complements
// the gitleaksDetector by catching secrets whose exact format
// Lockie doesn't recognise (AWS secret access keys, custom
// internal-API tokens, ...).
type entropyDetector struct {
	minLen      int
	minEntropy  float64
	keywords    []string
	proximity   int
	placeholder string
	priority    int
}

// NewEntropyDetector returns the Phase 1 entropy detector with the
// defaults documented above. Knobs are intentionally not exposed at
// the call-site for v0.1; the daemon will gain a rules.toml override
// in Phase 2.
func NewEntropyDetector() Detector {
	return &entropyDetector{
		minLen:      defaultEntropyMinLen,
		minEntropy:  defaultEntropyThreshold,
		keywords:    entropyKeywords,
		proximity:   proximityWindow,
		placeholder: "SECRET",
		// Priority 100 — every named gitleaks rule wins on overlap.
		priority: 100,
	}
}

// Scan finds every candidate run and emits a Finding when entropy is
// over threshold AND a keyword is within proximity bytes before the
// candidate. The keyword test runs against a lowercased copy of the
// input; offsets are reported in the original coordinate space.
func (e *entropyDetector) Scan(input []byte) ([]Finding, error) {
	if len(input) == 0 {
		return nil, nil
	}
	lower := strings.ToLower(string(input))
	candidates := candidatePattern.FindAllIndex(input, -1)
	if len(candidates) == 0 {
		return nil, nil
	}
	rule := Rule{
		ID:                "generic-high-entropy",
		PlaceholderPrefix: e.placeholder,
		Priority:          e.priority,
		Source:            "entropy:keyword-proximity",
	}
	var findings []Finding
	for _, span := range candidates {
		start, end := span[0], span[1]
		if end-start < e.minLen {
			continue
		}
		entropy := shannonEntropy(input[start:end])
		if entropy < e.minEntropy {
			continue
		}
		if !keywordNear(lower, start, e.keywords, e.proximity) {
			continue
		}
		findings = append(findings, Finding{
			Match:      string(input[start:end]),
			Start:      start,
			End:        end,
			Rule:       rule,
			Confidence: math.Min(1.0, entropy/6.0),
		})
	}
	return findings, nil
}

// ScanStream delegates to the shared line-buffered scanner.
func (e *entropyDetector) ScanStream(r io.Reader, emit func(Finding)) error {
	return scanStream(r, e, emit, defaultStreamLookback)
}

// shannonEntropy returns the Shannon entropy (bits per symbol) of b.
// Returns 0 for empty input.
func shannonEntropy(b []byte) float64 {
	if len(b) == 0 {
		return 0
	}
	var counts [256]int
	for _, c := range b {
		counts[c]++
	}
	total := float64(len(b))
	var ent float64
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / total
		ent -= p * math.Log2(p)
	}
	return ent
}

// keywordNear reports whether any keyword appears in lower at an
// offset within window bytes before pos. Same-line behaviour: the
// window comfortably covers `KEY = "<value>"` shapes without leaking
// matches across many lines of unrelated text.
func keywordNear(lower string, pos int, keywords []string, window int) bool {
	from := pos - window
	if from < 0 {
		from = 0
	}
	region := lower[from:pos]
	for _, kw := range keywords {
		if strings.Contains(region, kw) {
			return true
		}
	}
	return false
}
