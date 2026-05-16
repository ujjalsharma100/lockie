// Package cursor is the Lockie Agent adapter for Cursor. It owns the
// wire format under ~/.cursor/hooks.json (or .cursor/hooks.json for
// project scope) and translates Cursor hook events into the canonical
// agent.Event type.
//
// Cross-reference: IMPLEMENTATION.md §6.2.
package cursor

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
	agentName     = "cursor"
	displayName   = "Cursor"
	configDirName = ".cursor"
	hooksFilename = "hooks.json"
)

var (
	errInstallNotImplemented   = errors.New("cursor: install not implemented (step 8.3b)")
	errUninstallNotImplemented = errors.New("cursor: uninstall not implemented (step 8.3b)")
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
// scope. Step 8.2 inspects only file presence — the real parser that
// reads `_lockie_managed` entries lands in step 8.3b.
func (a *Agent) Status(scope agent.Scope) (agent.Status, error) {
	path, err := a.hooksPath(scope)
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
	st.Warnings = append(st.Warnings, "hooks.json exists; install state inspection lands in step 8.3b")
	return st, nil
}

// Install wires Lockie's hooks into hooks.json. In step 8.2 only
// dry-run is supported: it writes the JSON it WOULD have written to
// opts.DryRunOutput (or os.Stdout) so callers can preview the merge.
// Real install (step 8.3b) merges into the existing file in-place.
func (a *Agent) Install(opts agent.InstallOptions) error {
	if !opts.DryRun {
		return errInstallNotImplemented
	}

	hooks := opts.EnabledHooks
	if len(hooks) == 0 {
		hooks = agent.AllHooks()
	}
	body, err := json.MarshalIndent(buildHooks(hooks), "", "  ")
	if err != nil {
		return fmt.Errorf("cursor: marshal dry-run hooks: %w", err)
	}

	w := opts.DryRunOutput
	if w == nil {
		w = os.Stdout
	}
	if _, err := io.WriteString(w, string(body)+"\n"); err != nil {
		return fmt.Errorf("cursor: write dry-run output: %w", err)
	}
	return nil
}

// Uninstall is a stub until step 8.3b.
func (a *Agent) Uninstall(_ agent.Scope) error {
	return errUninstallNotImplemented
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
