# TUI: right-pane border overflow + section focus indicator

Date: 2026-06-02

## Problem

Two issues in the PR-detail TUI (`internal/tui`):

1. **Right border extends past the bottom.** When viewing PR details, the
   right pane's bottom border can be pushed below the visible area.
2. **No focus indicator.** Nothing shows which section (PR list, detail,
   comments) the scroll keys are currently acting on.

## Root cause (issue 1)

`Model.View()` lays out `header (1) + body (paneH+2) + footer`. With
`paneInnerSize` reserving `termH-4`, the body's outer height is
`paneH+2 = termH-2`, so `header + body + footer` totals **exactly `termH`**
with zero spare rows. The footer is normally 1 line, but grows to 2–3 lines
when a transient status message and/or active `/filter` line is shown. The
total then becomes `termH+1`/`termH+2`; in alt-screen mode the terminal drops
the overflowing bottom row — the pane border.

Measured (termH=40): plain=40, statusMsg=41, filter=41, both=42.

Left and right columns are already exactly equal height — the bug is total
vertical overflow, not left/right asymmetry.

## Design

### Fix 1 — fit the body to the real chrome height

In `View()`, build `header` and the fully-assembled `footer` (including the
optional status and filter lines) **before** sizing the body. Compute
`avail = termH - height(header) - height(footer)`; if the body's outer height
(`paneH+2`) exceeds `avail`, shrink `paneH` to `avail-2` (floor 5).

Propagate the (possibly shrunk) `paneH` to every body renderer:
`renderRightSplit` already recomputes its detail/comments split from its
`paneH` argument, so it follows automatically. Set the per-frame copies
`m.listH`, `m.resultH`, `m.diffVP.Height`, `m.checksVP.Height`,
`m.statsVP.Height = paneH` so the left list and any full-pane viewports also
match. These are value-receiver local mutations, no-ops when nothing shrinks.

Net effect: panes stay full size normally and give back 1–2 rows only while a
status/filter line is on screen — never overflowing.

### Fix 2 — section focus indicator

- Add `focusArea` enum (`focusList`, `focusDetail`, `focusComments`) and a
  `focus` field on `Model` (zero value = `focusList`, preserving today's
  behavior for users who never touch the new keys).
- Cycle focus with `l`/`→` (forward) and `h`/`←` (back), added to the main
  key switch only — sub-modes (diff/checks/stats/result) are unaffected.
  `Tab` stays bound to PR-tab switching.
- Border color follows focus: the focused box uses `borderActiveColor`
  (bright amber), the others `borderColor` (dim). Replaces today's
  "left always bright, right always dim". A full right pane (diff/checks/
  result) is bright whenever focus is not on the list.
- `j`/`k` act on the focused pane: list = move cursor (current behavior),
  detail = scroll detail, comments = scroll comments. Legacy `J`/`K`
  (comments page) and `[`/`]` (detail page) keep working regardless of focus.
- Footer gains an `h/l focus` hint.

## Testing

- `View()` height ≤ `termH` for several terminal sizes with statusMsg and
  filter set (regression for issue 1).
- `h`/`l` cycle `m.focus` through the three areas.
- With `focus == focusDetail`, `j` increments `detailOffset` and does not move
  the list cursor; with `focusList`, `j` moves the cursor (unchanged).

## Out of scope

Left/right pane width, the detail/comments 60/40 split ratio, and the existing
per-pane dedicated scroll keys are unchanged.
