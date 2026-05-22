# prcheck — PR Review CLI (Design)

**Date:** 2026-05-22
**Status:** Draft for review
**Author:** Kamil Supera

## 1. Purpose

A single-user terminal tool that:

1. Lists GitHub Pull Requests you authored, that you've been asked to review, or where you're mentioned.
2. For a selected PR, runs an automated code review using Claude Code with the `caveman:caveman-review` skill, augmented with:
   - GitHub Actions check status,
   - Jira issue context (auto-extracted from PR description),
   - quality and performance commentary.
3. Posts the review to GitHub as a **pending** review (with inline comments and a summary). The user opens the PR in the browser and submits the review manually.

The tool is interactive (Bubble Tea TUI), runs on macOS and Linux, and reuses the user's existing `gh` CLI and `claude` CLI installations.

## 2. Non-goals

- Not a hosted service or CI bot. Single-user local tool.
- Does not submit reviews automatically. Human-in-the-loop is the whole point.
- No support for GitLab/Bitbucket. GitHub only.
- No persistent cache or database in MVP. Fresh fetch each launch.
- No web UI, no daemon, no scheduling.
- No multi-PR batch review (single-select only).

## 3. Stack

- **Language:** Go (single static binary distribution).
- **TUI:** [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) for styling + [Bubbles](https://github.com/charmbracelet/bubbles) for list/spinner.
- **GitHub:** shell out to `gh` CLI (and `gh api` for raw REST when needed).
- **Jira:** direct REST calls (`net/http`), API token auth.
- **Claude:** spawn `claude -p "<prompt>" --output-format json` subprocess.

Rationale captured in the brainstorming transcript: gh CLI reuses existing auth, direct Jira REST is small, Claude subprocess preserves the `caveman:caveman-review` skill (which lives inside the Claude Code harness).

## 4. Architecture

Single Go binary `prcheck`. Bubble Tea drives the UI. Side-effecting work (gh, claude, Jira) runs in goroutines and sends `tea.Msg` values back to the model. No persistent state on disk for MVP.

```
┌──────────────────────────────────────────────────────────┐
│                    prcheck (Go binary)                   │
│                                                          │
│  ┌────────────────┐   tea.Msg     ┌────────────────┐    │
│  │  Bubble Tea    │ ◄──────────── │  Pipeline      │    │
│  │  TUI (model,   │               │  goroutines    │    │
│  │  update, view) │ ── commands ─►│                │    │
│  └────────────────┘               └───────┬────────┘    │
│                                           │             │
└───────────────────────────────────────────┼─────────────┘
                                            │ exec
                            ┌───────────────┼──────────────┐
                            ▼               ▼              ▼
                       ┌─────────┐    ┌──────────┐   ┌──────────┐
                       │   gh    │    │ claude   │   │  Jira    │
                       │   CLI   │    │  CLI -p  │   │  REST    │
                       └─────────┘    └──────────┘   └──────────┘
```

Pipeline is a stateless function chain per PR. Cancellable via `context.Context` (Ctrl+C / `q` propagates to subprocesses via `exec.CommandContext`). The UI never blocks.

## 5. Components

```
cmd/prcheck/main.go          entrypoint, flag parsing, launches Bubble Tea
internal/tui/
  model.go                   Bubble Tea model (state)
  update.go                  message handling
  view.go                    rendering (list + detail panes)
  messages.go                tea.Msg types: prsLoaded, progressMsg, reviewDoneMsg, errMsg
internal/github/
  pr.go                      gh pr list/view wrappers, struct types
  checks.go                  gh pr checks → ChecksSummary
  review.go                  build POST body, gh api POST /repos/.../pulls/N/reviews
internal/jira/
  client.go                  REST GET /rest/api/3/issue/{key}
  extract.go                 regex Jira keys from PR body/branch
internal/claude/
  invoke.go                  exec.Command("claude", "-p", prompt, "--output-format", "json")
  prompt.go                  build prompt: skill load + diff + Jira + checks + JSON schema
  parse.go                   parse Claude's JSON review payload
internal/pipeline/
  pipeline.go                orchestrate: fetch → checks → jira → claude → post
  diff.go                    diff size cap (~100k chars), truncation warning
internal/config/
  env.go                     load GH_*, JIRA_* env vars
```

Component responsibilities:

- **tui** — pure rendering + key bindings. Sends commands. Receives messages. No I/O.
- **github** — thin wrappers over `gh` subprocess. Returns typed structs.
- **jira** — minimal REST client. One method: `FetchIssue(key) (*Issue, error)`.
- **claude** — builds prompt, runs subprocess, parses JSON. Skill loaded via prompt prefix.
- **pipeline** — orchestrator. Emits progress events. Cancellable via context.
- **config** — env var loading + validation at startup.

## 6. Data flow

### 6.1 Startup

1. Validate env: run `gh auth status` (must be authenticated), check `JIRA_BASE_URL`, `JIRA_EMAIL`, `JIRA_TOKEN` are set, check `claude` is on PATH. Hard-fail with a clear message on any missing piece.
2. Spawn 3 parallel `gh search prs` queries:
   - `is:open is:pr author:@me`
   - `is:open is:pr review-requested:@me`
   - `is:open is:pr mentions:@me`

   Dedupe by PR URL. Group into 3 tabs (Mine / Review / Mentioned).
3. Mount the TUI.

### 6.2 PR detail (passive load on cursor move)

When the cursor lands on a PR, lazily run:

```
gh pr view <url> --json title,body,headRefName,baseRefName,author,statusCheckRollup,url
```

Cache the response in `model.details[url]` so re-cursoring is free.

### 6.3 Review pipeline (Enter pressed)

```
[1/5] Fetch diff       gh pr diff <url>              → unified diff text
[2/5] Fetch checks     gh pr checks <url>            → ChecksSummary{passed,failed,pending}
[3/5] Fetch Jira       regex body → key → REST       → IssueSummary (skip if no key)
[4/5] Run Claude       claude -p "<prompt>" --output-format json   → JSON {summary, comments[]}
[5/5] Post review      gh api POST /repos/.../reviews   → review_id (PENDING, not submitted)
```

Each step emits `progressMsg{step, status}`. Spinner advances. Final `reviewDoneMsg{url, reviewID}` shows:
> "Pending review #ID posted. Press `o` to open in browser, `n` for next PR."

### 6.4 Claude prompt shape

```
Use skill caveman:caveman-review.

PR: <title>
Branch: <head> → <base>
Author: <user>

Jira issue (if present):
<key>: <summary>
<description>

CI checks:
- failed: <list>
- pending: <list>

Diff (truncated at 100k chars if longer — see warning at end):
<unified diff>

Return ONLY JSON matching this schema:
{
  "summary": "string (overall review summary, 2-5 sentences)",
  "comments": [
    {
      "path": "file path from diff",
      "line": <int, line number in NEW file>,
      "side": "RIGHT" | "LEFT",
      "body": "review comment",
      "severity": "blocker" | "major" | "minor" | "nit"
    }
  ]
}
```

### 6.5 Posted review body

`POST /repos/{owner}/{repo}/pulls/{number}/reviews` with:

```json
{
  "body": "<summary>",
  "comments": [
    { "path": "...", "line": 42, "side": "RIGHT", "body": "..." }
  ]
}
```

No `event` field → review stays **PENDING**. User submits manually in the GitHub web UI.

## 7. TUI flow

Two-pane layout. Tabs across the top: `Mine | Review | Mentioned`.

```
┌─ prcheck ─────────────────────────────────────────────────────┐
│ [Mine] Review  Mentioned                                      │
├──────────────────────────────┬────────────────────────────────┤
│ ► #1234 Add caching layer    │ Title: Add caching layer       │
│   #1235 Fix auth race        │ Branch: feat/cache → develop   │
│   #1236 Refactor pipeline    │ Author: kamil                  │
│   #1240 Bump deps            │ Checks: 3 passed, 1 failed     │
│                              │ Jira:   ABC-123 (extracted)    │
│                              │                                │
│                              │ Press Enter to run review.     │
├──────────────────────────────┴────────────────────────────────┤
│ j/k move  Tab switch  Enter review  o open  q quit            │
└───────────────────────────────────────────────────────────────┘
```

Keys:

| Key            | Action                                       |
|----------------|----------------------------------------------|
| `j` / `↓`      | Move down                                    |
| `k` / `↑`      | Move up                                      |
| `Tab` / `Shift+Tab` | Cycle tabs                              |
| `Enter`        | Run review pipeline on highlighted PR        |
| `o`            | Open highlighted PR (or completed review) in browser |
| `r`            | Refresh PR lists                             |
| `q` / `Ctrl+C` | Quit (or cancel running pipeline)            |

During a pipeline, the detail pane is replaced with a progress view showing the 5 steps and a spinner on the active step. The list pane remains visible.

## 8. Configuration

Env vars only (no config file in MVP):

| Var               | Purpose                                       |
|-------------------|-----------------------------------------------|
| (gh auth)         | GitHub auth via `gh auth login` — no env var needed |
| `JIRA_BASE_URL`   | e.g. `https://your-org.atlassian.net`         |
| `JIRA_EMAIL`      | Atlassian account email                       |
| `JIRA_TOKEN`      | Atlassian API token                           |
| `PRCHECK_DEBUG`   | If `1`, append subprocess stdout/stderr to log |

Logs: `~/.local/state/prcheck/prcheck.log` (XDG state dir; fallback `~/.prcheck.log` on systems without `XDG_STATE_HOME`).

## 9. Error handling

### Startup (fatal, exit before TUI)

- `gh auth status` non-zero → "Run `gh auth login` first." exit 1.
- Missing Jira env vars → list which are missing. exit 1.
- `claude` not on PATH → "claude CLI not found. Install Claude Code." exit 1.

### Runtime (non-fatal, surface in TUI)

| Step           | Failure mode                  | Behavior                                                                 |
|----------------|-------------------------------|--------------------------------------------------------------------------|
| Fetch PR lists | `gh search` non-zero          | Show tab with error banner, other tabs still work                        |
| Fetch diff     | Empty diff / gh error         | Banner "no diff", abort pipeline                                         |
| Fetch checks   | gh timeout                    | Warn banner, continue with `checks=unknown` (do not block review)        |
| Fetch Jira     | No key in body                | Skip silently, omit Jira section from prompt                             |
| Fetch Jira     | Key found, API 404/401        | Warn banner, continue without Jira section                               |
| Run Claude     | Non-zero exit                 | Banner with stderr tail, abort, keep partial logs                        |
| Run Claude     | Invalid JSON output           | Banner "Claude returned non-JSON, see log", abort. No retry.             |
| Post review    | gh api 4xx / network          | Banner with API error. Dump review JSON to `/tmp/prcheck-<pr>-<ts>.json` so it's not lost |

Cancellation: ctx cancel propagates to all `exec.CommandContext` subprocesses. No orphan processes.

## 10. Testing strategy

### Unit (table-driven, no network)

- `internal/jira/extract_test.go` — regex extraction: `ABC-123`, branch name `proj/feat/ABC-123-foo`, multi-key body, no key, malformed.
- `internal/claude/parse_test.go` — JSON parse: valid, missing fields, bad severity enum, extra prose around JSON, empty comments array.
- `internal/claude/prompt_test.go` — prompt builder: golden-file comparison; diff truncation at cap; Jira-absent shape; checks-failed shape.
- `internal/github/review_test.go` — POST body builder: maps comments to GitHub schema, LEFT/RIGHT side mapping.
- `internal/pipeline/diff_test.go` — cap logic: under cap untouched, over cap truncated with marker.

### Integration (fake binaries on PATH)

- Test helper builds tiny Go stubs: `fake-gh`, `fake-claude`. Tests prepend a tempdir to `PATH`. Stubs read fixtures from `testdata/`, echo canned output.
- `internal/pipeline/pipeline_test.go` — full pipeline with fake gh + fake claude. Asserts: correct subprocess calls, correct order, progress messages emitted, final review posted with right body.
- Failure injection: stubs exit non-zero, output garbage, hang (with short ctx timeout).

### TUI (teatest)

- Boot model with seeded PR list. Send key events (j, k, Tab, Enter, q). Assert visible output via `teatest.WaitFor`. No real I/O.
- Cancel test: enter pipeline → fire ctx cancel → assert spinner gone, list visible.

### Manual smoke (documented in README)

- One real PR you own. `prcheck` → review it → confirm pending review appears on github.com.

### Out of scope

- No e2e against real GitHub. No mocked HTTP server (subprocess abstraction makes it unnecessary).
- No coverage gate. Focus on critical paths: parse, prompt build, post body.

## 11. Open questions

None at design time. Implementation may surface details (exact JSON parsing edge cases from Claude, line-number mapping for diff hunks → GitHub `line` field). Those will be handled in the implementation plan.

## 12. Future (out of scope for MVP)

- Multi-PR batch review queue.
- On-disk cache for PR list / details to make re-launch instant.
- Headless CLI mode (`prcheck review <url>` without TUI) for scripting.
- Slack / mail notification when reviews are ready to submit.
- Support for GitLab / Bitbucket.
- Config file (`~/.config/prcheck/config.yaml`) for non-secret prefs.
