package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ujjalsharma100/lockie/internal/agent"
	"github.com/ujjalsharma100/lockie/internal/agent/claudecode"
	"github.com/ujjalsharma100/lockie/internal/daemon"
)

// AgentEnv optionally pins which agent adapter decodes stdin/stdout. When
// unset, the wire JSON is inspected (hook_event_name → claude-code,
// sessionId → cursor).
const AgentEnv = "LOCKIE_AGENT"

func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "hook",
		Short:  "Agent hook entrypoints (invoked by Claude Code / Cursor)",
		Hidden: true,
	}
	cmd.AddCommand(
		newHookSubCmd("prompt", agent.HookPromptSubmit, runHookPrompt),
		newHookSubCmd("pre-tool", agent.HookPreToolUse, runHookPreTool),
		newHookSubCmd("post-tool", agent.HookPostToolUse, runHookPostTool),
		newHookSubCmd("session-start", agent.HookSessionStart, runHookSessionStart),
		newHookSubCmd("session-stop", agent.HookSessionStop, runHookSessionStop),
	)
	return cmd
}

func newHookSubCmd(use string, hook agent.HookType, run func(context.Context, *cobra.Command, agent.Agent, []byte) (*agent.Response, error)) *cobra.Command {
	var socket string
	cmd := &cobra.Command{
		Use:   use,
		Short: fmt.Sprintf("Handle %s hook events", hook),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("hook: read stdin: %w", err)
			}
			ag, err := resolveAgent(raw)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmdContext(cmd), 5*time.Second)
			defer cancel()
			resp, err := run(ctx, cmd, ag, raw)
			if err != nil {
				return err
			}
			out, err := ag.EncodeResponse(resp, hook)
			if err != nil {
				return err
			}
			if len(out) == 0 {
				out = []byte("{}")
			}
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
	cmd.Flags().StringVar(&socket, "socket", "", "Override the daemon socket path")
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		// Stash resolved socket on the command context for the run func.
		path, err := resolveSocket(socket)
		if err != nil {
			return err
		}
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		cmd.SetContext(context.WithValue(ctx, hookSocketKey{}, path))
		return nil
	}
	return cmd
}

type hookSocketKey struct{}

func hookSocket(cmd *cobra.Command) string {
	if v, ok := cmd.Context().Value(hookSocketKey{}).(string); ok && v != "" {
		return v
	}
	path, _ := resolveSocket("")
	return path
}

func hookClient(cmd *cobra.Command) (*daemon.Client, error) {
	opts := daemon.DefaultLaunchOptions(hookSocket(cmd))
	opts.Binary = os.Args[0]
	opts.Args = []string{"daemon", "start", "--foreground", "--socket", hookSocket(cmd)}
	ctx, cancel := context.WithTimeout(cmdContext(cmd), 2*time.Second)
	defer cancel()
	return daemon.EnsureRunning(ctx, opts)
}

func resolveAgent(raw []byte) (agent.Agent, error) {
	if name := os.Getenv(AgentEnv); name != "" {
		return agent.Get(name)
	}
	var peek map[string]json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &peek); err != nil {
			return nil, fmt.Errorf("hook: decode stdin for agent detection: %w", err)
		}
	}
	if _, ok := peek["hook_event_name"]; ok {
		return agent.Get("claude-code")
	}
	if _, ok := peek["sessionId"]; ok {
		return agent.Get("cursor")
	}
	if _, ok := peek["session_id"]; ok {
		return agent.Get("claude-code")
	}
	return nil, fmt.Errorf("hook: cannot detect agent from stdin (set %s)", AgentEnv)
}

func runHookSessionStart(ctx context.Context, cmd *cobra.Command, ag agent.Agent, raw []byte) (*agent.Response, error) {
	ev, err := ag.DecodeEvent(raw, agent.HookSessionStart)
	if err != nil {
		return nil, err
	}
	client, err := hookClient(cmd)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	_, err = client.SessionStart(ctx, daemon.SessionStartParams{
		SessionID: ev.SessionID,
		Agent:     ag.Name(),
		CWD:       ev.CWD,
	})
	if err != nil {
		return nil, err
	}
	return &agent.Response{}, nil
}

func runHookSessionStop(ctx context.Context, cmd *cobra.Command, ag agent.Agent, raw []byte) (*agent.Response, error) {
	ev, err := ag.DecodeEvent(raw, agent.HookSessionStop)
	if err != nil {
		return nil, err
	}
	if ev.SessionID == "" {
		return &agent.Response{}, nil
	}
	client, err := hookClient(cmd)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if err := client.SessionStop(ctx, daemon.SessionStopParams{SessionID: ev.SessionID}); err != nil {
		return nil, err
	}
	return &agent.Response{}, nil
}

func runHookPrompt(ctx context.Context, cmd *cobra.Command, ag agent.Agent, raw []byte) (*agent.Response, error) {
	ev, err := ag.DecodeEvent(raw, agent.HookPromptSubmit)
	if err != nil {
		return nil, err
	}
	client, err := hookClient(cmd)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	sid, err := ensureSession(ctx, client, ev.SessionID, ag.Name(), ev.CWD)
	if err != nil {
		return nil, err
	}
	r, err := client.HookPrompt(ctx, daemon.HookPromptParams{SessionID: sid, Prompt: ev.Prompt})
	if err != nil {
		return nil, err
	}
	return &agent.Response{Modified: r.Modified, ModifiedText: r.Prompt}, nil
}

func runHookPreTool(ctx context.Context, cmd *cobra.Command, ag agent.Agent, raw []byte) (*agent.Response, error) {
	ev, err := ag.DecodeEvent(raw, agent.HookPreToolUse)
	if err != nil {
		return nil, err
	}
	client, err := hookClient(cmd)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	sid, err := ensureSession(ctx, client, ev.SessionID, ag.Name(), ev.CWD)
	if err != nil {
		return nil, err
	}
	r, err := client.HookPreTool(ctx, daemon.HookPreToolParams{
		SessionID: sid,
		Tool:      ev.Tool,
		Input:     stripLockieMeta(ev.Input),
	})
	if err != nil {
		return nil, err
	}
	return &agent.Response{Modified: r.Modified, ModifiedInput: r.Input}, nil
}

func runHookPostTool(ctx context.Context, cmd *cobra.Command, ag agent.Agent, raw []byte) (*agent.Response, error) {
	ev, err := ag.DecodeEvent(raw, agent.HookPostToolUse)
	if err != nil {
		return nil, err
	}
	client, err := hookClient(cmd)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	sid, err := ensureSession(ctx, client, ev.SessionID, ag.Name(), ev.CWD)
	if err != nil {
		return nil, err
	}
	params := daemon.HookPostToolParams{SessionID: sid, Tool: ev.Tool}
	if ev.Output != nil {
		params.Output = daemon.HookPostToolOutput{
			Stdout:   ev.Output.Stdout,
			Stderr:   ev.Output.Stderr,
			ExitCode: ev.Output.ExitCode,
			Content:  ev.Output.Content,
			Diff:     ev.Output.Diff,
		}
	}
	r, err := client.HookPostTool(ctx, params)
	if err != nil {
		return nil, err
	}
	out := toolOutputFromDaemon(r.Output)
	if ag.Name() == "claude-code" {
		return claudecode.ResponseFromPostTool(ev, r.Modified, out), nil
	}
	resp := &agent.Response{Modified: r.Modified}
	if r.Modified {
		resp.ModifiedText = firstNonEmpty(out.Content, out.Stdout, out.Stderr, out.Diff)
	}
	return resp, nil
}

func toolOutputFromDaemon(o daemon.HookPostToolOutput) *agent.ToolOutput {
	return &agent.ToolOutput{
		Stdout:   o.Stdout,
		Stderr:   o.Stderr,
		ExitCode: o.ExitCode,
		Content:  o.Content,
		Diff:     o.Diff,
	}
}

func ensureSession(ctx context.Context, client *daemon.Client, sessionID, agentName, cwd string) (string, error) {
	if sessionID == "" {
		r, err := client.SessionStart(ctx, daemon.SessionStartParams{Agent: agentName, CWD: cwd})
		if err != nil {
			return "", err
		}
		return r.SessionID, nil
	}
	_, err := client.SessionStart(ctx, daemon.SessionStartParams{
		SessionID: sessionID,
		Agent:     agentName,
		CWD:       cwd,
	})
	if err == nil {
		return sessionID, nil
	}
	return "", err
}

func stripLockieMeta(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if len(k) >= 9 && k[:9] == "__lockie_" {
			continue
		}
		out[k] = v
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
