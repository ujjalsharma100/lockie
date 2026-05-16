package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestInstallDryRun_ClaudeCode(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"install", "claude-code", "--dry-run"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	body := out.String()
	for _, want := range []string{
		`"_lockie_managed": true`,
		`"PostToolUse"`,
		`"UserPromptSubmit"`,
		`"lockie hook prompt"`,
		`"lockie hook post-tool"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dry-run output missing %q\nfull output:\n%s", want, body)
		}
	}
}

func TestInstallDryRun_Cursor(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"install", "cursor", "--dry-run"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	body := out.String()
	for _, want := range []string{
		`"version": 1`,
		`"sessionStart"`,
		`"postToolUseFailure"`,
		`"_lockie_managed": true`,
		`"matcher": ".*"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dry-run output missing %q\nfull output:\n%s", want, body)
		}
	}
}

func TestInstallUnknownAgent(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"install", "no-such-agent", "--dry-run"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown agent, got nil")
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("error = %v, want unknown agent", err)
	}
}

func TestInstallInvalidScope(t *testing.T) {
	root := NewRoot()
	root.SetArgs([]string{"install", "claude-code", "--scope", "global"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error for invalid scope, got nil")
	}
	if !strings.Contains(err.Error(), "invalid --scope") {
		t.Fatalf("error = %v, want invalid --scope", err)
	}
}
