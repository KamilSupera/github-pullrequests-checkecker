# Session token-usage counter

Date: 2026-06-02

## Goal

Show how many Claude tokens (and the estimated USD cost) the current
`prcheck` session has consumed, accumulated across **all** Claude calls
(the review pipeline and the Jira-context fetch), displayed as a running
total in the TUI footer.

## Where tokens come from

Both Claude calls shell out to the `claude` CLI and both live in package
`internal/claude`:
- `invoke.go` — the review call.
- `jira.go` — the Jira issue fetch via Atlassian MCP.

Today both pass `--output-format text`. The CLI's `--output-format json`
returns an envelope:

```json
{"type":"result","result":"<text>","is_error":false,
 "total_cost_usd":0.0928,
 "usage":{"input_tokens":13802,"output_tokens":4,
          "cache_creation_input_tokens":2368,"cache_read_input_tokens":17837}}
```

`.result` holds the text we currently parse; `.usage` and
`.total_cost_usd` give the counts.

## Design

### Capture — package-level accumulator

Add `claude/usage.go`:

```go
type Usage struct {
    InputTokens, OutputTokens         int64
    CacheCreationTokens, CacheReadTokens int64
    CostUSD                           float64
    Calls                             int
}
func SessionUsage() Usage   // mutex-guarded snapshot
func addUsage(Usage)        // called by each CLI invocation
func resetUsage()           // test hook
```

Because both call sites are in package `claude`, a single unexported
accumulator covers them with no cross-package plumbing.

### Shared runner

Add `runClaude(ctx, prompt) (string, error)` in `invoke.go`, used by both
`Invoke` and `JiraMCPFetcher.FetchIssue`:

1. Exec `claude -p <prompt> --output-format json`.
2. Unmarshal stdout. If it is an envelope (`type == "result"`), record
   usage, honor `is_error`, and return `.result`.
3. **Fallback:** if stdout is not an envelope (stubbed text in tests / the
   demo fake), return raw stdout unchanged and record no usage. This keeps
   existing tests and the demo working without rewriting their fixtures.

`Invoke` parses the returned text with `ParseReview`; `FetchIssue` parses it
with the existing `findJSONObject` + jira unmarshal.

### Display — footer line

`tui/view.go` renders a dim session line above the key-help when
`SessionUsage().Calls > 0`:

```
⛁ 13.8k in · 4 out · 20.2k cached · $0.0928 · 1 call
```

Token counts use a `k`/`M` humanizer. The line is read directly from
`claude.SessionUsage()` at render time — no message plumbing — and refreshes
on every re-render (each progress/done event triggers one). The earlier
body-height fix already absorbs the extra footer line without overflow.

## Testing

- Envelope parse: fake `claude` emits an envelope; `Invoke` returns the
  inner review and `SessionUsage()` reflects the tokens/cost.
- Accumulation: two calls sum; `Calls == 2`.
- Fallback: bare-JSON stub still parses and records zero usage (regression
  for existing behavior).

## Out of scope

Per-review breakdown, persistence across sessions, and counting `gh` calls
(no token cost).
