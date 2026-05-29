# prcheck demo mode

Run `prcheck` against **canned mock pull requests** — for screenshots, recordings,
or trying the UI without exposing real (company) PRs.

It works by putting fake `gh` and `claude` executables (in `bin/`) at the front of
`PATH`. They serve static fixtures from `fixtures/` and **never contact GitHub or
Anthropic**. Nothing is posted; the "post review" call returns a fake review id.

## Run

```bash
./demo/demo.sh
```

This builds `prcheck`, points it at the fakes, isolates the cache to `demo/.cache`,
and launches the TUI. Drive it normally:

- `Tab` cycles the three tabs (Mine · Review · Mentioned).
- `Space` loads PR details, `d` shows the diff, `c` shows checks, `s` the stats board.
- `Enter` runs the review pipeline — the fake `claude` returns a sample review with a
  summary, an aspect checklist, and three inline comments, and you'll see
  `Pending review #99042 posted.`

## The mock data

| Tab | PRs |
|-----|-----|
| Mine | #42 dark-mode toggle · #57 search debounce · #63 retry middleware |
| Review | #88 idempotency keys · #91 push crash fix |
| Mentioned | #104 terraform provider bump |

Edit the JSON/diff/text files in `fixtures/` to change what's shown. The fake `gh`
picks fixtures by PR number (`view-42.json`, `diff-42.txt`, `checks-42.txt`); add
`view-<n>.json` etc. for new PRs, or rely on the `*-default` fallbacks.

The sample review is hard-coded in `bin/claude`.

## Not committed

`demo/prcheck` (built binary) and `demo/.cache` are gitignored.
