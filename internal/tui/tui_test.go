package tui

import (
	"context"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/cache"
	"github.com/KamilSupera/github-pullrequests-checkecker/internal/github"
	"github.com/KamilSupera/github-pullrequests-checkecker/internal/pipeline"
)

func TestModel_ShowsMineTab(t *testing.T) {
	loader := func(ctx context.Context, q github.Query) ([]github.PR, error) {
		if q == github.QueryAuthored {
			return []github.PR{
				{Number: 1, Title: "First", URL: "https://example/pull/1"},
				{Number: 2, Title: "Second", URL: "https://example/pull/2"},
			}, nil
		}
		return nil, nil
	}
	detail := func(ctx context.Context, url string) (*github.PRDetail, error) {
		d := &github.PRDetail{}
		d.PR.Title = "First"
		d.PR.HeadRefName = "feat/x"
		d.PR.BaseRefName = "main"
		d.PR.Author = "kamil"
		return d, nil
	}
	diff := func(ctx context.Context, url string) (string, error) {
		return "diff --git a/x b/x\n+stub\n", nil
	}
	checks := func(ctx context.Context, url string) (string, error) {
		return "build  pass  https://x", nil
	}
	quickReview := func(ctx context.Context, url, event, body string) (int64, error) {
		return 99, nil
	}
	runPipe := func(ctx context.Context, url string, emit func(pipeline.Event)) (*pipeline.Result, error) {
		return &pipeline.Result{ID: 7}, nil
	}
	open := func(url string) error { return nil }

	m := NewModel(loader, detail, diff, checks, quickReview, runPipe, open, 5)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return containsAll(string(out), "Mine", "#1 First", "#2 Second")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
	_, _ = io.ReadAll(tm.FinalOutput(t))
}

func containsAll(s string, needles ...string) bool {
	for _, n := range needles {
		if !contains(s, n) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

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
