package testutil

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	loadOnce sync.Once
	loadErr  error
)

// Require fails the test if test secrets could not be loaded from test/.env.
func Require(t interface {
	Helper()
	Fatal(...any)
}) {
	t.Helper()
	loadOnce.Do(loadSecrets)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
}

func ensureLoaded() error {
	loadOnce.Do(loadSecrets)
	return loadErr
}

// RunMain loads test/.env and runs m. Use from TestMain in packages that import testutil.
// Exits with status 1 when secrets are missing or invalid.
func RunMain(m interface{ Run() int }) int {
	loadOnce.Do(loadSecrets)
	if loadErr != nil {
		fmt.Fprintln(os.Stderr, loadErr)
		return 1
	}
	return m.Run()
}

func loadSecrets() {
	path, err := secretsPath()
	if err != nil {
		loadErr = err
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		loadErr = fmt.Errorf("testutil: read %s: %w\n\nCopy test/.env.example to test/.env and paste doc-shaped sample keys (see test/README.md)", path, err)
		return
	}
	vals, err := parseEnvFile(b)
	if err != nil {
		loadErr = fmt.Errorf("testutil: parse %s: %w", path, err)
		return
	}
	loadErr = applySecrets(vals)
}

func secretsPath() (string, error) {
	if p := os.Getenv("LOCKIE_TEST_ENV"); p != "" {
		return p, nil
	}
	root, err := moduleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "test", ".env"), nil
}

func moduleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("testutil: runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}

func parseEnvFile(b []byte) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(b))
	for lineNum := 1; sc.Scan(); lineNum++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected KEY=value", lineNum)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNum)
		}
		if strings.HasPrefix(val, "<") && strings.HasSuffix(val, ">") {
			return nil, fmt.Errorf("line %d: replace placeholder %s in test/.env", lineNum, val)
		}
		out[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func applySecrets(vals map[string]string) error {
	dest := map[string]*string{
		"STRIPE_SECRET_KEY":      &StripeSecretKey,
		"STRIPE_PUBLISHABLE_KEY": &StripePublishableKey,
		"STRIPE_RESTRICTED_KEY":  &StripeRestrictedKey,
		"AWS_ACCESS_KEY_ID":      &AWSAccessKeyID,
		"AWS_SECRET_ACCESS_KEY":  &AWSSecretAccessKey,
		"GITHUB_TOKEN":           &GitHubPAT,
		"SLACK_BOT_TOKEN":        &SlackBotToken,
		"ANTHROPIC_API_KEY":      &AnthropicKey,
		"GOOGLE_API_KEY":         &GoogleAPIKey,
		"JWT":                    &JWTExample,
		"INTERNAL_API_TOKEN":     &InternalAPITok,
	}
	var missing []string
	for key, ptr := range dest {
		v, ok := vals[key]
		if !ok || v == "" {
			missing = append(missing, key)
			continue
		}
		*ptr = v
	}
	if len(missing) > 0 {
		return fmt.Errorf("testutil: test/.env missing required keys: %s\n\nCopy test/.env.example to test/.env and fill every value", strings.Join(missing, ", "))
	}
	return nil
}
