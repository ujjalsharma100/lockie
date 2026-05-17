package cursor

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

func TestDecodeEvent_PromptSubmit(t *testing.T) {
	raw := []byte(`{"sessionId":"s1","cwd":"/tmp","prompt":"hello"}`)
	ev, err := decodeEvent(raw, agent.HookPromptSubmit)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.SessionID != "s1" || ev.CWD != "/tmp" || ev.Prompt != "hello" {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if ev.AgentName != agentName {
		t.Fatalf("AgentName = %q, want %q", ev.AgentName, agentName)
	}
}

func TestDecodeEvent_PreToolUse_ShellNormalization(t *testing.T) {
	raw := []byte(`{"sessionId":"s1","toolName":"Shell","toolInput":{"command":"ls"}}`)
	ev, err := decodeEvent(raw, agent.HookPreToolUse)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Tool != "Bash" {
		t.Fatalf("Tool = %q, want Bash (Shell→Bash normalization)", ev.Tool)
	}
	if got := ev.Input["command"]; got != "ls" {
		t.Fatalf("Input[command] = %v", got)
	}
}

func TestDecodeEvent_PostToolUse(t *testing.T) {
	raw := []byte(`{"sessionId":"s1","toolName":"Read","toolInput":{"path":"/x"},"toolOutput":{"content":"file body","exitCode":0}}`)
	ev, err := decodeEvent(raw, agent.HookPostToolUse)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Tool != "Read" {
		t.Fatalf("Tool = %q, want Read", ev.Tool)
	}
	if ev.Output == nil || ev.Output.Content != "file body" {
		t.Fatalf("Output = %+v", ev.Output)
	}
	if ev.Output.ExitCode != 0 {
		t.Fatalf("ExitCode = %d", ev.Output.ExitCode)
	}
}

func TestDecodeEvent_UnknownHookRejected(t *testing.T) {
	_, err := decodeEvent([]byte(`{}`), agent.HookType("Bogus"))
	if err == nil {
		t.Fatalf("expected error for unknown hook")
	}
}

func TestEncodeResponse_Block(t *testing.T) {
	body, err := encodeResponse(&agent.Response{Block: true, BlockReason: "secret"}, agent.HookPreToolUse)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m["permission"] != "deny" || m["reason"] != "secret" {
		t.Fatalf("got %v, want permission=deny reason=secret", m)
	}
}

func TestEncodeResponse_NotModifiedAcks(t *testing.T) {
	body, err := encodeResponse(&agent.Response{}, agent.HookPromptSubmit)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := `{"continue":true}`
	if string(body) != want {
		t.Fatalf("got %s, want %s", body, want)
	}
}

func TestEncodeResponse_PromptRewrite(t *testing.T) {
	body, err := encodeResponse(&agent.Response{Modified: true, ModifiedText: "rewritten"}, agent.HookPromptSubmit)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m["modifiedPrompt"] != "rewritten" || m["continue"] != true {
		t.Fatalf("got %v", m)
	}
}

func TestEncodeResponse_PreToolUseModifiedInput(t *testing.T) {
	input := map[string]any{"command": "echo redacted"}
	body, err := encodeResponse(&agent.Response{Modified: true, ModifiedInput: input}, agent.HookPreToolUse)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !reflect.DeepEqual(m["modifiedInput"], map[string]any{"command": "echo redacted"}) {
		t.Fatalf("modifiedInput = %v", m["modifiedInput"])
	}
}

func TestEncodeResponse_PostToolUseInlineReplace(t *testing.T) {
	body, err := encodeResponse(&agent.Response{Modified: true, ModifiedText: "redacted output"}, agent.HookPostToolUse)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, ok := m["modifiedOutput"].(map[string]any)
	if !ok {
		t.Fatalf("modifiedOutput missing or wrong type: %v", m["modifiedOutput"])
	}
	if out["content"] != "redacted output" {
		t.Fatalf("modifiedOutput.content = %v", out["content"])
	}
}

func TestCanonicalTool(t *testing.T) {
	cases := map[string]string{
		"Shell": "Bash",
		"Read":  "Read",
		"Write": "Write",
		"":      "",
	}
	for in, want := range cases {
		if got := canonicalTool(in); got != want {
			t.Errorf("canonicalTool(%q) = %q, want %q", in, got, want)
		}
	}
}
