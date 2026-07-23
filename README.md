# prcheck

[![CI](https://github.com/KamilSupera/github-pullrequests-checkecker/actions/workflows/ci.yml/badge.svg)](https://github.com/KamilSupera/github-pullrequests-checkecker/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/KamilSupera/github-pullrequests-checkecker.svg)](https://pkg.go.dev/github.com/KamilSupera/github-pullrequests-checkecker)
[![Go Report Card](https://goreportcard.com/badge/github.com/KamilSupera/github-pullrequests-checkecker)](https://goreportcard.com/report/github.com/KamilSupera/github-pullrequests-checkecker)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A terminal UI for reviewing GitHub pull requests with Claude Code or Cursor.

`prcheck` shows the PRs you authored, the ones waiting on your review, and the ones
that mention you, across every repo you can see. Pick one and it runs a review
pipeline that drafts a **pending** GitHub review for you to read and submit. It never
submits anything on its own.

It is a thin wrapper around the `gh` and `claude` (or `cursor-agent`) CLIs you already
have. It keeps no credentials of its own. All auth lives in those tools.

https://github.com/user-attachments/assets/3203ce5e-6b75-4a7f-956f-cc453ca0255e

## What it does

- **Three PR queues** in tabs: authored, review-requested, mentioned. Grouped by repo,
  newest activity first.
- **Automated review** that drafts a pending GitHub review with a summary, an
  aspect checklist, and inline comments. You submit it yourself.
- **Diff viewer** with per-file jumping and color.
- **CI checks viewer**, plus a key to open the checks page in the browser.
- **Quick-approve** that posts an `LGTM` approval without leaving the list.
- **Bookmarks** and **batch review** to queue several PRs and review them in a row.
- **Filter** the list by title, repo, branch, or author.
- **State indicators** on every row: `✦` fresh, `•` updated since you last looked, `·` seen,
  plus `★` bookmarked and `✓` reviewed — with a legend under the tabs.
- **Watch mode** (`w`): auto-refresh all queues every few minutes and, opt-in, get a desktop
  notification when a review is requested, a new PR shows up, or a tracked PR changes.
- **Stats dashboard**: counts per tab, per repo, and stale-PR age.
- **Desktop notifications** when a review finishes (opt-in).
- **Pick your agent**: Claude or Cursor, switched live from inside the app.
- **Pick your model per review**: pressing `Enter` opens a model picker for the active
  agent (e.g. Opus, Sonnet, Haiku for Claude) before the review starts. The `default`
  entry shows which model the CLI would use on its own.
- **Token and cost counter** for the session (Claude reports usage; Cursor does not).
- **Jira context**: pulls the linked issue through Claude's Atlassian MCP server, so the
  review knows what the PR was supposed to do.

## How it works

When you run a review on a PR, the pipeline:

1. Fetches the diff with `gh pr diff`.
2. Fetches CI check status with `gh pr view`.
3. Looks for a Jira key in the PR body or branch name. If it finds one, it fetches the
   issue through the agent's Atlassian MCP server (no Jira token stored locally).
4. Runs the agent (`claude -p`, or `cursor-agent`) with the diff, the CI failures, and
   the Jira context.
5. Posts a **pending** review to GitHub through `gh`: a summary plus inline comments.

Then you open the PR in the browser and submit or edit the review by hand.

## Requirements

- **Go 1.26+**, to build.
- **`gh` CLI**, authenticated (`gh auth login`).
- An **agent CLI**:
  - **`claude`** (Claude Code), authenticated, with the Atlassian MCP server set up if
    you want Jira context. This is the default and the only one that reports token usage.
  - **`cursor-agent`** (Cursor), if you set `PRCHECK_AGENT=cursor` or switch to it in the
    app.

## Install

```bash
go install github.com/KamilSupera/github-pullrequests-checkecker/cmd/prcheck@latest
```

Or build locally:

```bash
go build -o prcheck ./cmd/prcheck
```

Prebuilt binaries for Linux, macOS, and Windows are attached to each
[GitHub Release](https://github.com/KamilSupera/github-pullrequests-checkecker/releases).

## Configure

Nothing is required. Everything is optional:

| Variable | Effect |
|----------|--------|
| `PRCHECK_THEME` | Color scheme: `auto` (default — detect the terminal background at startup, dark if unsure), `dark`, or `light`. |
| `PRCHECK_AGENT` | Which agent to start with: `claude` (default) or `cursor`. You can also switch in the app with `A`. |
| `PRCHECK_FOCUS` | Review aspects to emphasize, comma-separated, e.g. `security,performance,requirements,tests`. Empty means a balanced review. |
| `PRCHECK_NOTIFY=1` | Send a desktop notification when a review finishes. |
| `PRCHECK_WATCH` | Auto-refresh interval in minutes for Watch mode (default 5). |
| `PRCHECK_DEBUG=1` | Send subprocess (`gh`/agent) output to the log for troubleshooting. |
| `PRCHECK_CACHE_DIR` | Where snapshots, bookmarks, and history are written. Defaults to the OS cache dir (see below). |

**Config file (optional).** Settings can also live in a JSON file so you don't have to
export env vars. prcheck looks for it at (via `os.UserConfigDir()`):

- Linux: `~/.config/prcheck/config.json`
- macOS: `~/Library/Application Support/prcheck/config.json`
- Windows: `%AppData%\prcheck\config.json`

```json
{
  "theme": "light"
}
```

Environment variables **override** the file (e.g. `PRCHECK_THEME=dark` wins over
`"theme": "light"`). A missing file is fine; a malformed one is ignored with a warning.
Currently only `theme` is read from the file — the other settings remain env-only for now.

**Color scheme.** `theme` (or `PRCHECK_THEME`) is `auto`, `dark`, or `light`. `auto` checks
the terminal's background color once at startup and picks the matching scheme, falling back
to `dark` if the terminal doesn't report one. The default `dark` scheme is tuned for dark
terminals; `light` is tuned for white/light backgrounds.

**Watch mode** (`w` key) auto-refreshes all tabs every `PRCHECK_WATCH` minutes (default 5).
When combined with `PRCHECK_NOTIFY=1`, it sends desktop notifications: "Review requested"
when a PR (re-)appears in the Review tab, "New PR" for new PRs elsewhere, and "Updated"
when a tracked PR changes.

**First Jira run:** if your MCP setup asks for consent the first time a tool is used, run
one `claude` query that hits an Atlassian tool (ask it to fetch any issue) before your
first review, so the token cache is warm.

## Use

```bash
prcheck            # start the UI
prcheck --version  # print version, commit, build date
```

### Layout

The tab bar sits on top. Below it, the PR list is on the left; on the right, the
selected PR's detail sits above its comments. The session token and cost counter, any
status message, and the key help run along the bottom.

Three panes can hold focus: the list, the detail box, and the comments box. The focused
box gets a bright border, and `j`/`k` scroll whichever one is focused. Cycle focus with
`h`/`l` or the arrow keys.

### List indicators

Each PR row starts with a two-character state gutter:

| glyph | meaning |
|-------|---------|
| `✦` | fresh — you have never opened this PR |
| `•` | updated — opened before, new activity since |
| `·` | seen — opened, nothing new |
| `★` | bookmarked |
| `✓` | you posted a review on it |

The first column is always one of `✦`/`•`/`·`; the second shows `★` (or `✓` if reviewed and not bookmarked).

### Keys

| Key | Action |
|-----|--------|
| `j` / `k` (or `↓`/`↑`) | Move the cursor, or scroll the focused pane |
| `h` / `l` (or `←`/`→`) | Cycle focus between list, detail, and comments |
| `g` / `G` | Jump to top / bottom of the list |
| `Tab` / `Shift+Tab` | Switch tab (Mine, Review, Mentioned) |
| `Space` | Load PR details, and mark it seen |
| `Enter` | Pick a model, then run the review pipeline on the selected PR |
| `d` | Open the diff (`n`/`p` jump between files, `esc` to go back) |
| `c` | Open the CI checks |
| `a` | Quick-approve (posts `LGTM`) |
| `b` | Bookmark, or remove the bookmark |
| `B` | Review every bookmarked PR in sequence |
| `R` | Open the PR's checks page in the browser |
| `s` | Open the stats dashboard |
| `A` | Switch the agent (Claude / Cursor) |
| `o` | Open the PR in the browser |
| `r` | Refresh the current tab |
| `w` | Toggle Watch mode (auto-refresh + desktop notifications) |
| `/` | Filter the list |
| `J` / `K` | Page the comments pane |
| `]` / `[` | Page the detail pane |
| `esc` | Clear the filter, or dismiss the result view |
| `q` / `Ctrl+C` | Quit, or cancel a running pipeline |

The footer wraps onto more lines on a narrow terminal, so every key stays visible.

## Data and privacy

`prcheck` reads and sends data on your behalf. Worth knowing before you use it:

- **Read from GitHub** (via `gh`): PR lists, diffs, CI status, comments.
- **Sent to the agent**: the PR diff, the CI failure text, and the Jira issue text go to
  Claude or Cursor to generate the review. Your PR code leaves the machine the same way
  it does for any other Claude Code or Cursor usage.
- **Jira**: fetched through the agent's Atlassian MCP server. `prcheck` never touches
  Jira credentials.
- **Posted to GitHub** (via `gh`): a *pending* review with inline comments. Nothing is
  submitted automatically.
- **Stored locally**: tab snapshots, bookmarks, seen-markers, and review history, under
  your OS cache dir (`~/Library/Caches/prcheck` on macOS, `$XDG_CACHE_HOME/prcheck` or
  `~/.cache/prcheck` on Linux), or `PRCHECK_CACHE_DIR` if set. A failed review POST is
  dumped to `$TMPDIR/prcheck-*.json` (mode `0600`) so the agent's output isn't lost.
- **No secrets stored**: `prcheck` holds no API keys or tokens. Auth is delegated to `gh`
  and the agent.

## Manual smoke test

1. Open `prcheck`.
2. Put the cursor on a PR you own.
3. Press `Enter`.
4. Wait for `Pending review #N posted.`
5. Press `o`. The PR opens in your browser.
6. On the Files changed tab the pending review is at the top with its inline comments.
   Submit it by hand.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Run `go build ./...`, `go vet ./...`,
`go test -race ./...`, and `gofmt -l .` before opening a PR.

## Thanks

[@kaniak274](https://github.com/kaniak274) for QA and for the idea of showing
token usage in the UI.

## License

[MIT](LICENSE) © Kamil Supera
