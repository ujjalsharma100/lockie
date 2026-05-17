package claudecode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

// loadSettings reads settings.json into a generic map. A missing file
// is treated as an empty document so callers can write the first
// version.
func loadSettings(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("claudecode: read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("claudecode: parse %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// mergeInstall returns a copy of settings with Lockie's enabled hook
// entries installed. For each managed hook key:
//   - any existing _lockie_managed entries are replaced (idempotent)
//   - user-authored entries are preserved in place
//
// Top-level user keys (e.g. "model") are untouched.
func mergeInstall(settings map[string]any, enabled []agent.HookType) map[string]any {
	out := cloneTopLevel(settings)
	hooks := getOrCreateHooks(out)
	for nativeKey, lockieEntries := range buildLockiePlan(enabled) {
		existing := asSlice(hooks[nativeKey])
		merged := stripLockieEntries(existing)
		merged = append(merged, lockieEntries...)
		hooks[nativeKey] = merged
	}
	return out
}

// mergeUninstall returns a copy of settings with every Lockie-managed
// entry removed from the managed hook keys. Hook keys that become
// empty are dropped; if the "hooks" map ends up empty, it is dropped
// as well.
func mergeUninstall(settings map[string]any) map[string]any {
	out := cloneTopLevel(settings)
	hooks, ok := out["hooks"].(map[string]any)
	if !ok {
		return out
	}
	for _, nativeKey := range nativeKeysOwned() {
		entries := asSlice(hooks[nativeKey])
		kept := stripLockieEntries(entries)
		if len(kept) == 0 {
			delete(hooks, nativeKey)
			continue
		}
		hooks[nativeKey] = kept
	}
	if len(hooks) == 0 {
		delete(out, "hooks")
	}
	return out
}

// installedHooks reports which canonical hooks have a Lockie-managed
// entry under their corresponding native key. The result is in
// canonical (hookSpecs) order.
func installedHooks(settings map[string]any) []agent.HookType {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	var out []agent.HookType
	for _, spec := range hookSpecs {
		entries := asSlice(hooks[spec.nativeKey])
		for _, e := range entries {
			if isLockieManaged(e) {
				out = append(out, spec.canonical)
				break
			}
		}
	}
	return out
}

// renderSettings serializes the settings map to indented JSON with a
// trailing newline. Both the dry-run path and the on-disk write path
// funnel through this so their outputs are byte-identical.
func renderSettings(settings map[string]any) ([]byte, error) {
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("claudecode: marshal settings: %w", err)
	}
	return append(b, '\n'), nil
}

// writeSettingsAtomic writes data to path via a sibling temp file and
// os.Rename so concurrent readers never observe a half-written file.
func writeSettingsAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("claudecode: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".settings.json.*.tmp")
	if err != nil {
		return fmt.Errorf("claudecode: create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("claudecode: write temp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("claudecode: chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("claudecode: close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("claudecode: rename temp: %w", err)
	}
	return nil
}

// cloneTopLevel returns a shallow copy of the top-level map plus a
// shallow copy of the nested "hooks" map (the only nested map the
// merger mutates). Per-entry slices are also cloned so the caller can
// safely append/filter without mutating the input.
func cloneTopLevel(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	if hooks, ok := out["hooks"].(map[string]any); ok {
		clone := make(map[string]any, len(hooks))
		for k, v := range hooks {
			if s, ok := v.([]any); ok {
				clone[k] = append([]any(nil), s...)
			} else {
				clone[k] = v
			}
		}
		out["hooks"] = clone
	}
	return out
}

func getOrCreateHooks(m map[string]any) map[string]any {
	if h, ok := m["hooks"].(map[string]any); ok {
		return h
	}
	h := map[string]any{}
	m["hooks"] = h
	return h
}

// asSlice returns the value as a []any. Returns nil for missing or
// wrong-typed values (treated as "no entries").
func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// stripLockieEntries returns a new slice containing only the entries
// that are NOT Lockie-managed.
func stripLockieEntries(in []any) []any {
	if len(in) == 0 {
		return nil
	}
	out := make([]any, 0, len(in))
	for _, e := range in {
		if !isLockieManaged(e) {
			out = append(out, e)
		}
	}
	return out
}
