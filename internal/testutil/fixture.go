package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FixturesDir returns the absolute path to test/fixtures.
func FixturesDir() (string, error) {
	root, err := moduleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "test", "fixtures"), nil
}

// ReadFixture reads a file under test/fixtures and expands {{KEY}} placeholders
// using values from test/.env.
func ReadFixture(t testing.TB, rel string) []byte {
	t.Helper()
	if err := ensureLoaded(); err != nil {
		t.Fatal(err)
	}
	dir, err := FixturesDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	return Expand(b)
}

// Expand replaces {{KEY}} placeholders in template with loaded secret values.
func Expand(template []byte) []byte {
	s := string(template)
	for _, key := range secretKeys {
		val := secretValue(key)
		if val == "" {
			continue
		}
		s = strings.ReplaceAll(s, "{{"+key+"}}", val)
	}
	return []byte(s)
}

func secretValue(key string) string {
	switch key {
	case "STRIPE_SECRET_KEY":
		return StripeSecretKey
	case "STRIPE_PUBLISHABLE_KEY":
		return StripePublishableKey
	case "STRIPE_RESTRICTED_KEY":
		return StripeRestrictedKey
	case "AWS_ACCESS_KEY_ID":
		return AWSAccessKeyID
	case "AWS_SECRET_ACCESS_KEY":
		return AWSSecretAccessKey
	case "GITHUB_TOKEN":
		return GitHubPAT
	case "SLACK_BOT_TOKEN":
		return SlackBotToken
	case "ANTHROPIC_API_KEY":
		return AnthropicKey
	case "GOOGLE_API_KEY":
		return GoogleAPIKey
	case "JWT":
		return JWTExample
	case "INTERNAL_API_TOKEN":
		return InternalAPITok
	default:
		return ""
	}
}
