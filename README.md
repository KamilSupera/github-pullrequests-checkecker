# prcheck

[![CI](https://github.com/KamilSupera/github-pullrequests-checkecker/actions/workflows/ci.yml/badge.svg)](https://github.com/KamilSupera/github-pullrequests-checkecker/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/KamilSupera/github-pullrequests-checkecker.svg)](https://pkg.go.dev/github.com/KamilSupera/github-pullrequests-checkecker)
[![Go Report Card](https://goreportcard.com/badge/github.com/KamilSupera/github-pullrequests-checkecker)](https://goreportcard.com/report/github.com/KamilSupera/github-pullrequests-checkecker)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A terminal UI for reviewing GitHub Pull Requests with Claude Code.

`prcheck` lists PRs you authored, PRs that need your review, and PRs where you're
mentioned — across all repos you can see. Select any PR and it runs an automated
review pipeline that drafts a **pending** review for you to inspect and submit.

It is a thin orchestrator: it shells out to the `gh` and `claude` CLIs you already
have. It stores **no credentials of its own** — all auth lives in those tools.

## How it works

For a selected PR the pipeline:

1. Fetches the diff with `gh pr diff`.
2. Fetches CI check status with `gh pr view`.
3. Extracts a Jira key from the PR body or branch and fetches the issue via Claude's
   Atlassian MCP server (no Jira API token stored locally).
4. Runs `claude -p` with the `caveman:caveman-review` skill, passing the diff, CI
   failures, and Jira context.
5. Posts a **pending** review to GitHub (via `gh`) with inline comments and a summary.

You then open the PR in the browser and submit (or edit) the review manually —
`prcheck` never auto-submits.

## Requirements

- **Go 1.26+** (to build only).
- **`gh` CLI**, authenticated — `gh auth login`.
- **`claude` CLI** (Claude Code), authenticated, with the Atlassian MCP server
  configured. The MCP server handles Jira — no Jira token needed locally.

## Install

```bash
go install github.com/KamilSupera/github-pullrequests-checkecker/cmd/prcheck@latest
```

Or build locally:

```bash
go build -o prcheck ./cmd/prcheck
```

Prebuilt binaries for Linux/macOS/Windows are attached to each
[GitHub Release](https://github.com/KamilSupera/github-pullrequests-checkecker/releases).

## Configure

No environment variables are required. All are optional:

| Variable | Effect |
|----------|--------|
| `PRCHECK_DEBUG=1` | Route subprocess (`gh`/`claude`) output to the log for troubleshooting. |
| `PRCHECK_FOCUS` | Comma-separated review aspects to emphasize, e.g. `security,performance,requirements,tests`. Empty = balanced review. |
| `PRCHECK_NOTIFY=1` | Send a desktop notification when a review finishes. |
| `PRCHECK_CACHE_DIR` | Override where local snapshots/bookmarks/history are written (default: OS cache dir, see below). |

**First-run Jira note:** if your MCP setup requires interactive consent on first tool
use, run a one-time `claude` query that hits an Atlassian MCP tool (e.g. ask it to
fetch any issue) before the first `prcheck` review, so the token cache is warm.

## Use

```bash
prcheck
```

```bash
prcheck --version   # print version/commit/build date
```

### Keys

**List view**

| Key | Action |
|-----|--------|
| `j` / `k` (or `↓`/`↑`) | Move cursor |
| `g` / `G` | Jump to top / bottom |
| `Tab` / `Shift+Tab` | Switch tab (authored · review-requested · mentioned) |
| `Space` | Load PR details / mark as seen |
| `Enter` | Run the review pipeline on the selected PR |
| `d` | View the diff (`n`/`p` jump between files, `esc` back) |
| `c` | View CI checks |
| `a` | Quick-approve (posts `LGTM`) |
| `b` | Toggle bookmark on the selected PR |
| `B` | Batch-review every bookmarked PR sequentially |
| `R` | Open the PR's checks page in the browser |
| `s` | Open the stats dashboard |
| `o` | Open the PR in the browser |
| `r` | Refresh the current tab |
| `/` | Filter the list |
| `J` / `K` | Scroll the comments pane |
| `]` / `[` | Scroll the detail pane |
| `esc` | Clear filter / dismiss the result view |
| `q` / `Ctrl+C` | Quit (or cancel a running pipeline) |

## Data & privacy

`prcheck` reads and sends data on your behalf — worth knowing before public use:

- **Read from GitHub** (via `gh`): PR lists, diffs, CI status, comments.
- **Sent to Claude** (via `claude`): the PR **diff**, CI failure text, and Jira issue
  text are passed to Claude Code to generate the review. PR code leaves your machine
  for Anthropic's API exactly as it does for any other Claude Code usage.
- **Jira**: fetched through the Atlassian MCP server that `claude` is configured
  against — `prcheck` itself never touches Jira credentials.
- **Posted to GitHub** (via `gh`): a *pending* review with inline comments. Nothing is
  submitted automatically.
- **Stored locally**: tab snapshots, bookmarks, seen-markers, and review history under
  your OS cache dir (`~/Library/Caches/prcheck` on macOS, `$XDG_CACHE_HOME/prcheck` or
  `~/.cache/prcheck` on Linux), or `PRCHECK_CACHE_DIR` if set. A failed review POST is
  dumped to `$TMPDIR/prcheck-*.json` (mode `0600`) so the Claude output isn't lost.
- **No secrets stored**: `prcheck` holds no API keys or tokens. Auth is delegated
  entirely to `gh` and `claude`.

## Manual smoke test

1. Open `prcheck`.
2. Move the cursor onto a PR you own.
3. Press `Enter`.
4. Wait for `Pending review #N posted.`
5. Press `o` — the PR opens in your browser.
6. On the **Files changed** tab the pending review appears at the top with all inline
   comments. Submit it manually.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Run `go build ./...`, `go vet ./...`,
`go test -race ./...`, and `gofmt -l .` before opening a PR.

## License

[MIT](LICENSE) © Kamil Supera
