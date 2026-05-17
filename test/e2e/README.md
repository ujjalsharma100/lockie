# Manual end-to-end checks (Phase 1)

Automated hook round-trips live in `test/integration/hook_roundtrip_test.go`.
Use this checklist before tagging a release.

## Prerequisites

- `make build` produces `./lockie`
- `lockie install claude-code --scope user` (or `--dry-run` to preview)
- `lockie install cursor --scope user`

## Claude Code

1. Start a session with hooks installed.
2. Paste a test-mode Stripe key (`sk_test_…`) into the prompt — transcript should show a `STRIPE_KEY_*` placeholder, not the literal.
3. `Read` a `.env` containing secrets — tool output in the transcript should be redacted.
4. Ask Claude to `curl` using the placeholder — the shell command should run with the real key rehydrated in `PreToolUse` (verify via your own logging, not the model transcript).

## Cursor

1. `lockie install cursor --scope user`
2. Repeat the Read / Shell scenarios above using Cursor's hook JSON shapes.

## Daemon

```bash
lockie daemon status
lockie hook post-tool < test/fixtures/hooks/claudecode_post_tool_read.json
```

Expect JSON with `hookSpecificOutput.updatedToolOutput` containing a `STRIPE_KEY_*` placeholder.
