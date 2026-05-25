package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/pipeline"
)

// Messages flow from goroutines into the Bubble Tea Update function.

type prsLoadedMsg struct {
	tab Tab
	prs []github.PR
	err error
}

type prDetailMsg struct {
	url    string
	detail *github.PRDetail
	err    error
}

type progressMsg struct {
	step   string
	status string
	note   string
	err    error
}

type reviewDoneMsg struct {
	url      string
	reviewID int64
	err      error
}

type diffLoadedMsg struct {
	url  string
	diff string
	err  error
}

type Tab int

const (
	TabMine Tab = iota
	TabReview
	TabMentioned
)

func (t Tab) Label() string {
	switch t {
	case TabMine:
		return "Mine"
	case TabReview:
		return "Review"
	case TabMentioned:
		return "Mentioned"
	}
	return "?"
}

// ProgressFromEvent constructs the public progress message used by
// the main package to stream live pipeline events.
func ProgressFromEvent(e pipeline.Event) tea.Msg {
	return progressMsg{step: e.Step, status: e.Status, note: e.Note, err: e.Err}
}
