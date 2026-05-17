// Package detect implements the Lockie secret-detection pipeline. It
// exposes a small Detector interface (Scan / ScanStream) and a default
// Engine that fans input out to multiple detectors, then resolves any
// overlapping findings into a stable, non-overlapping result.
//
// Cross-reference: IMPLEMENTATION.md §3.3, §8.4 and PLAN.md §4.
package detect

import (
	"fmt"
	"io"
)

// Finding is one match emitted by a Detector. Start/End are byte
// offsets into the input that was passed to Scan (or into the
// underlying stream when emitted by ScanStream).
type Finding struct {
	Match      string
	Start, End int
	Rule       Rule
	Confidence float64
}

// Rule describes one detection rule. The placeholder prefix flows
// through into the substitution engine: e.g. a Stripe rule with
// PlaceholderPrefix "STRIPE_KEY" produces placeholders like
// "STRIPE_KEY_1", "STRIPE_KEY_2", ... within a session.
//
// Priority is used to break ties when two rules cover overlapping
// regions of input; lower wins. See priority.go.
type Rule struct {
	ID                string
	PlaceholderPrefix string
	Priority          int
	Source            string
}

// Detector scans bytes for secrets. Implementations must be safe for
// concurrent use: the daemon may call Scan from multiple goroutines.
type Detector interface {
	Scan(input []byte) ([]Finding, error)
	ScanStream(r io.Reader, emit func(Finding)) error
}

// Engine fans input out to a chain of detectors and consolidates the
// results via the overlap resolver in priority.go. It implements the
// Detector interface itself so it can be composed.
type Engine struct {
	detectors []Detector
	// streamLookback is the per-line buffer ceiling used by ScanStream.
	// Defaults to defaultStreamLookback.
	streamLookback int
}

// NewEngine constructs an Engine from an explicit list of detectors.
// At least one detector is required; passing zero is a programmer
// error and panics.
func NewEngine(detectors ...Detector) *Engine {
	if len(detectors) == 0 {
		panic("detect: NewEngine requires at least one Detector")
	}
	return &Engine{
		detectors:      detectors,
		streamLookback: defaultStreamLookback,
	}
}

// NewDefaultEngine returns the Phase 1 detector chain: the
// gitleaks-derived rule set followed by an entropy-with-keyword
// detector for things the rule set misses. The order matches PLAN.md §4.
func NewDefaultEngine() (*Engine, error) {
	g, err := NewGitleaksDetector(DefaultRules())
	if err != nil {
		return nil, fmt.Errorf("detect: build gitleaks detector: %w", err)
	}
	return NewEngine(g, NewEntropyDetector()), nil
}

// WithStreamLookback overrides the default per-line buffer ceiling
// used by ScanStream. It returns the receiver to allow chaining.
func (e *Engine) WithStreamLookback(n int) *Engine {
	if n > 0 {
		e.streamLookback = n
	}
	return e
}

// Scan runs every detector in turn, then resolves overlapping
// findings into a stable, non-overlapping result.
func (e *Engine) Scan(input []byte) ([]Finding, error) {
	var all []Finding
	for _, d := range e.detectors {
		findings, err := d.Scan(input)
		if err != nil {
			return nil, err
		}
		all = append(all, findings...)
	}
	return resolveOverlaps(all), nil
}

// ScanStream runs the engine over r line-by-line, emitting each
// resolved finding to emit. Byte offsets are translated to the
// underlying stream's coordinate space.
func (e *Engine) ScanStream(r io.Reader, emit func(Finding)) error {
	return scanStream(r, e, emit, e.streamLookback)
}
