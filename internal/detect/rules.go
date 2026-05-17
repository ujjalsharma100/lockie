package detect

// RuleDef is the on-disk shape of a detection rule. It mirrors the
// subset of the gitleaks rule schema Lockie cares about so the rules
// loaded by gitleaks_adapter.go can later be replaced by the live
// gitleaks library without touching anything downstream.
//
// Phase 1 ships the curated default set inline (see DefaultRules
// below). User and project overlays land in Phase 2 alongside the
// keychain store — they will append to / override entries returned
// here based on the rule ID.
type RuleDef struct {
	// ID is the canonical rule identifier. Mirrors the gitleaks rule
	// IDs verbatim where applicable so a future swap-in of the real
	// gitleaks library leaves rule attribution stable.
	ID string
	// PlaceholderPrefix is the semantic prefix the substitution engine
	// uses when minting placeholders for matches of this rule.
	PlaceholderPrefix string
	// Pattern is the RE2-compatible regular expression used to find
	// matches. Lookbehind / negative-lookahead are not supported; use
	// Priority (below) to disambiguate overlaps instead.
	Pattern string
	// Group is the regex capture group whose span identifies the
	// secret. Zero means the entire match; positive numbers mean the
	// Nth submatch (1-based). Use a capture group when the pattern
	// needs anchoring context (e.g. a leading boundary character) that
	// is not itself secret.
	Group int
	// Priority is the tie-breaker the overlap resolver uses when two
	// rules match the exact same span. Lower wins. See priority.go.
	Priority int
}

// DefaultRules returns the curated rule set Lockie ships in Phase 1.
// Patterns are sourced verbatim from the gitleaks v8 ruleset
// (https://github.com/gitleaks/gitleaks/blob/master/config/gitleaks.toml)
// to keep attribution stable when the live gitleaks library is wired
// in. Coverage focuses on the secret types empirically present in
// developer environments (PLAN.md §4): cloud providers, code-hosting
// PATs, LLM API keys, and standards-based tokens (JWT).
func DefaultRules() []RuleDef {
	return []RuleDef{
		{
			ID:                "stripe-access-token",
			PlaceholderPrefix: "STRIPE_KEY",
			Pattern:           `\b(?:sk|rk)_(?:test|live|prod)_[A-Za-z0-9]{24,247}\b`,
			Priority:          10,
		},
		{
			ID:                "aws-access-token",
			PlaceholderPrefix: "AWS_ACCESS_KEY_ID",
			Pattern:           `\b(?:AKIA|ASIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|APKA|ABIA|ACCA)[A-Z0-9]{16}\b`,
			Priority:          10,
		},
		{
			ID:                "github-personal-access-token",
			PlaceholderPrefix: "GITHUB_TOKEN",
			Pattern:           `\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36}\b`,
			Priority:          10,
		},
		{
			ID:                "github-fine-grained-pat",
			PlaceholderPrefix: "GITHUB_TOKEN",
			Pattern:           `\bgithub_pat_[A-Za-z0-9_]{82}\b`,
			Priority:          10,
		},
		{
			ID:                "slack-bot-token",
			PlaceholderPrefix: "SLACK_TOKEN",
			Pattern:           `\bxox[abprs]-[A-Za-z0-9-]{10,200}`,
			Priority:          10,
		},
		{
			ID:                "anthropic-api-key",
			PlaceholderPrefix: "ANTHROPIC_KEY",
			Pattern:           `\bsk-ant-(?:api|admin)\d{2}-[A-Za-z0-9_-]{40,200}`,
			Priority:          5,
		},
		{
			ID:                "google-api-key",
			PlaceholderPrefix: "GOOGLE_API_KEY",
			Pattern:           `\bAIza[0-9A-Za-z_-]{35}\b`,
			Priority:          10,
		},
		{
			ID:                "jwt",
			PlaceholderPrefix: "JWT",
			Pattern:           `\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`,
			Priority:          20,
		},
	}
}
