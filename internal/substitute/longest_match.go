package substitute

import "github.com/ujjalsharma100/lockie/internal/placeholder"

// placeholderSpan is one resolved placeholder match in an input
// buffer: the placeholder name plus its [start, end) byte span.
type placeholderSpan struct {
	name       string
	start, end int
}

// findRegisteredPlaceholders walks input and returns one
// placeholderSpan per registered placeholder occurrence, ordered by
// start byte. Matching follows three rules:
//
//  1. Longest match wins. We rely on RE2's leftmost-longest semantics
//     in placeholder.Pattern(), so STRIPE_KEY_10 beats STRIPE_KEY_1
//     when both could match — this is invariant #3 from
//     IMPLEMENTATION.md §3.4 and PLAN.md §6.
//  2. Only registered placeholders count. A candidate identifier that
//     is not in the session map is silently dropped, never substituted
//     (invariant #2).
//  3. Spans never overlap. RE2's FindAllIndex already guarantees that;
//     we surface it explicitly here so future callers can rely on it.
//
// The function does not back off to shorter matches: if RE2 matches
// STRIPE_KEY_10 and the session knows only STRIPE_KEY_1, the entire
// candidate is treated as unknown and passes through. That is the
// correct read of PLAN.md §6 "Substring overlap": treating a literal
// like STRIPE_KEY_10 as STRIPE_KEY_1 followed by "0" would corrupt
// downstream identifiers and surprise the model.
func findRegisteredPlaceholders(input []byte, sm SessionMap) []placeholderSpan {
	matches := placeholder.Pattern().FindAllIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]placeholderSpan, 0, len(matches))
	for _, m := range matches {
		name := string(input[m[0]:m[1]])
		if _, err := sm.ResolveAlias(name); err != nil {
			continue
		}
		out = append(out, placeholderSpan{name: name, start: m[0], end: m[1]})
	}
	return out
}
