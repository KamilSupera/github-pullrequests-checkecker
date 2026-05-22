# prcheck

A terminal UI for reviewing GitHub Pull Requests with Claude Code.

`prcheck` lists PRs you authored, PRs that need your review, and PRs where you're mentioned. For any selected PR it runs an automated review pipeline:

1. Fetches the diff with `gh pr diff`
2. Fetches CI check status with `gh pr view`
3. Extracts a Jira key from the PR body or branch and fetches the issue
4. Runs `claude -p` with the `caveman:caveman-review` skill, the diff, CI failures, and Jira context
5. Posts a **pending** review with inline comments and a summary

You then open the PR in the browser and submit (or edit) the review manually.

## Requirements

- Go 1.22+ (to build)
- `gh` CLI, authenticated (`gh auth login`)
- `claude` CLI (Claude Code), authenticated
- Atlassian API token

## Install

```bash
go install github.com/ksupera/prcheck/cmd/prcheck@latest
```

Or build locally:
```bash
go build -o prcheck ./cmd/prcheck
```

## Configure

```bash
export JIRA_BASE_URL=https://your-org.atlassian.net
export JIRA_EMAIL=you@example.com
export JIRA_TOKEN=...               # https://id.atlassian.com/manage-profile/security/api-tokens
export PRCHECK_DEBUG=1               # optional: subprocess output to log
```

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
