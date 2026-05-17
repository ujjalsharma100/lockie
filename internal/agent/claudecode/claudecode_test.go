package claudecode

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	home := t.TempDir()
	return &Agent{
		homeDir:    home,
		projectDir: t.TempDir(),
		lookPath:   func(string) (string, error) { return "", errors.New("no binary") },
	}
}

func TestDetect_NoConfigNoBinary(t *testing.T) {
	a := newTestAgent(t)
	got, err := a.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if got.Installed {
		t.Fatalf("Installed = true, want false (empty home, no binary)")
	}
	if got.BinaryPath != "" || got.ConfigDir != "" {
		t.Fatalf("unexpected DetectResult: %+v", got)
	}
}

func TestDetect_ConfigDirPresent(t *testing.T) {
	a := newTestAgent(t)
	configDir := filepath.Join(a.homeDir, configDirName)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := a.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !got.Installed {
		t.Fatalf("Installed = false, want true with config dir present")
	}
	if got.ConfigDir != configDir {
		t.Fatalf("ConfigDir = %q, want %q", got.ConfigDir, configDir)
	}
}

func TestDetect_BinaryOnPath(t *testing.T) {
	a := newTestAgent(t)
	a.lookPath = func(name string) (string, error) {
		if name == "claude" {
			return "/usr/local/bin/claude", nil
		}
		return "", errors.New("not found")
	}
	got, err := a.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !got.Installed {
		t.Fatalf("Installed = false, want true when binary is on PATH")
	}
	if got.BinaryPath != "/usr/local/bin/claude" {
		t.Fatalf("BinaryPath = %q", got.BinaryPath)
	}
	if got.ConfigDir != "" {
		t.Fatalf("ConfigDir = %q, want empty (no config dir present)", got.ConfigDir)
	}
}

func TestStatus_NoSettingsFile(t *testing.T) {
	a := newTestAgent(t)
	st, err := a.Status(agent.ScopeUser)
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	wantPath := filepath.Join(a.homeDir, configDirName, settingsFilename)
	if st.SettingsPath != wantPath {
		t.Fatalf("SettingsPath = %q, want %q", st.SettingsPath, wantPath)
	}
	if st.Installed {
		t.Fatalf("Installed = true, want false (no settings.json)")
	}
}

func TestStatus_BaselineHasNoLockieEntries(t *testing.T) {
	a := newTestAgent(t)
	dir := filepath.Join(a.homeDir, configDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	baseline, err := os.ReadFile("../../../test/fixtures/settings/claudecode_baseline.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, settingsFilename), baseline, 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	st, err := a.Status(agent.ScopeUser)
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if st.Installed {
		t.Fatalf("Installed = true, want false (baseline has no Lockie entries)")
	}
	if len(st.InstalledFor) != 0 {
		t.Fatalf("InstalledFor = %v, want empty", st.InstalledFor)
	}
}

func TestStatus_AfterInstall(t *testing.T) {
	a := newTestAgent(t)
	if err := a.Install(agent.InstallOptions{Scope: agent.ScopeUser}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	st, err := a.Status(agent.ScopeUser)
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if !st.Installed {
		t.Fatalf("Installed = false, want true after install")
	}
	if got := len(st.InstalledFor); got != len(agent.AllHooks()) {
		t.Fatalf("InstalledFor has %d hooks, want %d", got, len(agent.AllHooks()))
	}
}

func TestInstall_DryRunMatchesGolden(t *testing.T) {
	a := newTestAgent(t)
	var buf bytes.Buffer
	opts := agent.InstallOptions{
		Scope:        agent.ScopeUser,
		DryRun:       true,
		DryRunOutput: &buf,
	}
	if err := a.Install(opts); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	want, err := os.ReadFile("../../../test/fixtures/golden/dryrun/claudecode_install.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got := buf.String(); got != string(want) {
		t.Fatalf("dry-run output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSettingsPath(t *testing.T) {
	a := newTestAgent(t)
	cases := []struct {
		scope agent.Scope
		want  string
	}{
		{agent.ScopeUser, filepath.Join(a.homeDir, configDirName, settingsFilename)},
		{agent.ScopeProject, filepath.Join(a.projectDir, configDirName, settingsFilename)},
		{agent.ScopeProjectLocal, filepath.Join(a.projectDir, configDirName, "settings.local.json")},
	}
	for _, tc := range cases {
		got, err := a.settingsPath(tc.scope)
		if err != nil {
			t.Fatalf("settingsPath(%v) error: %v", tc.scope, err)
		}
		if got != tc.want {
			t.Errorf("settingsPath(%v) = %q, want %q", tc.scope, got, tc.want)
		}
	}
}
