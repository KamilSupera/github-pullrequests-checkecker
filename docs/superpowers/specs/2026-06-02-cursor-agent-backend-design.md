# Selectable agent CLI: claude or cursor

Date: 2026-06-02

## Goal

Let `prcheck` drive either the `claude` CLI (default) or Cursor's
`cursor-agent` CLI for the review and Jira-fetch calls, chosen by the user.

## Facts

- `cursor-agent -p --output-format json -f "<prompt>"` — `-p`/`--print` is a
  boolean (prompt is positional), `-f`/`--yolo` forces headless (no approval
  prompts, needed for the MCP Jira call).
- Its JSON envelope has `type`, `subtype`, `is_error`, `result`,
  `session_id`, `request_id` — **no token or cost fields**. Cursor reports no
  usage.
- `claude -p "<prompt>" --output-format json` — envelope additionally carries
  `usage` and `total_cost_usd` (already handled today).

## Design

### Agent abstraction (`claude/agent.go`)

A small selector, set once at startup:

```go
func SelectAgent(name string) error // "", "claude" | "cursor"/"cursor-agent"
func AgentBinary() string            // "claude" | "cursor-agent"
func AgentName() string              // "claude" | "cursor"
func AgentRecordsUsage() bool        // true for claude, false for cursor
func agentCommand(ctx, prompt) *exec.Cmd
```

`agentCommand` builds the per-CLI argv:
- claude: `claude -p <prompt> --output-format json`
- cursor: `cursor-agent -p --output-format json -f <prompt>`

Default is claude, preserving current behavior. The package keeps its name
(`claude`) to avoid churn in the `pipeline.ClaudeInvoker` interface and all
imports; the new file generalizes the runner inside it.

### Runner (`claude/invoke.go`)

`runClaude` → `runAgent`, building its command via `agentCommand`. Envelope
parsing is unchanged except the "is this a real envelope?" check becomes
`env.Type != ""` (both CLIs set `type`; bare stub/legacy JSON does not, so it
still falls through to raw-stdout). Cursor envelopes carry no `usage`, so the
recorded `Usage` is all zeros — but `addUsage` still increments `Calls`.

### Selection (`config` + `main`)

- `config.Config` gains `Agent string` from `PRCHECK_AGENT`.
- `main.run` calls `claude.SelectAgent(cfg.Agent)` and replaces the hardcoded
  `checkBinary("claude")` with `checkBinary(claude.AgentBinary())`, so it
  validates whichever CLI is selected.

### Footer (`tui/view.go`)

`renderUsageLine` branches on `claude.AgentRecordsUsage()`:
- claude: `⛁ 13.8k in · 4 out · 20.2k cached · $0.0928 · 1 call` (unchanged)
- cursor: `⛁ 3 calls · tokens n/a (cursor)`

The `Calls > 0` gate still drives visibility for both.

## Testing

- `SelectAgent` maps names; unknown name errors.
- With cursor selected, a stubbed `cursor-agent` envelope yields a parsed
  review, `Calls` increments, `AgentRecordsUsage()` is false.
- claude path and existing fallback behavior unchanged.

## Out of scope

A `--agent` CLI flag (env var is the selection mechanism), and surfacing
cursor cost once/if Cursor adds usage fields.
