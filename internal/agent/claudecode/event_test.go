package claudecode

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ujjalsharma100/lockie/internal/agent"
	"github.com/ujjalsharma100/lockie/internal/testutil"
)

const testStripeKey = testutil.StripeSecretKey

func TestDecodeEvent_PostToolUse_ReadStringResponse(t *testing.T) {
	raw := []byte(`{
		"session_id": "s1",
		"cwd": "/tmp",
		"hook_event_name": "PostToolUse",
		"tool_name": "Read",
		"tool_input": {"file_path": "/tmp/.env"},
		"tool_response": "STRIPE_SECRET_KEY=` + testStripeKey + `\n"
	}`)
	ev, err := decodeEvent(raw, agent.HookPostToolUse)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Tool != "Read" || ev.SessionID != "s1" {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if ev.Output == nil || !strings.Contains(ev.Output.Content, testStripeKey) {
		t.Fatalf("Output.Content = %+v", ev.Output)
	}
	if ev.Input[metaToolResponseKind] != "string" {
		t.Fatalf("meta kind = %v", ev.Input[metaToolResponseKind])
	}
}

func TestDecodeEvent_PreToolUse(t *testing.T) {
	raw := []byte(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"echo hi"}}`)
	ev, err := decodeEvent(raw, agent.HookPreToolUse)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Tool != "Bash" || ev.Input["command"] != "echo hi" {
		t.Fatalf("unexpected: %+v", ev)
	}
}

func TestEncodeResponse_PostToolUse_StringReplacement(t *testing.T) {
	ev := &agent.Event{
		Input: map[string]any{
			metaToolResponseKind: "string",
			metaToolResponseRaw:  "literal\n",
		},
	}
	resp := ResponseFromPostTool(ev, true, &agent.ToolOutput{Content: "STRIPE_KEY_1\n"})
	body, err := encodeResponse(resp, agent.HookPostToolUse)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	hs, ok := m["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %v", m)
	}
	if hs["hookEventName"] != "PostToolUse" {
		t.Fatalf("hookEventName = %v", hs["hookEventName"])
	}
	updated, ok := hs["updatedToolOutput"].(string)
	if !ok || !strings.Contains(updated, "STRIPE_KEY_1") {
		t.Fatalf("updatedToolOutput = %v", hs["updatedToolOutput"])
	}
}

func TestEncodeResponse_PreToolUse_RehydratedInput(t *testing.T) {
	input := map[string]any{"command": "curl " + testStripeKey}
	body, err := encodeResponse(&agent.Response{
		Modified:      true,
		ModifiedInput: input,
	}, agent.HookPreToolUse)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	hs := m["hookSpecificOutput"].(map[string]any)
	ui := hs["updatedInput"].(map[string]any)
	if ui["command"] != "curl "+testStripeKey {
		t.Fatalf("updatedInput = %v", ui)
	}
}

func TestEncodeResponse_PromptUpdated(t *testing.T) {
	body, err := encodeResponse(&agent.Response{
		Modified:     true,
		ModifiedText: "use STRIPE_KEY_1",
	}, agent.HookPromptSubmit)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(body), "updatedPrompt") {
		t.Fatalf("body = %s", body)
	}
}

func TestEncodeResponse_BlockPreTool(t *testing.T) {
	body, err := encodeResponse(&agent.Response{
		Block:       true,
		BlockReason: "nope",
	}, agent.HookPreToolUse)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(body), "deny") {
		t.Fatalf("body = %s", body)
	}
}
