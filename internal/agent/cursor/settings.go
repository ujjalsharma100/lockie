package cursor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

// loadHooksFile reads hooks.json into a generic map. A missing file
// is treated as an empty document so callers can write the first
// version.
func loadHooksFile(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("cursor: read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("cursor: parse %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// mergeInstall returns a copy of the hooks file with Lockie's enabled
// hook entries installed and a "version" field populated to
// hooksFileVersion if the file did not carry one already.
func mergeInstall(file map[string]any, enabled []agent.HookType) map[string]any {
	out := cloneTopLevel(file)
	if _, ok := out["version"]; !ok {
		out["version"] = hooksFileVersion
	}
	hooks := getOrCreateHooks(out)
	for nativeKey, lockieEntries := range buildLockiePlan(enabled) {
		existing := asSlice(hooks[nativeKey])
		merged := stripLockieEntries(existing)
		merged = append(merged, lockieEntries...)
		hooks[nativeKey] = merged
	}
	return out
}

// mergeUninstall returns a copy of the hooks file with every
// Lockie-managed entry removed from the managed hook keys. Empty hook
// keys are dropped; the "hooks" map is dropped only if Lockie owned
// every entry and no other user-authored hook keys remain.
func mergeUninstall(file map[string]any) map[string]any {
	out := cloneTopLevel(file)
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

// installedHooks reports which canonical hooks are wired up. We only
// need to check one native key per canonical hook because the install
// path always writes the full fan-out (postToolUse +
// postToolUseFailure) together.
func installedHooks(file map[string]any) []agent.HookType {
	hooks, _ := file["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	var out []agent.HookType
	for _, spec := range hookSpecs {
		key := spec.nativeKeys[0]
		entries := asSlice(hooks[key])
		for _, e := range entries {
			if isLockieManaged(e) {
				out = append(out, spec.canonical)
				break
			}
		}
	}
	return out
}

// renderHooksFile serializes the hooks-file map to indented JSON with
// a trailing newline. Dry-run and on-disk paths both call this so the
// output is byte-identical.
func renderHooksFile(file map[string]any) ([]byte, error) {
	b, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cursor: marshal hooks.json: %w", err)
	}
	return append(b, '\n'), nil
}

// writeHooksAtomic writes data to path via a sibling temp file and
// os.Rename so concurrent readers never observe a half-written file.
func writeHooksAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cursor: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".hooks.json.*.tmp")
	if err != nil {
		return fmt.Errorf("cursor: create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("cursor: write temp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("cursor: chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("cursor: close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("cursor: rename temp: %w", err)
	}
	return nil
}

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

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

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
