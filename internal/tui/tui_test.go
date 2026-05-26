package tui

import (
	"context"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/pipeline"
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
	runPipe := func(ctx context.Context, url string, emit func(pipeline.Event)) (*pipeline.Result, error) {
		return &pipeline.Result{ID: 7}, nil
	}
	open := func(url string) error { return nil }

	m := NewModel(loader, detail, diff, checks, runPipe, open)
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
