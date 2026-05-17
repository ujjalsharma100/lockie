// Package config resolves Lockie data-directory paths.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// UserDataDir returns the Lockie state directory (~/.lockie by default).
// LOCKIE_DATA_DIR overrides the base path (used by tests).
func UserDataDir() (string, error) {
	if d := os.Getenv("LOCKIE_DATA_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: home dir: %w", err)
	}
	return filepath.Join(home, ".lockie"), nil
}

// AliasesPath returns the Phase 1 durable alias file
// (~/.lockie/aliases.json). Literals are stored in plaintext with
// 0600 permissions until Phase 2 moves values to the OS keychain.
func AliasesPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "aliases.json"), nil
}

// AuditPath returns the append-only substitution audit log
// (~/.lockie/audit.log). Events record placeholder names and rule ids
// only — never literals.
func AuditPath() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "audit.log"), nil
}

// EnsureUserDataDir creates ~/.lockie (or LOCKIE_DATA_DIR) with 0700.
func EnsureUserDataDir() (string, error) {
	dir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("config: mkdir %s: %w", dir, err)
	}
	return dir, nil
}
