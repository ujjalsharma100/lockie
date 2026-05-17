package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

// installAgainstBaseline writes the baseline fixture into a tmp home,
// installs Lockie, and returns the resulting bytes plus the agent.
func installAgainstBaseline(t *testing.T) (*Agent, []byte) {
	t.Helper()
	a := newTestAgent(t)
	settingsPath := filepath.Join(a.homeDir, configDirName, settingsFilename)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	baseline, err := os.ReadFile("../../../test/fixtures/settings/claudecode_baseline.json")
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	if err := os.WriteFile(settingsPath, baseline, 0o600); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	if err := a.Install(agent.InstallOptions{Scope: agent.ScopeUser}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read post-install: %v", err)
	}
	return a, got
}

func TestInstall_BaselineMatchesGolden(t *testing.T) {
	_, got := installAgainstBaseline(t)
	want, err := os.ReadFile("../../../test/fixtures/settings/claudecode_with_hooks.json")
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
	settingsPath := filepath.Join(a.homeDir, configDirName, settingsFilename)
	second, err := os.ReadFile(settingsPath)
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
	settingsPath := filepath.Join(a.homeDir, configDirName, settingsFilename)
	got, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read post-uninstall: %v", err)
	}
	want, err := os.ReadFile("../../../test/fixtures/settings/claudecode_baseline.json")
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
	settingsPath := filepath.Join(a.homeDir, configDirName, settingsFilename)
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("settings.json exists after uninstall on absent file: %v", err)
	}
}

func TestInstall_PreservesUserHookEntries(t *testing.T) {
	a := newTestAgent(t)
	settingsPath := filepath.Join(a.homeDir, configDirName, settingsFilename)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := []byte(`{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {"type": "command", "command": "/usr/local/bin/audit"}
        ]
      }
    ]
  }
}
`)
	if err := os.WriteFile(settingsPath, original, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := a.Install(agent.InstallOptions{Scope: agent.ScopeUser}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	out, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	pre, _ := parsed["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(pre) != 2 {
		t.Fatalf("PreToolUse has %d entries, want 2 (user + lockie); got %v", len(pre), pre)
	}
	// One Lockie-managed entry and one user-authored entry.
	var sawUser, sawLockie bool
	for _, e := range pre {
		em := e.(map[string]any)
		if isLockieManaged(em) {
			sawLockie = true
		} else {
			sawUser = true
		}
	}
	if !sawUser || !sawLockie {
		t.Fatalf("missing entries: sawUser=%v sawLockie=%v", sawUser, sawLockie)
	}

	// Uninstall must remove ONLY the Lockie entry, leaving the user entry.
	if err := a.Uninstall(agent.ScopeUser); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	out2, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var after map[string]any
	if err := json.Unmarshal(out2, &after); err != nil {
		t.Fatalf("parse: %v", err)
	}
	pre2 := after["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(pre2) != 1 {
		t.Fatalf("after uninstall PreToolUse has %d entries, want 1", len(pre2))
	}
	if isLockieManaged(pre2[0].(map[string]any)) {
		t.Fatalf("remaining entry is Lockie-managed: %v", pre2[0])
	}
}

func TestInstall_DryRunDoesNotTouchDisk(t *testing.T) {
	a := newTestAgent(t)
	settingsPath := filepath.Join(a.homeDir, configDirName, settingsFilename)
	opts := agent.InstallOptions{Scope: agent.ScopeUser, DryRun: true}
	if err := a.Install(opts); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("settings.json should not exist after dry-run, stat err: %v", err)
	}
}

func TestMergeInstall_SkipsDisabledHooks(t *testing.T) {
	out := mergeInstall(map[string]any{}, []agent.HookType{agent.HookPromptSubmit})
	hooks := out["hooks"].(map[string]any)
	if _, ok := hooks["UserPromptSubmit"]; !ok {
		t.Fatalf("missing UserPromptSubmit key")
	}
	for _, key := range []string{"PreToolUse", "PostToolUse", "SessionStart", "Stop"} {
		if _, ok := hooks[key]; ok {
			t.Errorf("unexpected hook key %q with enabled set {PromptSubmit}", key)
		}
	}
}

func TestIsLockieManaged(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{map[string]any{"_lockie_managed": true}, true},
		{map[string]any{"_lockie_managed": false}, false},
		{map[string]any{"matcher": "Bash"}, false},
		{nil, false},
		{"not a map", false},
	}
	for i, tc := range cases {
		if got := isLockieManaged(tc.in); got != tc.want {
			t.Errorf("case %d: isLockieManaged(%v) = %v, want %v", i, tc.in, got, tc.want)
		}
	}
}

func TestCanonicalFor(t *testing.T) {
	if h, ok := canonicalFor("UserPromptSubmit"); !ok || h != agent.HookPromptSubmit {
		t.Fatalf("UserPromptSubmit → %v ok=%v", h, ok)
	}
	if _, ok := canonicalFor("Nonexistent"); ok {
		t.Fatalf("Nonexistent should not be canonical")
	}
}

func TestRenderSettings_Stable(t *testing.T) {
	in := map[string]any{"hooks": map[string]any{"X": []any{}}, "model": "m"}
	a, errA := renderSettings(in)
	b, errB := renderSettings(in)
	if errA != nil || errB != nil {
		t.Fatalf("render error: %v / %v", errA, errB)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("render not deterministic across calls")
	}
}
