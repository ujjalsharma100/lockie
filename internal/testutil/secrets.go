// Package testutil holds test credentials loaded from test/.env (gitignored).
// Copy test/.env.example to test/.env and fill with doc-shaped sample keys.
package testutil

// Populated by loadSecrets from test/.env on first Require or ReadFixture.
var (
	StripeSecretKey      string
	StripePublishableKey string
	StripeRestrictedKey  string
	AWSAccessKeyID       string
	AWSSecretAccessKey   string
	GitHubPAT            string
	SlackBotToken        string
	AnthropicKey         string
	GoogleAPIKey         string
	JWTExample           string
	InternalAPITok       string
)

// secretKeys lists env keys in template substitution order (longest names first
// is unnecessary here because placeholders are disjoint).
var secretKeys = []string{
	"STRIPE_SECRET_KEY",
	"STRIPE_PUBLISHABLE_KEY",
	"STRIPE_RESTRICTED_KEY",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"GITHUB_TOKEN",
	"SLACK_BOT_TOKEN",
	"ANTHROPIC_API_KEY",
	"GOOGLE_API_KEY",
	"INTERNAL_API_TOKEN",
	"JWT",
}
