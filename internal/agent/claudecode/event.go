package claudecode

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

// Claude Code wire format (IMPLEMENTATION.md §6.1, code.claude.com hooks docs):
//   - Hook input uses snake_case: session_id, hook_event_name, tool_name,
//     tool_input, tool_response.
//   - PreToolUse decisions live in hookSpecificOutput.permissionDecision /
//     updatedInput; PostToolUse rewrites use hookSpecificOutput.updatedToolOutput.
//   - UserPromptSubmit uses top-level decision/reason to block; there is no
//     documented prompt-replacement field, so a modified prompt is emitted as
//     hookSpecificOutput.updatedPrompt following the updatedInput naming pattern.

const (
	metaToolResponseRaw  = "__lockie_tool_response_raw"
	metaToolResponseKind = "__lockie_tool_response_kind"
	metaRedactedOutput   = "__lockie_redacted_output"
)

// ResponseFromPostTool builds the canonical Response for a PostToolUse hook
// after the daemon has redacted tool output. The original tool_response shape
// is preserved via metadata stashed on Event.Input during decode.
func ResponseFromPostTool(ev *agent.Event, modified bool, out *agent.ToolOutput) *agent.Response {
	resp := &agent.Response{Modified: modified}
	if !modified || out == nil {
		return resp
	}
	resp.ModifiedInput = map[string]any{metaRedactedOutput: toolOutputMap(out)}
	if ev != nil && ev.Input != nil {
		if k, ok := ev.Input[metaToolResponseKind]; ok {
			resp.ModifiedInput[metaToolResponseKind] = k
		}
		if raw, ok := ev.Input[metaToolResponseRaw]; ok {
			resp.ModifiedInput[metaToolResponseRaw] = raw
		}
	}
	resp.ModifiedText = primaryOutputText(out)
	return resp
}

// decodeEvent translates a Claude Code hook event JSON into the canonical
// agent.Event.
func decodeEvent(raw []byte, hook agent.HookType) (*agent.Event, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("claudecode: decode event: %w", err)
	}
	ev := &agent.Event{
		Hook:      hook,
		AgentName: agentName,
		SessionID: stringField(m, "session_id"),
		CWD:       stringField(m, "cwd"),
		Timestamp: time.Now().UTC(),
	}
	switch hook {
	case agent.HookPromptSubmit:
		ev.Prompt = stringField(m, "prompt")
	case agent.HookPreToolUse:
		ev.Tool = stringField(m, "tool_name")
		ev.Input = mapField(m, "tool_input")
	case agent.HookPostToolUse:
		ev.Tool = stringField(m, "tool_name")
		ev.Input = mapField(m, "tool_input")
		decodeToolResponse(m["tool_response"], ev)
	case agent.HookSessionStart, agent.HookSessionStop:
		// session_id + cwd only
	default:
		return nil, fmt.Errorf("claudecode: unsupported hook %q", hook)
	}
	return ev, nil
}

// encodeResponse translates the canonical agent.Response into the Claude Code
// wire format expected on stdout.
func encodeResponse(resp *agent.Response, hook agent.HookType) ([]byte, error) {
	if resp == nil {
		return []byte("{}"), nil
	}
	if resp.Block {
		return encodeBlock(resp, hook)
	}
	if !resp.Modified {
		return []byte("{}"), nil
	}
	switch hook {
	case agent.HookPromptSubmit:
		return json.Marshal(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName": "UserPromptSubmit",
				"updatedPrompt": resp.ModifiedText,
			},
		})
	case agent.HookPreToolUse:
		out := map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     "PreToolUse",
				"permissionDecision": "allow",
			},
		}
		if resp.ModifiedInput != nil {
			out["hookSpecificOutput"].(map[string]any)["updatedInput"] = resp.ModifiedInput
		}
		return json.Marshal(out)
	case agent.HookPostToolUse:
		updated, err := encodeUpdatedToolOutput(resp)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     "PostToolUse",
				"updatedToolOutput": updated,
			},
		})
	case agent.HookSessionStart, agent.HookSessionStop:
		return []byte("{}"), nil
	default:
		return nil, fmt.Errorf("claudecode: unsupported hook %q", hook)
	}
}

func encodeBlock(resp *agent.Response, hook agent.HookType) ([]byte, error) {
	switch hook {
	case agent.HookPreToolUse:
		return json.Marshal(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": resp.BlockReason,
			},
		})
	case agent.HookPromptSubmit, agent.HookPostToolUse:
		return json.Marshal(map[string]any{
			"decision": "block",
			"reason":   resp.BlockReason,
		})
	default:
		return []byte("{}"), nil
	}
}

// encodeUpdatedToolOutput rebuilds Claude's updatedToolOutput value. When the
// original tool_response was a string (typical for Read/Grep content), the
// replacement is the redacted string in ModifiedText. Otherwise we merge
// redacted canonical fields back into a copy of the original object.
func encodeUpdatedToolOutput(resp *agent.Response) (any, error) {
	if resp.ModifiedInput == nil {
		if resp.ModifiedText != "" {
			return resp.ModifiedText, nil
		}
		return map[string]any{}, nil
	}
	kind, _ := resp.ModifiedInput[metaToolResponseKind].(string)
	redacted, _ := resp.ModifiedInput[metaRedactedOutput].(map[string]any)
	switch kind {
	case "string":
		if t := stringField(redacted, "content"); t != "" {
			return t, nil
		}
		return resp.ModifiedText, nil
	case "object":
		raw, ok := resp.ModifiedInput[metaToolResponseRaw].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("claudecode: missing preserved tool_response object")
		}
		return mergeToolOutput(cloneMap(raw), redacted), nil
	default:
		if resp.ModifiedText != "" {
			return resp.ModifiedText, nil
		}
		return map[string]any{}, nil
	}
}

func mergeToolOutput(dst, redacted map[string]any) map[string]any {
	for _, pair := range []struct{ srcKey, dstKey string }{
		{"stdout", "stdout"},
		{"stderr", "stderr"},
		{"content", "content"},
		{"content", "fileContent"},
		{"diff", "diff"},
	} {
		if v := stringField(redacted, pair.srcKey); v != "" {
			if _, ok := dst[pair.dstKey]; ok || pair.dstKey == "stdout" || pair.dstKey == "content" {
				dst[pair.dstKey] = v
			}
		}
	}
	return dst
}

func toolOutputMap(out *agent.ToolOutput) map[string]any {
	if out == nil {
		return nil
	}
	m := map[string]any{}
	if out.Stdout != "" {
		m["stdout"] = out.Stdout
	}
	if out.Stderr != "" {
		m["stderr"] = out.Stderr
	}
	if out.Content != "" {
		m["content"] = out.Content
	}
	if out.Diff != "" {
		m["diff"] = out.Diff
	}
	return m
}

func primaryOutputText(out *agent.ToolOutput) string {
	if out == nil {
		return ""
	}
	return firstNonEmpty(out.Content, out.Stdout, out.Stderr, out.Diff)
}

func decodeToolResponse(v any, ev *agent.Event) {
	if v == nil {
		return
	}
	if ev.Input == nil {
		ev.Input = make(map[string]any)
	}
	switch t := v.(type) {
	case string:
		ev.Output = &agent.ToolOutput{Content: t}
		ev.Input[metaToolResponseKind] = "string"
		ev.Input[metaToolResponseRaw] = t
	case map[string]any:
		ev.Output = decodeToolOutputObject(t)
		ev.Input[metaToolResponseKind] = "object"
		ev.Input[metaToolResponseRaw] = t
	default:
		// Best-effort: re-marshal unknown shapes through JSON.
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			ev.Output = decodeToolOutputObject(m)
			ev.Input[metaToolResponseKind] = "object"
			ev.Input[metaToolResponseRaw] = m
		}
	}
}

func decodeToolOutputObject(m map[string]any) *agent.ToolOutput {
	out := &agent.ToolOutput{
		Stdout:  stringField(m, "stdout"),
		Stderr:  stringField(m, "stderr"),
		Content: firstNonEmpty(stringField(m, "content"), stringField(m, "fileContent")),
		Diff:    stringField(m, "diff"),
	}
	if ec, ok := m["exitCode"]; ok {
		switch n := ec.(type) {
		case float64:
			out.ExitCode = int(n)
		case int:
			out.ExitCode = n
		}
	}
	return out
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func mapField(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
