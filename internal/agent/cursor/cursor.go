// Package cursor is the Lockie Agent adapter for Cursor. It owns the
// wire format under ~/.cursor/hooks.json (or .cursor/hooks.json for
// project scope) and translates Cursor hook events into the canonical
// agent.Event type.
//
// Cross-reference: IMPLEMENTATION.md §6.2.
package cursor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

const (
	agentName     = "cursor"
	displayName   = "Cursor"
	configDirName = ".cursor"
	hooksFilename = "hooks.json"
)

// Agent implements agent.Agent for Cursor.
type Agent struct {
	homeDir    string
	projectDir string
	lookPath   func(string) (string, error)
}

// New returns a Cursor adapter using the real environment.
func New() *Agent {
	return &Agent{lookPath: exec.LookPath}
}

func init() {
	agent.Register(New())
}

// Name returns the canonical CLI name ("cursor").
func (a *Agent) Name() string { return agentName }

// DisplayName returns the human-facing name ("Cursor").
func (a *Agent) DisplayName() string { return displayName }

// SupportedHooks returns the canonical hooks Cursor can serve. Cursor
// exposes generic preToolUse/postToolUse hooks that match every tool
// type via matcher ".*" (§6.0).
func (a *Agent) SupportedHooks() []agent.HookType { return agent.AllHooks() }

// Detect reports whether Cursor looks installed on this machine. It
// considers the agent present if either the user config dir
// (~/.cursor) exists or a `cursor` binary is on PATH.
func (a *Agent) Detect() (agent.DetectResult, error) {
	home, err := a.home()
	if err != nil {
		return agent.DetectResult{}, err
	}
	configDir := filepath.Join(home, configDirName)

	var binPath string
	if a.lookPath != nil {
		if p, err := a.lookPath("cursor"); err == nil {
			binPath = p
		}
	}

	configExists := false
	if info, err := os.Stat(configDir); err == nil && info.IsDir() {
		configExists = true
	}

	res := agent.DetectResult{
		Installed:  configExists || binPath != "",
		BinaryPath: binPath,
	}
	if configExists {
		res.ConfigDir = configDir
	}
	return res, nil
}

// Status reports the current Lockie integration state for the given
// scope. It loads hooks.json (if present) and inspects which
// canonical hooks have a `_lockie_managed: true` entry.
func (a *Agent) Status(scope agent.Scope) (agent.Status, error) {
	path, err := a.hooksPath(scope)
	if err != nil {
		return agent.Status{}, err
	}
	st := agent.Status{SettingsPath: path}
	file, err := loadHooksFile(path)
	if err != nil {
		return st, err
	}
	st.InstalledFor = installedHooks(file)
	st.Installed = len(st.InstalledFor) > 0
	return st, nil
}

// Install merges Lockie's hook entries into the hooks.json owned by
// opts.Scope. With DryRun, the merged document is written to
// opts.DryRunOutput (or os.Stdout) instead of disk.
//
// Install is idempotent and preserves user-authored entries.
func (a *Agent) Install(opts agent.InstallOptions) error {
	hooks := opts.EnabledHooks
	if len(hooks) == 0 {
		hooks = agent.AllHooks()
	}
	path, err := a.hooksPath(opts.Scope)
	if err != nil {
		return err
	}
	current, err := loadHooksFile(path)
	if err != nil {
		return err
	}
	merged := mergeInstall(current, hooks)
	body, err := renderHooksFile(merged)
	if err != nil {
		return err
	}
	if opts.DryRun {
		w := opts.DryRunOutput
		if w == nil {
			w = os.Stdout
		}
		if _, err := w.Write(body); err != nil {
			return fmt.Errorf("cursor: write dry-run output: %w", err)
		}
		return nil
	}
	return writeHooksAtomic(path, body)
}

// Uninstall removes every Lockie-managed entry from the hooks.json
// owned by scope. If the user has no other top-level keys left
// (i.e. the file was Lockie-only), the file is removed entirely so
// the directory looks untouched; otherwise the remaining keys
// (including a user-authored "version") are written back.
func (a *Agent) Uninstall(scope agent.Scope) error {
	path, err := a.hooksPath(scope)
	if err != nil {
		return err
	}
	current, err := loadHooksFile(path)
	if err != nil {
		return err
	}
	cleaned := mergeUninstall(current)
	if len(cleaned) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cursor: remove %s: %w", path, err)
		}
		return nil
	}
	body, err := renderHooksFile(cleaned)
	if err != nil {
		return err
	}
	return writeHooksAtomic(path, body)
}

// DecodeEvent translates a Cursor hook event JSON into the canonical
// agent.Event. Stub until step 8.7.
func (a *Agent) DecodeEvent(raw []byte, hook agent.HookType) (*agent.Event, error) {
	return decodeEvent(raw, hook)
}

// EncodeResponse translates the canonical agent.Response into the
// Cursor wire format. Stub until step 8.7.
func (a *Agent) EncodeResponse(resp *agent.Response, hook agent.HookType) ([]byte, error) {
	return encodeResponse(resp, hook)
}

func (a *Agent) home() (string, error) {
	if a.homeDir != "" {
		return a.homeDir, nil
	}
	return os.UserHomeDir()
}

func (a *Agent) project() (string, error) {
	if a.projectDir != "" {
		return a.projectDir, nil
	}
	return os.Getwd()
}

// hooksPath returns the on-disk path to the hooks.json file owned by
// the given scope. Cursor uses .cursor/hooks.json for both project and
// project-local; the latter just lives next to the project file with
// a "-local" suffix is NOT in current Cursor docs, so we point both
// project and project-local at the same path until upstream
// disambiguates (§6.2 scope precedence).
func (a *Agent) hooksPath(scope agent.Scope) (string, error) {
	switch scope {
	case agent.ScopeUser:
		home, err := a.home()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, configDirName, hooksFilename), nil
	case agent.ScopeProject, agent.ScopeProjectLocal:
		proj, err := a.project()
		if err != nil {
			return "", err
		}
		return filepath.Join(proj, configDirName, hooksFilename), nil
	default:
		return "", fmt.Errorf("cursor: unknown scope %v", scope)
	}
}
