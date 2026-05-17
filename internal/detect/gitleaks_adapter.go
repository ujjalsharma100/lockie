package detect

import (
	"fmt"
	"io"
	"regexp"
)

// gitleaksDetector runs a curated subset of the gitleaks v8 ruleset.
//
// Why an adapter and not the upstream library: gitleaks v8 requires
// Go 1.24.11+ and pulls in ~40 transitive deps (viper, zerolog,
// wazero/WASM runtime, charm libs, ...). For the Phase 1 MVP that
// tradeoff isn't worth it — we ship the same rule attributions
// against the same set of well-known secret formats and keep the
// dependency surface lean. Phase 2 can drop in the live library
// behind this same Detector boundary; nothing downstream needs to
// change because Rule.ID values match the gitleaks IDs verbatim.
type gitleaksDetector struct {
	rules []compiledRule
}

type compiledRule struct {
	rule    Rule
	pattern *regexp.Regexp
	group   int
}

// NewGitleaksDetector compiles defs into a Detector. Compilation
// failures fail the constructor so a malformed rule never produces a
// partially-initialised engine at runtime.
func NewGitleaksDetector(defs []RuleDef) (Detector, error) {
	compiled := make([]compiledRule, 0, len(defs))
	for _, def := range defs {
		re, err := regexp.Compile(def.Pattern)
		if err != nil {
			return nil, fmt.Errorf("detect: compile rule %q: %w", def.ID, err)
		}
		compiled = append(compiled, compiledRule{
			rule: Rule{
				ID:                def.ID,
				PlaceholderPrefix: def.PlaceholderPrefix,
				Priority:          def.Priority,
				Source:            "gitleaks:" + def.ID,
			},
			pattern: re,
			group:   def.Group,
		})
	}
	return &gitleaksDetector{rules: compiled}, nil
}

// Scan runs every compiled rule against input and returns the raw
// (possibly overlapping) findings. The Engine's overlap resolver is
// responsible for de-duplicating across detectors.
func (g *gitleaksDetector) Scan(input []byte) ([]Finding, error) {
	var findings []Finding
	for _, cr := range g.rules {
		matches := cr.pattern.FindAllSubmatchIndex(input, -1)
		for _, m := range matches {
			start, end := m[0], m[1]
			if cr.group > 0 {
				gi := cr.group * 2
				if gi+1 < len(m) && m[gi] >= 0 {
					start, end = m[gi], m[gi+1]
				}
			}
			findings = append(findings, Finding{
				Match:      string(input[start:end]),
				Start:      start,
				End:        end,
				Rule:       cr.rule,
				Confidence: 1.0,
			})
		}
	}
	return findings, nil
}

// ScanStream defers to the shared line-buffered stream scanner so
// every Detector implementation gets the same byte-offset behaviour.
func (g *gitleaksDetector) ScanStream(r io.Reader, emit func(Finding)) error {
	return scanStream(r, g, emit, defaultStreamLookback)
}
