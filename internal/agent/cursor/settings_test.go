package cursor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

func installAgainstBaseline(t *testing.T) (*Agent, []byte) {
	t.Helper()
	a := newTestAgent(t)
	path := filepath.Join(a.homeDir, configDirName, hooksFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	baseline, err := os.ReadFile("../../../test/fixtures/settings/cursor_baseline.json")
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	if err := os.WriteFile(path, baseline, 0o600); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	if err := a.Install(agent.InstallOptions{Scope: agent.ScopeUser}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read post-install: %v", err)
	}
	return a, got
}

func TestInstall_BaselineMatchesGolden(t *testing.T) {
	_, got := installAgainstBaseline(t)
	want, err := os.ReadFile("../../../test/fixtures/settings/cursor_with_hooks.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("install output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestInstall_Idempotent(t *testing.T) {
	a, first := installAgainstBaseline(t)
	if err := a.Install(agent.InstallOptions{Scope: agent.ScopeUser}); err != nil {
		t.Fatalf("Install() #2 error: %v", err)
	}
	path := filepath.Join(a.homeDir, configDirName, hooksFilename)
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("install is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestInstallUninstall_RoundTrip(t *testing.T) {
	a, _ := installAgainstBaseline(t)
	if err := a.Uninstall(agent.ScopeUser); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	path := filepath.Join(a.homeDir, configDirName, hooksFilename)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read post-uninstall: %v", err)
	}
	want, err := os.ReadFile("../../../test/fixtures/settings/cursor_baseline.json")
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("uninstall did not restore baseline.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestUninstall_OnAbsentFileIsNoOp(t *testing.T) {
	a := newTestAgent(t)
	if err := a.Uninstall(agent.ScopeUser); err != nil {
		t.Fatalf("Uninstall() on absent file should be a no-op, got: %v", err)
	}
	path := filepath.Join(a.homeDir, configDirName, hooksFilename)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("hooks.json exists after uninstall on absent file: %v", err)
	}
}

func TestInstall_FromScratchAddsVersion(t *testing.T) {
	a := newTestAgent(t)
	if err := a.Install(agent.InstallOptions{Scope: agent.ScopeUser}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	path := filepath.Join(a.homeDir, configDirName, hooksFilename)
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v, ok := parsed["version"].(float64); !ok || int(v) != hooksFileVersion {
		t.Fatalf("version = %v, want %d", parsed["version"], hooksFileVersion)
	}
}

func TestInstall_PreservesUserHookEntries(t *testing.T) {
	a := newTestAgent(t)
	path := filepath.Join(a.homeDir, configDirName, hooksFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := []byte(`{
  "version": 1,
  "hooks": {
    "preToolUse": [
      {"command": "/usr/local/bin/audit", "matcher": "Shell"}
    ]
  }
}
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := a.Install(agent.InstallOptions{Scope: agent.ScopeUser}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	pre := parsed["hooks"].(map[string]any)["preToolUse"].([]any)
	if len(pre) != 2 {
		t.Fatalf("preToolUse has %d entries, want 2 (user + lockie)", len(pre))
	}

	if err := a.Uninstall(agent.ScopeUser); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	out2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var after map[string]any
	if err := json.Unmarshal(out2, &after); err != nil {
		t.Fatalf("parse: %v", err)
	}
	pre2 := after["hooks"].(map[string]any)["preToolUse"].([]any)
	if len(pre2) != 1 {
		t.Fatalf("after uninstall preToolUse has %d entries, want 1", len(pre2))
	}
	if isLockieManaged(pre2[0].(map[string]any)) {
		t.Fatalf("remaining entry is Lockie-managed: %v", pre2[0])
	}
}

func TestInstall_DryRunDoesNotTouchDisk(t *testing.T) {
	a := newTestAgent(t)
	path := filepath.Join(a.homeDir, configDirName, hooksFilename)
	opts := agent.InstallOptions{Scope: agent.ScopeUser, DryRun: true}
	if err := a.Install(opts); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("hooks.json should not exist after dry-run, stat err: %v", err)
	}
}

func TestCanonicalFor(t *testing.T) {
	if h, ok := canonicalFor("postToolUseFailure"); !ok || h != agent.HookPostToolUse {
		t.Fatalf("postToolUseFailure → %v ok=%v", h, ok)
	}
	if _, ok := canonicalFor("notARealHook"); ok {
		t.Fatalf("notARealHook should not be canonical")
	}
}

func TestIsLockieManaged(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{map[string]any{"_lockie_managed": true}, true},
		{map[string]any{"_lockie_managed": false}, false},
		{map[string]any{"command": "foo"}, false},
		{nil, false},
	}
	for i, tc := range cases {
		if got := isLockieManaged(tc.in); got != tc.want {
			t.Errorf("case %d: isLockieManaged(%v) = %v, want %v", i, tc.in, got, tc.want)
		}
	}
}
