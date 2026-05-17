// Package testutil holds fabricated credentials shaped like real vendor
// secrets so detection rules fire in tests. Values are taken from public
// documentation examples (Stripe, AWS, jwt.io, etc.) — not live credentials.
package testutil

// Stripe-shaped keys (stripe-access-token rule: sk_test_ / rk_test_).
//
//nolint:gosec // public documentation examples; not live credentials
const (
	StripeSecretKey      = "<sample-stripe-key>"
	StripePublishableKey = "<stripe-publishable-key>"
	StripeRestrictedKey  = "<stripe-restricted-key>"
)

// AWS-shaped keys (aws-access-token + generic-high-entropy for secret key).
//
//nolint:gosec
const (
	AWSAccessKeyID     = "<aws-access-key-id>"
	AWSSecretAccessKey = "<aws-secret-access-key>"
)

// Other vendor tokens matched by DefaultRules().
//
//nolint:gosec
const (
	GitHubPAT      = "<github-personal-access-token>"
	SlackBotToken  = "<slack-bot-token>"
	AnthropicKey   = "<anthropic-api-key>"
	GoogleAPIKey   = "<google-api-key>"
	JWTExample     = "<jwt-example>"
	InternalAPITok = "<internal-api-token>"
)
