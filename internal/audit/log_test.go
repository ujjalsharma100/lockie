package audit_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ujjalsharma100/lockie/internal/audit"
	"github.com/ujjalsharma100/lockie/internal/detect"
	"github.com/ujjalsharma100/lockie/internal/placeholder"
	"github.com/ujjalsharma100/lockie/internal/substitute"
	"github.com/ujjalsharma100/lockie/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.RunMain(m))
}

func TestLog_AppendAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	log, err := audit.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ts := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	if err := log.Append(audit.Event{
		Timestamp:   ts,
		SessionID:   "sess-1",
		Tool:        "Read",
		RuleID:      "stripe-secret-key",
		Placeholder: "STRIPE_KEY_1",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	events, err := audit.Read(path, audit.Filter{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Placeholder != "STRIPE_KEY_1" {
		t.Errorf("placeholder = %q", events[0].Placeholder)
	}
}

func TestLog_ReadFiltersSinceAndName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	log, err := audit.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	base := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	_ = log.Append(
		audit.Event{Timestamp: base, SessionID: "s", Tool: "Read", RuleID: "r1", Placeholder: "A_1"},
		audit.Event{Timestamp: base.Add(time.Hour), SessionID: "s", Tool: "Read", RuleID: "r2", Placeholder: "B_1"},
	)
	events, err := audit.Read(path, audit.Filter{
		Since: base.Add(30 * time.Minute),
		Name:  "B_1",
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(events) != 1 || events[0].Placeholder != "B_1" {
		t.Fatalf("filter mismatch: %#v", events)
	}
}

func TestLog_FileMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	log, err := audit.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := log.Append(audit.Event{RuleID: "r", Placeholder: "X_1"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("audit.log perm = %o, want 0600", info.Mode().Perm())
	}
}

// TestAudit_NeverLogsLiterals_10k is the §8.9 property test: across
// 10k redactions the audit file must not contain any embedded literal.
func TestAudit_NeverLogsLiterals_10k(t *testing.T) {
	if testing.Short() {
		t.Skip("10k-iteration audit property test skipped in -short mode")
	}
	path := filepath.Join(t.TempDir(), "audit.log")
	log, err := audit.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	eng, err := detect.NewDefaultEngine()
	if err != nil {
		t.Fatalf("NewDefaultEngine: %v", err)
	}
	secrets := []string{
		testutil.StripeSecretKey,
		testutil.AWSAccessKeyID,
		testutil.GitHubPAT,
		testutil.AnthropicKey,
		testutil.SlackBotToken,
	}
	for i := 0; i < 10000; i++ {
		secret := secrets[i%len(secrets)]
		input := fmt.Sprintf("line=%d STRIPE_LIVE=%s tail\n", i, secret)
		sess := placeholder.NewSession()
		sub := &substitute.Substituter{Detector: eng, Session: sess}
		_, events, err := sub.Redact([]byte(input))
		if err != nil {
			t.Fatalf("i=%d Redact: %v", i, err)
		}
		if len(events) == 0 {
			t.Fatalf("i=%d expected redaction events", i)
		}
		enriched := make([]audit.Event, len(events))
		for j, ev := range events {
			enriched[j] = ev
			enriched[j].SessionID = fmt.Sprintf("sess-%d", i)
			enriched[j].Tool = "Read"
		}
		if err := log.Append(enriched...); err != nil {
			t.Fatalf("i=%d Append: %v", i, err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(raw)
	for i := 0; i < 10000; i++ {
		secret := secrets[i%len(secrets)]
		if strings.Contains(content, secret) {
			t.Fatalf("audit log contains literal at iteration %d", i)
		}
	}
}
