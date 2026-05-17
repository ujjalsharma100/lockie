package detect

import "sort"

// resolveOverlaps returns a stable, non-overlapping subset of in.
//
// Two findings overlap when their [Start, End) byte spans share any
// byte. When they do, the resolver picks the winner using a fixed
// precedence (PLAN.md §13.8 — substitution must be deterministic):
//
//  1. Longest match wins. A more specific rule almost always emits a
//     longer match than a generic one (e.g. anthropic-api-key beats
//     a generic entropy match because it starts earlier and ends
//     later than the inner high-entropy run).
//  2. Lower Rule.Priority wins. This is the explicit tie-breaker:
//     named gitleaks rules (priority 5–20) beat the entropy fallback
//     (priority 100).
//  3. Lexicographically smallest Rule.ID wins. Deterministic, boring,
//     keeps the test goldens stable when two equal-priority rules
//     pick out the same span.
//
// Non-overlapping findings are returned in ascending Start order so
// downstream code (notably the substituter) can walk them left to
// right and rewrite the input in a single pass.
func resolveOverlaps(in []Finding) []Finding {
	if len(in) <= 1 {
		return append([]Finding(nil), in...)
	}
	work := append([]Finding(nil), in...)
	sort.SliceStable(work, func(i, j int) bool {
		return less(work[i], work[j])
	})

	out := make([]Finding, 0, len(work))
	for _, f := range work {
		if conflicting, idx := overlapping(out, f); conflicting {
			if less(f, out[idx]) {
				out[idx] = f
			}
			continue
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Start < out[j].Start
	})
	return out
}

// less returns true when a should win over b in the precedence
// described on resolveOverlaps.
func less(a, b Finding) bool {
	alen, blen := a.End-a.Start, b.End-b.Start
	if alen != blen {
		return alen > blen
	}
	if a.Rule.Priority != b.Rule.Priority {
		return a.Rule.Priority < b.Rule.Priority
	}
	if a.Rule.ID != b.Rule.ID {
		return a.Rule.ID < b.Rule.ID
	}
	return a.Start < b.Start
}

// overlapping returns the index of any finding in out whose span
// shares a byte with f, plus a bool flag. We accept O(n²) here
// because n is the number of findings on a single hook payload — in
// practice a handful, never thousands.
func overlapping(out []Finding, f Finding) (bool, int) {
	for i, e := range out {
		if f.Start < e.End && e.Start < f.End {
			return true, i
		}
	}
	return false, -1
}
