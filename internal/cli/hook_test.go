package cli_test

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

const secretStripe = testutil.StripeSecretKey

func TestHook_PostTool_ClaudeCodeRoundTrip(t *testing.T) {
	sock, stop := startHookTestDaemon(t)
	defer stop()

	eventPath := filepath.Join("..", "..", "test", "fixtures", "hooks", "claudecode_post_tool_read.json")
	raw, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !strings.Contains(string(raw), secretStripe) {
		t.Fatalf("fixture missing test secret literal")
	}

	root := cli.NewRoot()
	root.SetArgs([]string{"hook", "post-tool", "--socket", sock})
	root.SetIn(bytes.NewReader(raw))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: %v\nstderr/stdout:\n%s", err, out.String())
	}

	body := out.Bytes()
	if len(body) == 0 {
		t.Fatalf("empty hook response")
	}
	if strings.Contains(string(body), secretStripe) {
		t.Fatalf("response still contains literal: %s", body)
	}
	if !strings.Contains(string(body), "STRIPE_KEY_") {
		t.Fatalf("response missing placeholder: %s", body)
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, body)
	}
	hs, ok := resp["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %v", resp)
	}
	updated, ok := hs["updatedToolOutput"].(string)
	if !ok {
		t.Fatalf("updatedToolOutput type = %T", hs["updatedToolOutput"])
	}
	if strings.Contains(updated, secretStripe) {
		t.Fatalf("updated tool output contains literal: %q", updated)
	}
	if !strings.Contains(updated, "STRIPE_KEY_") {
		t.Fatalf("updated tool output missing placeholder: %q", updated)
	}
}

func startHookTestDaemon(t *testing.T) (socketPath string, stop func()) {
	t.Helper()
	dir, err := os.MkdirTemp(shortTmp(t), "lk-hook-")
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

func shortTmp(t *testing.T) string {
	t.Helper()
	// macOS sun_path is 104 bytes; t.TempDir() paths are often too long.
	return "/tmp"
}
