// Integration tests for lockie hook …: CLI → daemon → substitute → agent wire JSON.
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ujjalsharma100/lockie/internal/cli"
	"github.com/ujjalsharma100/lockie/internal/daemon"
	"github.com/ujjalsharma100/lockie/internal/store/memory"
	"github.com/ujjalsharma100/lockie/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.RunMain(m))
}

func TestHookRoundTrip_ClaudeCodePostTool(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()

	raw := testutil.ReadFixture(t, "hooks/claudecode_post_tool_read.json")
	body := runHook(t, sock, []string{"hook", "post-tool"}, raw)

	assertNoLiteral(t, body, testutil.StripeSecretKey)
	if !strings.Contains(string(body), "STRIPE_KEY_") {
		t.Fatalf("response missing placeholder: %s", body)
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("response JSON: %v", err)
	}
	hs, ok := resp["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %v", resp)
	}
	updated, ok := hs["updatedToolOutput"].(string)
	if !ok {
		t.Fatalf("updatedToolOutput type = %T", hs["updatedToolOutput"])
	}
	if strings.Contains(updated, testutil.StripeSecretKey) {
		t.Fatalf("updatedToolOutput still contains literal")
	}
}

func TestHookRoundTrip_CursorPostTool(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()

	raw := testutil.ReadFixture(t, "hooks/cursor_post_tool_read.json")
	body := runHook(t, sock, []string{"hook", "post-tool"}, raw)

	assertNoLiteral(t, body, testutil.StripeSecretKey)
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("response JSON: %v", err)
	}
	if resp["continue"] != true {
		t.Fatalf("want continue:true, got %v", resp["continue"])
	}
	out, ok := resp["modifiedOutput"].(map[string]any)
	if !ok {
		t.Fatalf("missing modifiedOutput: %v", resp)
	}
	content, _ := out["content"].(string)
	if strings.Contains(content, testutil.StripeSecretKey) {
		t.Fatalf("modifiedOutput.content contains literal: %q", content)
	}
	if !strings.Contains(content, "STRIPE_KEY_") {
		t.Fatalf("modifiedOutput.content missing placeholder: %q", content)
	}
}

// TestHookRoundTrip_RedactThenRehydrate is the §8.7 exit-criterion flow:
// post-tool redacts a Read result, then pre-tool rehydrates the placeholder
// in a Bash command before execution.
func TestHookRoundTrip_RedactThenRehydrate(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()

	postIn := testutil.ReadFixture(t, "hooks/claudecode_post_tool_read.json")
	postOut := runHook(t, sock, []string{"hook", "post-tool"}, postIn)

	var postResp map[string]any
	if err := json.Unmarshal(postOut, &postResp); err != nil {
		t.Fatalf("post-tool response: %v", err)
	}
	hs := postResp["hookSpecificOutput"].(map[string]any)
	redacted, _ := hs["updatedToolOutput"].(string)
	placeholder := extractPlaceholder(t, redacted, "STRIPE_KEY_")

	preIn := []byte(`{
		"session_id": "lockie-test-session",
		"hook_event_name": "PreToolUse",
		"tool_name": "Bash",
		"tool_input": {
			"command": "curl -H 'Authorization: Bearer ` + placeholder + `' https://api.example.com"
		}
	}`)
	preOut := runHook(t, sock, []string{"hook", "pre-tool"}, preIn)

	var preResp map[string]any
	if err := json.Unmarshal(preOut, &preResp); err != nil {
		t.Fatalf("pre-tool response: %v", err)
	}
	inner := preResp["hookSpecificOutput"].(map[string]any)
	ui := inner["updatedInput"].(map[string]any)
	cmd, _ := ui["command"].(string)
	if !strings.Contains(cmd, testutil.StripeSecretKey) {
		t.Fatalf("rehydrated command missing literal: %q", cmd)
	}
	if strings.Contains(cmd, placeholder) {
		t.Fatalf("rehydrated command still has placeholder: %q", cmd)
	}
}

func runHook(t *testing.T, socket string, args []string, stdin []byte) []byte {
	t.Helper()
	full := append(append([]string{}, args...), "--socket", socket)
	root := cli.NewRoot()
	root.SetArgs(full)
	root.SetIn(bytes.NewReader(stdin))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("lockie %v: %v\noutput:\n%s", full, err, out.String())
	}
	return out.Bytes()
}

func startTestDaemon(t *testing.T) (socketPath string, stop func()) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "lk-int-")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath = filepath.Join(dir, "d.sock")
	st := memory.New()
	h, err := daemon.NewHandler(st)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	srv := daemon.NewServer(socketPath, h)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stop = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}
	return socketPath, stop
}

func assertNoLiteral(t *testing.T, body []byte, literal string) {
	t.Helper()
	if strings.Contains(string(body), literal) {
		t.Fatalf("response still contains literal: %s", body)
	}
}

func extractPlaceholder(t *testing.T, text, prefix string) string {
	t.Helper()
	i := strings.Index(text, prefix)
	if i < 0 {
		t.Fatalf("no %q placeholder in %q", prefix, text)
	}
	j := i + len(prefix)
	for j < len(text) {
		c := text[j]
		if c < '0' || c > '9' {
			break
		}
		j++
	}
	return text[i:j]
}
