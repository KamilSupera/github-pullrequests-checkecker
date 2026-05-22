# prcheck

A terminal UI for reviewing GitHub Pull Requests with Claude Code.

`prcheck` lists PRs you authored, PRs that need your review, and PRs where you're mentioned. For any selected PR it runs an automated review pipeline:

1. Fetches the diff with `gh pr diff`
2. Fetches CI check status with `gh pr view`
3. Extracts a Jira key from the PR body or branch and fetches the issue via Claude's Atlassian MCP server
4. Runs `claude -p` with the `caveman:caveman-review` skill, the diff, CI failures, and Jira context
5. Posts a **pending** review with inline comments and a summary

You then open the PR in the browser and submit (or edit) the review manually.

## Requirements

- Go 1.22+ (to build)
- `gh` CLI, authenticated (`gh auth login`)
- `claude` CLI (Claude Code), authenticated, with the Atlassian MCP server configured (handles Jira fetches — no Jira API token needed locally)

## Install

```bash
go install github.com/ksupera/prcheck/cmd/prcheck@latest
```

Or build locally:
```bash
go build -o prcheck ./cmd/prcheck
```

## Configure

No env vars are required. Optional:

```bash
export PRCHECK_DEBUG=1               # subprocess output to log
```

Jira credentials are not stored locally; the Atlassian MCP server that `claude` is configured against handles authentication. If your MCP setup requires interactive consent on first tool use, run a one-time `claude` query that hits an Atlassian MCP tool (e.g. ask it to fetch any issue) before the first `prcheck` review, so the token cache is warm.

## Use

```bash
prcheck
```

Keys: `j`/`k` move, `Tab` switch tab, `Enter` run review, `o` open in browser, `r` refresh, `q` quit (or cancel running pipeline).

## Manual smoke test

1. Open `prcheck`.
2. Cursor onto a PR you own.
3. Press `Enter`.
4. Wait for "Pending review #N posted."
5. Press `o`. The PR opens in your browser.
6. On the **Files changed** tab you should see your pending review at the top with all inline comments. Submit it manually.
