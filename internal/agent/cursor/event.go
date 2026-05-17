package cursor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

// Cursor wire format notes (IMPLEMENTATION.md §6.0 / §6.2):
//   - Hook keys are camelCase: `beforeSubmitPrompt`, `preToolUse`,
//     `postToolUse`, `postToolUseFailure`, `sessionStart`, `sessionEnd`.
//   - Cursor names the shell tool `Shell`; the canonical name is `Bash`.
//     Other built-ins (`Read`, `Write`, `Edit`, `Grep`, `Glob`) match
//     across agents.
//   - The response envelope is one of:
//       {"permission": "deny", "reason": "..."}            // block
//       {"continue": true, "modifiedPrompt": "..."}        // beforeSubmitPrompt rewrite
//       {"continue": true, "modifiedInput": {...}}         // preToolUse rewrite
//       {"continue": true, "modifiedOutput": {...}}        // postToolUse inline replace
//       {"continue": true}                                 // ack only
//   - The Phase 0 spike (§7.2) validates the exact field names Cursor
//     accepts for inline replacement of built-in tool output vs. the
//     additive `additional_context` channel; until that is settled,
//     `postToolUse` emits an inline replacement under `modifiedOutput`.

// decodeEvent translates a Cursor hook event JSON into the canonical
// agent.Event. Unknown fields in the source JSON are preserved on the
// canonical event's Input map so step 8.7's rehydrator can echo them
// back unchanged.
func decodeEvent(raw []byte, hook agent.HookType) (*agent.Event, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("cursor: decode event: %w", err)
	}
	ev := &agent.Event{
		Hook:      hook,
		AgentName: agentName,
		SessionID: stringField(m, "sessionId"),
		CWD:       stringField(m, "cwd"),
		Timestamp: time.Now().UTC(),
	}
	switch hook {
	case agent.HookPromptSubmit:
		ev.Prompt = stringField(m, "prompt")
	case agent.HookPreToolUse:
		ev.Tool = canonicalTool(stringField(m, "toolName"))
		ev.Input = mapField(m, "toolInput")
	case agent.HookPostToolUse:
		ev.Tool = canonicalTool(stringField(m, "toolName"))
		ev.Input = mapField(m, "toolInput")
		ev.Output = decodeToolOutput(m["toolOutput"])
	case agent.HookSessionStart, agent.HookSessionStop:
		// no payload fields beyond the common ones
	default:
		return nil, fmt.Errorf("cursor: unsupported hook %q", hook)
	}
	return ev, nil
}

// encodeResponse translates the canonical agent.Response into the
// Cursor wire format.
func encodeResponse(resp *agent.Response, hook agent.HookType) ([]byte, error) {
	if resp == nil {
		return []byte(`{"continue":true}`), nil
	}
	if resp.Block {
		return json.Marshal(map[string]any{
			"permission": "deny",
			"reason":     resp.BlockReason,
		})
	}
	out := map[string]any{"continue": true}
	if !resp.Modified {
		return json.Marshal(out)
	}
	switch hook {
	case agent.HookPromptSubmit:
		out["modifiedPrompt"] = resp.ModifiedText
	case agent.HookPreToolUse:
		if resp.ModifiedInput != nil {
			out["modifiedInput"] = resp.ModifiedInput
		}
	case agent.HookPostToolUse:
		// Inline replacement of the tool output. If the Phase 0 spike
		// (§7.2) shows Cursor will not honor inline replacement for
		// built-in tools, the daemon can switch to additive mode by
		// setting Response.ModifiedText empty and supplying the
		// redacted text via a separate channel (TODO once confirmed).
		out["modifiedOutput"] = map[string]any{
			"content": resp.ModifiedText,
		}
	case agent.HookSessionStart, agent.HookSessionStop:
		// no body beyond the ack
	default:
		return nil, fmt.Errorf("cursor: unsupported hook %q", hook)
	}
	return json.Marshal(out)
}

// canonicalTool maps a Cursor tool name to the canonical name used in
// agent.Event. Only `Shell` differs; everything else passes through.
func canonicalTool(name string) string {
	if name == "Shell" {
		return "Bash"
	}
	return name
}

func decodeToolOutput(v any) *agent.ToolOutput {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := &agent.ToolOutput{
		Stdout:  stringField(m, "stdout"),
		Stderr:  stringField(m, "stderr"),
		Content: stringField(m, "content"),
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
