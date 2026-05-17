// Package placeholder owns the identifier scheme Lockie uses to stand
// in for secrets in agent-visible text. Placeholders look like
// STRIPE_KEY_1 / AWS_ACCESS_KEY_ID_2 / JWT_42: an uppercase semantic
// prefix from the matching detection rule, then `_<N>` where N is a
// per-prefix counter scoped to a session.
//
// The package exposes three concerns:
//
//   - Pattern() — the RE2 expression that recognizes a placeholder.
//   - Parse()   — splits a placeholder identifier into (prefix, N).
//   - Session   — the per-session map that mints placeholders and
//                 resolves them back to literals.
//
// Cross-reference: PLAN.md §5 (Placeholder Naming) and
// IMPLEMENTATION.md §3.4 (Substituter invariants).
package placeholder

import "regexp"

// placeholderRE matches `[A-Z][A-Z0-9_]+_\d+`: an uppercase ASCII
// letter, then one or more uppercase letters / digits / underscores,
// then `_<digits>`. Examples that match: STRIPE_KEY_1, AWS_ACCESS_KEY_ID_2,
// GITHUB_TOKEN_42. Examples that don't: stripe_key_1 (lowercase),
// STRIPE_KEY (no counter), X_1 (prefix too short).
//
// The regex is leftmost-longest under RE2 (Go's default), which gives
// us "STRIPE_KEY_10 beats STRIPE_KEY_1" for free; see PLAN.md §6
// "Substring overlap".
var placeholderRE = regexp.MustCompile(`[A-Z][A-Z0-9_]+_\d+`)

// Pattern returns the compiled placeholder regex. The returned value
// is safe to share across goroutines — regexp.Regexp is concurrent-safe.
func Pattern() *regexp.Regexp { return placeholderRE }
