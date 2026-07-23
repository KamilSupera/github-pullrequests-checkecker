# PR-list State Indicators Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show per-PR state in the list via a 2-column gutter — fresh / updated / seen (column 1) and bookmarked / reviewed (column 2) — plus a legend line.

**Architecture:** A pure classifier `Model.prStateOf(pr) prState` reads the existing `seen`, `bookmarks`, and a new `reviewed` set and returns a struct of bools. `listLines` in `view.go` maps that struct to two glyphs. `reviewed` is built once from review history and updated live when a review completes.

**Tech Stack:** Go 1.26, Bubble Tea, lipgloss, existing `internal/cache` stores.

## Global Constraints

- Go module `github.com/KamilSupera/github-pullrequests-checkecker`, `go 1.26.3`. No new dependencies.
- Every commit must compile (`go build ./...`), pass `go test ./...`, and be **gofmt-clean** — this repo's CI runs `gofmt -l .` as a hard gate. Run `gofmt -w` on touched files (or `go fmt ./...`) and confirm `gofmt -l .` prints nothing before committing.
- Column 1 (freshness) is exactly one of fresh/updated/seen. Column 2 (mark) is bookmarked **or** reviewed **or** blank, in that priority (bookmarked wins).
- Follow existing patterns in the files touched; keep the existing row-text dimming (driven by `isPRNew`) unchanged.

---

### Task 1: `SeenStore.Opened`

**Files:**
- Modify: `internal/cache/seen.go`
- Test: `internal/cache/cache_test.go`

**Interfaces:**
- Produces: `func (s *SeenStore) Opened(url string) bool` — true iff the URL has ever been marked seen.

- [ ] **Step 1: Write the failing test**

Append to `internal/cache/cache_test.go`:

```go
func TestSeenOpened(t *testing.T) {
	s := &SeenStore{URLs: map[string]time.Time{}}
	if s.Opened("x") {
		t.Error("unknown url should not report opened")
	}
	s.Mark("x")
	if !s.Opened("x") {
		t.Error("marked url should report opened")
	}
}
```

(`time` is already imported by `cache_test.go`; if the build says otherwise, add it.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cache/ -run TestSeenOpened -v`
Expected: FAIL — `s.Opened undefined`.

- [ ] **Step 3: Implement `Opened`**

In `internal/cache/seen.go`, add after the `IsNew` method:

```go
// Opened reports whether the user has ever loaded this PR's detail
// (i.e. the URL has a recorded view time).
func (s *SeenStore) Opened(url string) bool {
	_, ok := s.URLs[url]
	return ok
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cache/ -run TestSeenOpened -v`
Expected: PASS.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w internal/cache/seen.go internal/cache/cache_test.go
gofmt -l .   # must print nothing
git add internal/cache/seen.go internal/cache/cache_test.go
git commit -m "feat(cache): add SeenStore.Opened"
```

---

### Task 2: `Model.reviewed` set (populate + live update)

**Files:**
- Modify: `internal/tui/model.go` (struct field + `NewModel` init)
- Modify: `internal/tui/update.go` (`reviewDoneMsg` handler)

**Interfaces:**
- Consumes: `cache.LoadHistory(0)` (existing), `reviewDoneMsg.url` (existing).
- Produces: `Model.reviewed map[string]bool` — set of PR URLs the user has posted a pipeline review on. Non-nil after `NewModel`.

The field is read by `prStateOf` in Task 3; adding it here (written in `NewModel` and `reviewDoneMsg`) compiles and passes on its own.

- [ ] **Step 1: Add the struct field**

In `internal/tui/model.go`, in the `Model` struct, next to the `seen`/`bookmarks` fields (around line 119-120), add:

```go
	reviewed  map[string]bool // PR URLs the user has posted a review on
```

- [ ] **Step 2: Populate it in `NewModel`**

In `internal/tui/model.go`, in the `Model{...}` literal returned by `NewModel`, next to `seen:`/`bookmarks:`, add:

```go
		reviewed:      reviewedFromHistory(),
```

Then add this helper at the end of `model.go`:

```go
// reviewedFromHistory builds the set of PR URLs the user has posted a
// review on, from the persisted review history. Always non-nil.
func reviewedFromHistory() map[string]bool {
	set := map[string]bool{}
	for _, e := range cache.LoadHistory(0) {
		if e.PRURL != "" {
			set[e.PRURL] = true
		}
	}
	return set
}
```

- [ ] **Step 3: Mark reviewed live on `reviewDoneMsg`**

In `internal/tui/update.go`, in the `case reviewDoneMsg:` handler, inside the existing `if msg.err == nil && msg.reviewID != 0 {` block (right after the `cache.AppendHistory(...)` call), add:

```go
			if m.reviewed == nil {
				m.reviewed = map[string]bool{}
			}
			m.reviewed[msg.url] = true
```

- [ ] **Step 4: Verify build + tests green**

Run: `gofmt -l . ; go build ./... && go test ./internal/tui/ ./internal/cache/`
Expected: `gofmt -l .` prints nothing; build + tests PASS. (New field is written but not yet read — legal in Go.)

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w internal/tui/model.go internal/tui/update.go
gofmt -l .   # must print nothing
git add internal/tui/model.go internal/tui/update.go
git commit -m "feat(tui): track reviewed PRs from history"
```

---

### Task 3: Classifier + 2-column gutter + legend

**Files:**
- Modify: `internal/tui/view.go` (`prState` type, `prStateOf`, `listLines` gutter, `renderLegend`, header wiring, `titleW`)
- Test: `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: `Model.seen` (+ `Opened`, `IsNew`), `Model.reviewed`, `Model.bookmarks` (all existing/Task 2).
- Produces: `type prState struct{ fresh, updated, seen, reviewed, bookmarked bool }`; `func (m Model) prStateOf(pr github.PR) prState`.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/tui_test.go`. It needs `cache` and `time` imports — add `"time"` and `"github.com/KamilSupera/github-pullrequests-checkecker/internal/cache"` to that file's import block if not present.

```go
func TestPRStateOf(t *testing.T) {
	now := time.Now()
	m := Model{
		seen: &cache.SeenStore{URLs: map[string]time.Time{
			"updated": now.Add(-time.Hour), // opened before its latest update
			"quiet":   now.Add(time.Hour),  // opened after its latest update
		}},
		reviewed:  map[string]bool{"rev": true, "both": true},
		bookmarks: &cache.BookmarkStore{URLs: map[string]bool{"bm": true, "both": true}},
	}
	upd := now.Format(time.RFC3339)
	past := now.Add(-2 * time.Hour).Format(time.RFC3339)

	// Column 1: freshness (exactly one true)
	if s := m.prStateOf(github.PR{URL: "brandnew", UpdatedAt: upd}); !s.fresh || s.updated || s.seen {
		t.Errorf("brandnew: got %+v, want fresh only", s)
	}
	if s := m.prStateOf(github.PR{URL: "updated", UpdatedAt: upd}); !s.updated || s.fresh || s.seen {
		t.Errorf("updated: got %+v, want updated only", s)
	}
	if s := m.prStateOf(github.PR{URL: "quiet", UpdatedAt: past}); !s.seen || s.fresh || s.updated {
		t.Errorf("quiet: got %+v, want seen only", s)
	}

	// Column 2: marks
	if s := m.prStateOf(github.PR{URL: "rev", UpdatedAt: upd}); !s.reviewed || s.bookmarked {
		t.Errorf("rev: got %+v, want reviewed only", s)
	}
	if s := m.prStateOf(github.PR{URL: "bm", UpdatedAt: upd}); !s.bookmarked || s.reviewed {
		t.Errorf("bm: got %+v, want bookmarked only", s)
	}
	if s := m.prStateOf(github.PR{URL: "both", UpdatedAt: upd}); !s.reviewed || !s.bookmarked {
		t.Errorf("both: got %+v, want reviewed+bookmarked", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestPRStateOf -v`
Expected: FAIL — `m.prStateOf undefined` / `prState` undefined.

- [ ] **Step 3: Add `prState` + `prStateOf`**

In `internal/tui/view.go`, add near `isPRNew` (after the `isPRNew` method):

```go
// prState is the display state of one PR row. Exactly one of
// fresh/updated/seen is true; reviewed and bookmarked are independent.
type prState struct {
	fresh, updated, seen bool
	reviewed             bool
	bookmarked           bool
}

// prStateOf classifies a PR for the list gutter. Pure: reads only the
// seen/reviewed/bookmark stores.
func (m Model) prStateOf(pr github.PR) prState {
	var st prState
	switch {
	case m.seen == nil || !m.seen.Opened(pr.URL):
		st.fresh = true
	default:
		t, _ := time.Parse(time.RFC3339, pr.UpdatedAt)
		if m.seen.IsNew(pr.URL, t) {
			st.updated = true
		} else {
			st.seen = true
		}
	}
	st.reviewed = m.reviewed[pr.URL]
	st.bookmarked = m.bookmarks != nil && m.bookmarks.Has(pr.URL)
	return st
}
```

(`m.reviewed[pr.URL]` on a nil map returns false safely.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestPRStateOf -v`
Expected: PASS.

- [ ] **Step 5: Render the 2-column gutter**

In `internal/tui/view.go`, in `listLines`:

a) Remove the now-unused `newDot` declaration:

```go
	newDot := lipgloss.NewStyle().Foreground(colPeach).Bold(true).Render("•")
```

b) Widen the gutter budget — change:

```go
	titleW := paneW - 17
```
to:
```go
	titleW := paneW - 18 // extra column for the 2-char state gutter
```

c) Replace the marker computation block:

```go
			marker := " "
			if isNew {
				marker = newDot
			}
			if m.bookmarks != nil && m.bookmarks.Has(pr.URL) {
				marker = lipgloss.NewStyle().Foreground(colYellow).Render("★")
			}
```
with:
```go
			st := m.prStateOf(pr)
			var col1 string
			switch {
			case st.fresh:
				col1 = lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render("✦")
			case st.updated:
				col1 = lipgloss.NewStyle().Foreground(colPeach).Bold(true).Render("•")
			default: // seen
				col1 = dim.Render("·")
			}
			var col2 string
			switch {
			case st.bookmarked:
				col2 = lipgloss.NewStyle().Foreground(colYellow).Render("★")
			case st.reviewed:
				col2 = lipgloss.NewStyle().Foreground(colGreen).Render("✓")
			default:
				col2 = " "
			}
			marker := col1 + col2
```

The `fmt.Sprintf("%s%s%s %s %s", prefix, marker, date, num, title)` line stays as-is (marker is now 2 visible chars). `isNew` remains used by the text-color branches below — do not remove it.

- [ ] **Step 6: Add the legend + wire it into the header**

In `internal/tui/view.go`, add a `renderLegend` method (e.g. after `renderTabs`):

```go
// renderLegend is the one-line key for the list gutter glyphs.
func (m Model) renderLegend() string {
	c := func(col lipgloss.Color, s string) string {
		return lipgloss.NewStyle().Foreground(col).Render(s)
	}
	return "  " + strings.Join([]string{
		c(colBlue, "✦") + hint.Render(" new"),
		c(colPeach, "•") + hint.Render(" upd"),
		dim.Render("·") + hint.Render(" seen"),
		c(colGreen, "✓") + hint.Render(" reviewed"),
		c(colYellow, "★") + hint.Render(" saved"),
	}, "  ")
}
```

Then change the header assignment (currently `header := m.renderTabs()`, around line 117) to:

```go
	header := m.renderTabs() + "\n" + m.renderLegend()
```

- [ ] **Step 7: Verify build, tests, gofmt**

Run: `gofmt -l . ; go build ./... && go test ./...`
Expected: `gofmt -l .` prints nothing; build + all tests PASS.

- [ ] **Step 8: Manual smoke check (optional but recommended)**

Run: `go run ./cmd/prcheck` (requires `gh` auth). Confirm the list shows the 2-char gutter and the legend line under the tabs. Press `q` to quit. If you cannot authenticate, skip and note it.

- [ ] **Step 9: gofmt + commit**

```bash
gofmt -w internal/tui/view.go internal/tui/tui_test.go
gofmt -l .   # must print nothing
git add internal/tui/view.go internal/tui/tui_test.go
git commit -m "feat(tui): 2-column PR-list state gutter with legend"
```

---

### Task 4: Docs — README indicators legend

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: the gutter glyphs from Task 3.
- Produces: user-facing documentation of the list indicators.

- [ ] **Step 1: Document the gutter**

In `README.md`, find where the PR list / keybindings are described (search for the keybinding table containing `r` / `Refresh`, or the "Use" section). Add a short subsection near it, matching the surrounding style:

```
**List indicators** — each PR row starts with a two-character state gutter:

| glyph | meaning |
|-------|---------|
| `✦` | fresh — you have never opened this PR |
| `•` | updated — opened before, new activity since |
| `·` | seen — opened, nothing new |
| `✓` | you have posted a review on it |
| `★` | bookmarked |

The first column is always one of `✦`/`•`/`·`; the second shows `★` (or `✓`
if reviewed and not bookmarked).
```

- [ ] **Step 2: Verify the addition**

Run: `grep -n "List indicators\|fresh — you have never" README.md`
Expected: both present.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document PR-list state indicators"
```

---

## Self-Review

**Spec coverage:**
- Column 1 fresh/updated/seen from SeenStore (`Opened` + `IsNew`) → Task 1 (`Opened`) + Task 3 (`prStateOf`). ✓
- Column 2 bookmarked/reviewed priority → Task 3 render switch (bookmarked case first). ✓
- Reviewed set from history + live update → Task 2. ✓
- Testable classifier `prStateOf` returning a bool struct → Task 3 (+ test). ✓
- Legend line under the tab bar → Task 3 Step 6. ✓
- Row-text dimming unchanged (`isNew` kept) → Task 3 Step 5 note. ✓
- Gutter width compensation (`titleW` −1) → Task 3 Step 5b. ✓
- Docs → Task 4 (beyond spec; keeps the glyphs discoverable). ✓
- Out-of-scope (quick-review marking, config, 3rd column) correctly omitted. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code. ✓

**Type consistency:** `prState{ fresh, updated, seen, reviewed, bookmarked bool }` used identically in the test, `prStateOf`, and the render switches. `reviewed map[string]bool` written in `NewModel`/`reviewDoneMsg` (Task 2) and read in `prStateOf` (Task 3). `SeenStore.Opened(string) bool` defined in Task 1, called in Task 3. ✓

**Compile-safety ordering:** Task 1 self-contained. Task 2 adds a field written but not read (Go-legal) + a helper. Task 3 reads them and renders. Task 4 docs-only. Every commit builds and is gofmt-clean (explicit gofmt step in each). ✓

**Note on colors:** Fresh uses `colBlue` (cyan) to pop as "brand new"; updated keeps the existing `•`/`colPeach`. This is a deliberate contrast choice — the glyph shape (`✦`/`•`/`·`) is the primary signal, color is secondary.
