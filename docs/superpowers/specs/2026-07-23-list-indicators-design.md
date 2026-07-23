# PR-list state indicators — design

**Date:** 2026-07-23
**Status:** approved, pending implementation
**Branch:** `feat/list-indicators` (off `develop`, independent of `feat/watch-mode`)

## Goal

Show, at a glance in the PR list, what state each PR is in: fresh (never
opened), updated since last seen, seen-and-quiet, whether the user has posted
a review on it, and whether it is bookmarked. Today the list collapses
fresh+updated into a single `•` dot and shows `★` for bookmarks; there is no
"seen" or "reviewed" indicator.

## Existing building blocks

- `cache.SeenStore` (`internal/cache/seen.go`): `URLs map[string]time.Time`,
  `Mark(url)`, `IsNew(url, updatedAt)` (true when never seen **or** updated
  since last view — cannot distinguish the two today), `Save()`.
- `cache.BookmarkStore`: `Has(url)`, `Toggle(url)`.
- `cache.LoadHistory(0)` returns every `HistoryEntry{PRURL, ...}` the app has
  written; `cache.AppendHistory` is called on `reviewDoneMsg`
  (`internal/tui/update.go`) when a pipeline review posts a pending review.
- `renderList` in `internal/tui/view.go` renders one `marker` char per row
  (last-wins: `•` for new, overridden by `★` for bookmarked).

## Design

### Two-column gutter

Each PR row gets a **2-character gutter** before the date: `<freshness><mark>`.

**Column 1 — freshness** (mutually exclusive, from `SeenStore`):

| glyph | color   | state   | rule |
|-------|---------|---------|------|
| `✦`   | peach   | fresh   | never opened: `url` NOT in `seen.URLs` |
| `•`   | orange  | updated | opened before AND `IsNew` true (activity since last view) |
| `·`   | dim     | seen    | opened AND `IsNew` false (quiet) |

Every PR falls into exactly one of these three, so column 1 is never blank.

**Column 2 — mark** (priority order, first match wins):

| glyph | color  | state      | rule |
|-------|--------|------------|------|
| `★`   | yellow | bookmarked | `bookmarks.Has(url)` |
| `✓`   | green  | reviewed   | `url` in the reviewed set (see below) AND not bookmarked |
| ` `   | —      | none       | neither |

If a PR is both reviewed and bookmarked, `★` wins (bookmark is the more
deliberate signal). Documented ponytail simplification; upgrade path is a
third gutter column if both must show at once.

### Row-text dimming (unchanged)

The existing behavior stays: fresh/updated rows render bright, seen rows
render dim, cursor row highlighted. Only the gutter is new. "Fresh" and
"updated" both count as "not seen / has activity" for the existing bright
vs. dim decision (i.e. `isPRNew` still drives text color).

### Reviewed set

Add `Model.reviewed map[string]bool`, populated once in `NewModel` from
`cache.LoadHistory(0)` (one entry per posted review; dedupe by `PRURL`).
Update it live: in the `reviewDoneMsg` handler, after `AppendHistory`, set
`m.reviewed[msg.url] = true` so the `✓` appears without a restart.

### Testable classifier

Extract the state decision out of rendering so it can be unit-tested without
a terminal:

```go
type prState struct {
	fresh, updated, seen bool // exactly one true
	reviewed             bool
	bookmarked           bool
}

func (m Model) prStateOf(pr github.PR) prState
```

`renderList` calls `prStateOf` and maps the struct to the two glyphs +
styles. The classifier is pure (reads `m.seen`, `m.reviewed`, `m.bookmarks`);
the test constructs a `Model` with known stores and asserts the struct.

### SeenStore helper

Add `func (s *SeenStore) Opened(url string) bool { _, ok := s.URLs[url]; return ok }`
(nil-safe at call site — `prStateOf` guards `m.seen == nil`).

### Legend

One faint line rendered under the tab bar (above the list):

```
✦ new  • upd  · seen  ✓ reviewed  ★ saved
```

Keep it to a single line; it uses the same glyph colors as the gutter.

## Files touched

- `internal/cache/seen.go` — add `Opened`.
- `internal/tui/model.go` — add `reviewed` field + populate in `NewModel`.
- `internal/tui/update.go` — mark `m.reviewed[msg.url]=true` in `reviewDoneMsg`.
- `internal/tui/view.go` — `prStateOf`, gutter render in `renderList`, legend line.
- Tests: `internal/cache/seen_test.go` (Opened), `internal/tui/*_test.go`
  (`prStateOf` classification).

## Testing

- `SeenStore.Opened`: true after `Mark`, false for unknown URL.
- `prStateOf`: for a `Model` with seeded stores, assert each of
  fresh/updated/seen (col 1) and the reviewed/bookmarked/none priority
  (col 2), including the bookmarked+reviewed → bookmarked-wins case.

## Out of scope (YAGNI)

- Marking quick-reviews (`a` approve/comment path) as reviewed — only
  pipeline reviews write history; quick reviews don't. Add if it turns out to
  matter.
- Per-state color/glyph configuration.
- A third gutter column to show reviewed+bookmarked simultaneously.
