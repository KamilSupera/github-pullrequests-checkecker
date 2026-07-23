# Watch Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Watch mode to the prcheck TUI that auto-refreshes all PR tabs on a timer and fires desktop notifications on new PRs, re-requested reviews, and updates.

**Architecture:** Reuse the existing `cache.Notify` / `cache.DetectChanges` / `PRCHECK_NOTIFY` machinery. Add a self-rescheduling `tea.Tick` loop guarded by a monotonic generation counter, toggled at runtime with the `w` key. Interval comes from a new `PRCHECK_WATCH` env var. Notification wording is enriched to distinguish new-in-Review (= review requested) from new-elsewhere and from plain updates.

**Tech Stack:** Go 1.26, Bubble Tea (`github.com/charmbracelet/bubbletea`), existing `internal/cache`, `internal/config`, `internal/tui`, `internal/github` packages.

## Global Constraints

- Go module: `github.com/KamilSupera/github-pullrequests-checkecker`, `go 1.26.3`.
- No new dependencies. `tea.Tick` ships with the already-vendored bubbletea.
- Notifications stay gated behind `PRCHECK_NOTIFY=1` (unchanged).
- Watch mode starts **off**; `w` toggles it. Interval default 5 minutes, clamp to ≥1.
- Every commit must compile (`go build ./...`) and pass `go test ./...`.
- Follow existing style: env config in `internal/config`, best-effort notifications, no error surfacing for notify failures.

---

### Task 1: Config — `PRCHECK_WATCH`

**Files:**
- Modify: `internal/config/env.go`
- Test: `internal/config/env_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Config.WatchMinutes int` — parsed from `PRCHECK_WATCH` (integer minutes), default 5, values <1 clamp to 5.

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/env_test.go`:

```go
func TestLoad_WatchDefault(t *testing.T) {
	t.Setenv("PRCHECK_WATCH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.WatchMinutes != 5 {
		t.Errorf("WatchMinutes = %d, want default 5", cfg.WatchMinutes)
	}
}

func TestLoad_WatchCustom(t *testing.T) {
	t.Setenv("PRCHECK_WATCH", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.WatchMinutes != 10 {
		t.Errorf("WatchMinutes = %d, want 10", cfg.WatchMinutes)
	}
}

func TestLoad_WatchClampsBelowOne(t *testing.T) {
	t.Setenv("PRCHECK_WATCH", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.WatchMinutes != 5 {
		t.Errorf("WatchMinutes = %d, want clamp to 5", cfg.WatchMinutes)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoad_Watch -v`
Expected: FAIL — `cfg.WatchMinutes` undefined (compile error).

- [ ] **Step 3: Implement config parsing**

In `internal/config/env.go`, add `"strconv"` to the import block. Add the field to `Config`:

```go
	// Agent selects the backend model CLI: "claude" (default) or "cursor".
	Agent string

	// WatchMinutes is the auto-refresh interval for Watch mode, in
	// minutes. Read from PRCHECK_WATCH; defaults to 5, clamped to >=1.
	WatchMinutes int
```

In `Load`, after the `Focus` parsing block and before `return cfg, nil`:

```go
	cfg.WatchMinutes = 5
	if w := strings.TrimSpace(os.Getenv("PRCHECK_WATCH")); w != "" {
		if n, err := strconv.Atoi(w); err == nil && n >= 1 {
			cfg.WatchMinutes = n
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all config tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/env.go internal/config/env_test.go
git commit -m "feat(config): add PRCHECK_WATCH interval"
```

---

### Task 2: Model fields + `NewModel` param

**Files:**
- Modify: `internal/tui/model.go:52-127` (struct fields), `internal/tui/model.go:129` (`NewModel` signature + init)
- Modify: `cmd/prcheck/main.go:100` (call site)
- Modify: `internal/tui/tui_test.go:48` (call site)

**Interfaces:**
- Consumes: `Config.WatchMinutes` from Task 1.
- Produces: `Model` fields `watching bool`, `watchMin int`, `watchGen int`, `seededNotify map[Tab]bool`; `NewModel(..., watchMin int) Model` (new trailing param).

Unused struct fields compile cleanly in Go, so this task builds and passes on its own even though the fields aren't read until Tasks 3–4.

- [ ] **Step 1: Add struct fields**

In `internal/tui/model.go`, inside the `Model` struct (e.g. right after the `spinner spinner.Model` / `err error` group near line 125, or anywhere in the struct), add:

```go
	// Watch mode: periodic auto-refresh + desktop notifications.
	watching     bool          // is watch currently on
	watchMin     int           // tick interval in minutes (>=1)
	watchGen     int           // generation guard against duplicate tickers
	seededNotify map[Tab]bool  // tab has had its first live load (baseline)
```

- [ ] **Step 2: Change `NewModel` signature and init**

At `internal/tui/model.go:129`, change the signature:

```go
func NewModel(loader loaderFn, df detailFn, dfn diffFn, cf checksFn, qr quickReviewFn, rp runPipelineFn, open openFn, watchMin int) Model {
```

In the returned `Model{...}` literal, add these fields (place near `seen:`/`bookmarks:`):

```go
		watchMin:     watchMin,
		seededNotify: map[Tab]bool{},
```

- [ ] **Step 3: Update the two call sites**

`cmd/prcheck/main.go:100` — pass the config value:

```go
	model := tui.NewModel(loader, detailFn, diffFn, checksFn, quickReviewFn, runPipe, openInBrowser, cfg.WatchMinutes)
```

`internal/tui/tui_test.go:48` — pass a fixed interval:

```go
	m := NewModel(loader, detail, diff, checks, quickReview, runPipe, open, 5)
```

- [ ] **Step 4: Run build + tests to verify green**

Run: `go build ./... && go test ./internal/tui/ ./cmd/... -v`
Expected: PASS (existing tui tests still pass; new fields unused but compile).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go cmd/prcheck/main.go internal/tui/tui_test.go
git commit -m "feat(tui): add watch-mode model fields"
```

---

### Task 3: Ticker + `w` toggle + footer

**Files:**
- Modify: `internal/tui/messages.go` (new msg type)
- Modify: `internal/tui/update.go` (tickCmd helper, `watchTickMsg` case, `w` key case)
- Modify: `internal/tui/view.go:218-232` (footer item)
- Test: `internal/tui/tui_test.go`

**Interfaces:**
- Consumes: `Model.watching`, `Model.watchGen`, `Model.watchMin` (Task 2); existing `Model.loadTab(t Tab) tea.Cmd`, `Tab` constants.
- Produces: `watchTickMsg{gen int}`, `tickCmd(gen, min int) tea.Cmd`; `w` keybinding toggles `m.watching`.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/tui_test.go`:

```go
func TestWatchToggleAndStaleTick(t *testing.T) {
	noPRs := func(ctx context.Context, q github.Query) ([]github.PR, error) { return nil, nil }
	noDetail := func(ctx context.Context, url string) (*github.PRDetail, error) { return &github.PRDetail{}, nil }
	noDiff := func(ctx context.Context, url string) (string, error) { return "", nil }
	noChecks := func(ctx context.Context, url string) (string, error) { return "", nil }
	noQR := func(ctx context.Context, url, event, body string) (int64, error) { return 0, nil }
	noPipe := func(ctx context.Context, url string, emit func(pipeline.Event)) (*pipeline.Result, error) {
		return nil, nil
	}
	noOpen := func(url string) error { return nil }

	m := NewModel(noPRs, noDetail, noDiff, noChecks, noQR, noPipe, noOpen, 5)

	// Pressing 'w' turns watch on and returns a tick command.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	mw := updated.(Model)
	if !mw.watching {
		t.Fatal("pressing w should enable watching")
	}
	if cmd == nil {
		t.Fatal("enabling watch should return a tick command")
	}

	// A tick from a stale generation is a no-op (no reschedule).
	_, staleCmd := mw.Update(watchTickMsg{gen: mw.watchGen - 1})
	if staleCmd != nil {
		t.Fatal("stale-gen tick should return nil command")
	}

	// Pressing 'w' again turns watch off.
	updated2, _ := mw.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if updated2.(Model).watching {
		t.Fatal("pressing w again should disable watching")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestWatchToggleAndStaleTick -v`
Expected: FAIL — `watchTickMsg` undefined (compile error).

- [ ] **Step 3: Add the message type**

In `internal/tui/messages.go`, add near the other message structs:

```go
// watchTickMsg fires on the watch-mode timer. gen guards against stale
// ticker loops surviving a watch off/on toggle.
type watchTickMsg struct{ gen int }
```

- [ ] **Step 4: Add tickCmd helper**

In `internal/tui/update.go` (top level, after the imports / before `Update`), add:

```go
// tickCmd schedules the next watch-mode refresh. gen is the generation
// this tick belongs to; the handler drops ticks whose gen is stale.
func tickCmd(gen, min int) tea.Cmd {
	return tea.Tick(time.Duration(min)*time.Minute, func(time.Time) tea.Msg {
		return watchTickMsg{gen: gen}
	})
}
```

- [ ] **Step 5: Handle `watchTickMsg`**

In `internal/tui/update.go`, add a case in the top-level `switch msg := msg.(type)` (e.g. right after the `prsLoadedMsg` case):

```go
	case watchTickMsg:
		// Drop stale ticks (watch was toggled off, or a newer loop
		// superseded this one). Otherwise refresh all tabs and
		// reschedule. Does NOT nil the lists, so no blank-flash.
		if msg.gen != m.watchGen || !m.watching {
			return m, nil
		}
		return m, tea.Batch(
			m.loadTab(TabMine),
			m.loadTab(TabReview),
			m.loadTab(TabMentioned),
			tickCmd(m.watchGen, m.watchMin),
		)
```

- [ ] **Step 6: Handle the `w` key**

In `internal/tui/update.go`, in the key-string switch, add a case adjacent to `case "r":` (around line 631):

```go
	case "w":
		m.watching = !m.watching
		if m.watching {
			m.watchGen++
			m.statusMsg = fmt.Sprintf("watch on (%dm)", m.watchMin)
			return m, tickCmd(m.watchGen, m.watchMin)
		}
		m.statusMsg = "watch off"
		return m, nil
```

- [ ] **Step 7: Add footer indicator**

In `internal/tui/view.go`, inside `renderFooter`'s `else` branch (the non-diff item list, ending around line 232), after the `items = []kh{...}` literal and before the `var parts []string` loop, append:

```go
		wLabel := "watch:off"
		if m.watching {
			wLabel = "watch:on"
		}
		items = append(items, kh{"w", wLabel})
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestWatchToggleAndStaleTick -v`
Expected: PASS.

- [ ] **Step 9: Run full package + build**

Run: `go build ./... && go test ./internal/tui/`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/tui/messages.go internal/tui/update.go internal/tui/view.go internal/tui/tui_test.go
git commit -m "feat(tui): watch-mode ticker and w toggle"
```

---

### Task 4: Notify — `DetectChanges` returns `[]Change`, enriched wording

**Files:**
- Modify: `internal/cache/notify.go:11-30` (`DetectChanges` + new `Change` type)
- Modify: `internal/cache/cache_test.go:126-146` (`TestDetectChanges`)
- Modify: `internal/tui/update.go:88-98` (notify block in `prsLoadedMsg`)

**Interfaces:**
- Consumes: `Model.seededNotify` (Task 2), `Model.prsByTab` (existing), `Tab.Label()` (existing), `cache.Notify` (existing).
- Produces: `cache.Change struct { PR github.PR; IsNew bool }`; `cache.DetectChanges(old *Snapshot, fresh []github.PR) []Change`.

This task changes `DetectChanges`'s return type **and** its only caller (`update.go`) in the same commit, so the tree never stops compiling.

- [ ] **Step 1: Rewrite the `DetectChanges` test**

In `internal/cache/cache_test.go`, replace the entire `TestDetectChanges` function (lines ~126 to its closing brace) with:

```go
func TestDetectChanges(t *testing.T) {
	if got := DetectChanges(nil, []github.PR{{URL: "a"}}); got != nil {
		t.Errorf("nil old snapshot = %v, want nil", got)
	}

	old := &Snapshot{PRs: []github.PR{
		{URL: "a", UpdatedAt: "t1"},
		{URL: "b", UpdatedAt: "t1"},
	}}
	fresh := []github.PR{
		{URL: "a", UpdatedAt: "t1"}, // unchanged -> omitted
		{URL: "b", UpdatedAt: "t2"}, // updated   -> IsNew=false
		{URL: "c", UpdatedAt: "t1"}, // new URL   -> IsNew=true
	}

	got := DetectChanges(old, fresh)
	if len(got) != 2 {
		t.Fatalf("DetectChanges = %v, want 2 entries", got)
	}
	isNew := map[string]bool{}
	present := map[string]bool{}
	for _, c := range got {
		present[c.PR.URL] = true
		isNew[c.PR.URL] = c.IsNew
	}
	if !present["b"] || !present["c"] {
		t.Fatalf("DetectChanges missing b/c: %v", got)
	}
	if present["a"] {
		t.Errorf("unchanged PR a should not be reported")
	}
	if isNew["b"] {
		t.Errorf("b changed UpdatedAt, IsNew should be false")
	}
	if !isNew["c"] {
		t.Errorf("c is a new URL, IsNew should be true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cache/ -run TestDetectChanges -v`
Expected: FAIL — `c.PR` undefined (`DetectChanges` still returns `[]string`).

- [ ] **Step 3: Change `DetectChanges` to return `[]Change`**

In `internal/cache/notify.go`, replace the `DetectChanges` function (lines 11-30) with:

```go
// Change describes one PR that differs from the previous snapshot.
// IsNew is true when the URL was absent from the old snapshot;
// otherwise the URL existed and its UpdatedAt changed.
type Change struct {
	PR    github.PR
	IsNew bool
}

// DetectChanges compares an old snapshot to a fresh PR list and returns
// the PRs that are new or freshly updated. Old==nil returns no changes
// (the very first launch shouldn't burst notifications).
func DetectChanges(old *Snapshot, fresh []github.PR) []Change {
	if old == nil {
		return nil
	}
	byURL := map[string]string{}
	for _, pr := range old.PRs {
		byURL[pr.URL] = pr.UpdatedAt
	}
	var out []Change
	for _, pr := range fresh {
		prev, seen := byURL[pr.URL]
		switch {
		case !seen:
			out = append(out, Change{PR: pr, IsNew: true})
		case pr.UpdatedAt != prev:
			out = append(out, Change{PR: pr, IsNew: false})
		}
	}
	return out
}
```

- [ ] **Step 4: Rewrite the notify block in `update.go`**

In `internal/tui/update.go`, replace the notify block inside `case prsLoadedMsg:` (currently lines ~87-97, the `if os.Getenv("PRCHECK_NOTIFY") == "1" { ... }` block) with:

```go
			// Compute deltas vs the in-memory list (pre-overwrite), then
			// fire desktop notifications if PRCHECK_NOTIFY=1. The first
			// live load per tab only seeds the baseline (no burst).
			if os.Getenv("PRCHECK_NOTIFY") == "1" {
				if !m.seededNotify[msg.tab] {
					m.seededNotify[msg.tab] = true
				} else {
					old := &cache.Snapshot{PRs: m.prsByTab[msg.tab]}
					notifyChanges(msg.tab, cache.DetectChanges(old, msg.prs))
				}
			}
```

- [ ] **Step 5: Add the `notifyChanges` helper**

In `internal/tui/update.go` (top level, near `tickCmd`), add:

```go
// notifyChanges fires desktop notifications for detected PR changes.
// A new URL in the Review tab means a review was (re-)requested. More
// than three changes collapse into a single summary to avoid a storm.
func notifyChanges(tab Tab, changes []cache.Change) {
	if len(changes) == 0 {
		return
	}
	if len(changes) > 3 {
		cache.Notify(
			fmt.Sprintf("prcheck — %s", tab.Label()),
			fmt.Sprintf("%d PRs changed", len(changes)),
		)
		return
	}
	for _, c := range changes {
		var title string
		switch {
		case c.IsNew && tab == TabReview:
			title = "Review requested"
		case c.IsNew:
			title = "New PR"
		default:
			title = "Updated"
		}
		cache.Notify(title, fmt.Sprintf("%s#%d %s", c.PR.Repo, c.PR.Number, c.PR.Title))
	}
}
```

- [ ] **Step 6: Run tests + build to verify green**

Run: `go build ./... && go test ./internal/cache/ ./internal/tui/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/cache/notify.go internal/cache/cache_test.go internal/tui/update.go
git commit -m "feat(tui): distinguish new PR, re-request, and update notifications"
```

---

### Task 5: Docs — README Watch mode

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: `PRCHECK_WATCH`, `PRCHECK_NOTIFY`, `w` key.
- Produces: user-facing documentation.

- [ ] **Step 1: Document watch mode**

In `README.md`, find the section listing env vars / keybindings (search for `PRCHECK_NOTIFY` or `PRCHECK_FOCUS`, and the key table containing `refresh`). Add:

Env var row/line:

```
PRCHECK_WATCH   Auto-refresh interval in minutes for Watch mode (default 5).
```

Keybinding row/line (next to `r  refresh`):

```
w   toggle Watch mode (auto-refresh + desktop notifications)
```

Add a short prose note near the notifications docs:

```
Watch mode (w) auto-refreshes all tabs every PRCHECK_WATCH minutes. With
PRCHECK_NOTIFY=1 it sends desktop notifications: "Review requested" when a
PR (re-)appears in the Review tab, "New PR" for new PRs elsewhere, and
"Updated" when a tracked PR changes.
```

- [ ] **Step 2: Verify the README references match reality**

Run: `grep -n "PRCHECK_WATCH\|toggle Watch\|Review requested" README.md`
Expected: the three additions are present.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document watch mode"
```

---

## Self-Review

**Spec coverage:**
- Config `PRCHECK_WATCH`, default 5, clamp ≥1 → Task 1. ✓
- Model fields `watching`/`watchMin`/`watchGen`/`seededNotify`, `NewModel` param, main passes `cfg.WatchMinutes` → Task 2. ✓
- `watchTickMsg`, `tickCmd`, `w` toggle with status, stale-gen guard, refresh-all-tabs-without-nil, footer `w watch:on|off` → Task 3. ✓
- `DetectChanges` → `[]Change{PR, IsNew}`; in-memory pre-overwrite compare; `seededNotify` gate; Review+IsNew="Review requested", other IsNew="New PR", else "Updated"; >3 summary; `PRCHECK_NOTIFY` gate unchanged → Task 4. ✓
- Tests: `DetectChanges` IsNew classification (Task 4), `w` toggle + stale tick no-op (Task 3), config parsing (Task 1). ✓
- Out-of-scope items (per-tab intervals, CI-status notify, sound) correctly omitted. ✓
- Docs → Task 5 (beyond spec, but keeps the feature discoverable). ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step shows complete code. ✓

**Type consistency:** `watchTickMsg{gen int}` used consistently in `tickCmd`, the handler, and the test. `cache.Change{PR github.PR; IsNew bool}` used in `notify.go`, `cache_test.go`, and `notifyChanges`. `NewModel(..., watchMin int)` matches both call sites and the test. `notifyChanges(tab Tab, changes []cache.Change)` signature matches its caller. ✓

**Compile-safety ordering:** Task 2 adds only unused fields (Go-legal). Task 3 reads those fields + existing `loadTab`. Task 4 changes `DetectChanges` and its sole caller in one commit. Every commit builds. ✓
