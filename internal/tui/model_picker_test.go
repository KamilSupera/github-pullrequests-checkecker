package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/claude"
	"github.com/KamilSupera/github-pullrequests-checkecker/internal/pipeline"
)

func TestModelPicker_EnterOpensBeforeReview(t *testing.T) {
	t.Cleanup(func() { claude.SelectModel("default") })
	m := populatedModel(120, 40)

	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if !m.selectingModel {
		t.Fatal("Enter should open the model picker, not start the review")
	}
	if m.running || cmd != nil {
		t.Error("review must not start before a model is picked")
	}
	if m.pendingReview != "u" {
		t.Errorf("pendingReview = %q, want %q", m.pendingReview, "u")
	}
}

func TestModelPicker_EscCancels(t *testing.T) {
	t.Cleanup(func() { claude.SelectModel("default") })
	m := populatedModel(120, 40)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if m.selectingModel || m.pendingReview != "" || m.running {
		t.Error("Esc should close the picker without starting a review")
	}
}

func TestModelPicker_SelectStartsReview(t *testing.T) {
	t.Cleanup(func() { claude.SelectModel("default") })
	m := populatedModel(120, 40)
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.runPipe = func(ctx context.Context, url string, emit func(pipeline.Event)) (*pipeline.Result, error) {
		return &pipeline.Result{ID: 1}, nil
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)

	// Move to the second entry ("sonnet" for claude) and select it.
	nm, _ = m.Update(key("j"))
	m = nm.(Model)
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)

	if m.selectingModel {
		t.Error("picker should close after selecting")
	}
	if !m.running || cmd == nil {
		t.Error("review should start after picking a model")
	}
	if got := claude.ModelName(); got != claude.Models()[1] {
		t.Errorf("model = %q, want %q", got, claude.Models()[1])
	}
}
