# Watch mode — design

**Date:** 2026-07-23
**Status:** approved, pending implementation

## Goal

Add a Watch mode to the prcheck TUI: auto-refresh all PR tabs on a timer and
fire desktop notifications on new PRs, re-requested reviews, and updates.

Most of the plumbing already exists:

- `cache.Notify` — desktop notification (macOS `osascript`, Linux `notify-send`).
- `cache.DetectChanges` — diffs an old snapshot against a fresh PR list.
- `update.go` already fires notifications on `prsLoadedMsg` when
  `PRCHECK_NOTIFY=1`.
- Manual refresh is bound to `r`.

Missing: an auto-refresh timer, and notification wording that distinguishes
new / re-request / update. This spec fills those two gaps.

## Key insight: re-request detection is free

When someone re-requests your review, the PR re-appears in the Review tab
(`gh search prs --review-requested=@me`). So **a new PR URL in the Review-tab
snapshot = a review request** (first or re-request). No extra API call needed —
`DetectChanges` already tracks which URLs are new vs pre-existing.

## Design

### Config (`internal/config/env.go`)

- Add `WatchMinutes int` to `Config`.
- Read `PRCHECK_WATCH` (integer minutes). Default 5. Clamp to ≥1.
- Watch mode starts **off**; the interval only says how often it ticks once on.

### Model (`internal/tui/model.go`)

New fields:

- `watching bool` — is watch currently on.
- `watchMin int` — tick interval in minutes.
- `watchGen int` — monotonic generation counter, guards against duplicate
  ticker loops (see Ticker below).
- `seededNotify map[Tab]bool` — has this tab had its first live load yet.

`NewModel` gains a `watchMin int` parameter; `main.go` passes
`cfg.WatchMinutes`. Initialize `seededNotify` to an empty map.

### Ticker (`internal/tui/update.go`)

- New message: `watchTickMsg{ gen int }`.
- Helper: `tickCmd(gen, min int) tea.Cmd` = `tea.Tick(time.Duration(min) *
  time.Minute, func(time.Time) tea.Msg { return watchTickMsg{gen} })`.
- `w` key handler:
  - Toggle `m.watching`.
  - On (off→on): `m.watchGen++`, set `m.statusMsg = "watch on (Nm)"`,
    return `tickCmd(m.watchGen, m.watchMin)`.
  - Off (on→off): set `m.statusMsg = "watch off"`, return `nil`.
    The generation counter is left as-is; any in-flight tick is dropped by the
    guard below.
- `watchTickMsg` handler:
  - If `msg.gen != m.watchGen || !m.watching` → `return m, nil` (this loop is
    stale and dies here). This is what prevents a toggle-off-then-on within one
    interval from leaving two live tickers.
  - Else refresh all three tabs and reschedule:
    `tea.Batch(m.loadTab(TabMine), m.loadTab(TabReview), m.loadTab(TabMentioned),
    tickCmd(m.watchGen, m.watchMin))`.
  - **Does not** set `m.prsByTab[t] = nil` (unlike the `r` handler), so the
    list never blank-flashes mid-view during an auto-refresh.

### Notify (`internal/tui/update.go` + `internal/cache/notify.go`)

- Change `DetectChanges` to return `[]Change` where
  `type Change struct { PR github.PR; IsNew bool }`. `IsNew` is true when the
  URL is absent from the old snapshot; otherwise the URL exists and its
  `UpdatedAt` differs.
- In the `prsLoadedMsg` handler, compare the fresh list against the
  **in-memory** `m.prsByTab[msg.tab]` captured *before* overwrite — not the
  on-disk snapshot. Gate on `seededNotify`:
  - If `!seededNotify[tab]`: set `seededNotify[tab] = true` and do **not**
    notify (seeds the baseline; avoids a burst on first load / startup).
  - Else: run `DetectChanges(old, fresh)` and notify per the rules below.
- Notification wording per `Change` (only when `PRCHECK_NOTIFY=1`):
  - Review tab + `IsNew` → title `"Review requested"`, body `repo#n title`.
  - Other tab + `IsNew` → title `"New PR"`, body `repo#n title`.
  - `!IsNew` (updated) → title `"Updated"`, body `repo#n title`.
  - If more than 3 changes in one tab, collapse to a single summary
    notification: `"<N> PRs changed in <tab>"` (avoids a notification storm).

### View (`internal/tui/view.go`)

- Add a footer item: key `w`, help `watch:on` or `watch:off` depending on
  `m.watching`.

## Testing

- Extend the existing `DetectChanges` test to assert `IsNew` classification
  (new URL → true, changed-`UpdatedAt` URL → false).
- Add a small `Update` test: pressing `w` sets `m.watching = true` and bumps
  `watchGen`; a `watchTickMsg` with a stale `gen` returns no command
  (no refresh, no reschedule).

## Out of scope (YAGNI)

- Per-tab intervals.
- Notify on CI/check status changes.
- Notification sound.
- Configurable notification templates.

Add any of these when a real need shows up.
