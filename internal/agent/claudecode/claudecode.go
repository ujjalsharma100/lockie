// Package claudecode is the Lockie Agent adapter for Claude Code. It
// owns the wire format under ~/.claude/settings.json (or the project
// equivalent) and translates Claude Code hook events into the canonical
// agent.Event type.
//
// Cross-reference: IMPLEMENTATION.md §6.1.
package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

const (
	agentName        = "claude-code"
	displayName      = "Claude Code"
	settingsFilename = "settings.json"
)

// configDirName is the per-scope subdirectory Claude Code uses.
// Both user and project scopes use ".claude"; only the parent differs.
const configDirName = ".claude"

// errInstallNotImplemented is returned from the non-dry-run install
// path until step 8.3 wires up the real settings.json merger.
var errInstallNotImplemented = errors.New("claudecode: install not implemented (step 8.3)")

// errUninstallNotImplemented is returned from Uninstall until the
// real settings merger lands.
var errUninstallNotImplemented = errors.New("claudecode: uninstall not implemented (step 8.3)")

// Agent implements agent.Agent for Claude Code. It is constructed
// once and registered via init().
type Agent struct {
	// homeDir overrides the user-home lookup. Empty means use the
	// OS-reported home; tests set this to a tmpdir.
	homeDir string
	// projectDir overrides the project root used for project-scope
	// paths. Empty means use the current working directory.
	projectDir string
	// lookPath finds an executable on PATH; defaults to exec.LookPath.
	// Override in tests to fake binary presence.
	lookPath func(string) (string, error)
}

// New returns a Claude Code adapter using the real environment.
func New() *Agent {
	return &Agent{lookPath: exec.LookPath}
}

func init() {
	agent.Register(New())
}

// Name returns the canonical CLI name ("claude-code").
func (a *Agent) Name() string { return agentName }

// DisplayName returns the human-facing name ("Claude Code").
func (a *Agent) DisplayName() string { return displayName }

// SupportedHooks returns the canonical hooks Claude Code can serve.
// Today that is every hook in agent.AllHooks() — see §6.1.
func (a *Agent) SupportedHooks() []agent.HookType { return agent.AllHooks() }

// Detect reports whether Claude Code looks installed on this machine.
// It considers the agent present if either the user config dir
// (~/.claude) exists or a `claude` binary is on PATH.
func (a *Agent) Detect() (agent.DetectResult, error) {
	home, err := a.home()
	if err != nil {
		return agent.DetectResult{}, err
	}
	configDir := filepath.Join(home, configDirName)

	var binPath string
	if a.lookPath != nil {
		if p, err := a.lookPath("claude"); err == nil {
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
// scope. Step 8.2 inspects only file presence — the real parser that
// reads `_lockie_managed` entries lands in step 8.3.
func (a *Agent) Status(scope agent.Scope) (agent.Status, error) {
	path, err := a.settingsPath(scope)
	if err != nil {
		return agent.Status{}, err
	}
	st := agent.Status{SettingsPath: path}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	// File exists but step 8.2 cannot yet tell whether Lockie owns it;
	// the real status check (step 8.3) parses _lockie_managed entries.
	st.Warnings = append(st.Warnings, "settings.json exists; install state inspection lands in step 8.3")
	return st, nil
}

// Install wires Lockie's hooks into the appropriate settings.json.
// In step 8.2 the only supported mode is dry-run: it writes the JSON
// it WOULD have written to opts.DryRunOutput (or os.Stdout) so callers
// can preview the merge. Real install (step 8.3) merges into the
// existing file in-place.
func (a *Agent) Install(opts agent.InstallOptions) error {
	if !opts.DryRun {
		return errInstallNotImplemented
	}

	hooks := opts.EnabledHooks
	if len(hooks) == 0 {
		hooks = agent.AllHooks()
	}
	body, err := json.MarshalIndent(buildSettings(hooks), "", "  ")
	if err != nil {
		return fmt.Errorf("claudecode: marshal dry-run settings: %w", err)
	}

	w := opts.DryRunOutput
	if w == nil {
		w = os.Stdout
	}
	if _, err := io.WriteString(w, string(body)+"\n"); err != nil {
		return fmt.Errorf("claudecode: write dry-run output: %w", err)
	}
	return nil
}

// Uninstall is a stub until step 8.3.
func (a *Agent) Uninstall(_ agent.Scope) error {
	return errUninstallNotImplemented
}

// DecodeEvent translates a Claude Code hook event JSON into the
// canonical agent.Event. Stub until step 8.7.
func (a *Agent) DecodeEvent(raw []byte, hook agent.HookType) (*agent.Event, error) {
	return decodeEvent(raw, hook)
}

// EncodeResponse translates the canonical agent.Response into the
// Claude Code wire format. Stub until step 8.7.
func (a *Agent) EncodeResponse(resp *agent.Response, hook agent.HookType) ([]byte, error) {
	return encodeResponse(resp, hook)
}

// home returns the user home directory, honoring the test override.
func (a *Agent) home() (string, error) {
	if a.homeDir != "" {
		return a.homeDir, nil
	}
	return os.UserHomeDir()
}

// project returns the project root used for project-scope paths.
func (a *Agent) project() (string, error) {
	if a.projectDir != "" {
		return a.projectDir, nil
	}
	return os.Getwd()
}

// settingsPath returns the on-disk path to the settings.json file
// owned by the given scope.
func (a *Agent) settingsPath(scope agent.Scope) (string, error) {
	switch scope {
	case agent.ScopeUser:
		home, err := a.home()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, configDirName, settingsFilename), nil
	case agent.ScopeProject:
		proj, err := a.project()
		if err != nil {
			return "", err
		}
		return filepath.Join(proj, configDirName, settingsFilename), nil
	case agent.ScopeProjectLocal:
		proj, err := a.project()
		if err != nil {
			return "", err
		}
		return filepath.Join(proj, configDirName, "settings.local.json"), nil
	default:
		return "", fmt.Errorf("claudecode: unknown scope %v", scope)
	}
}
