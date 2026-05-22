package tui

import (
	"github.com/ksupera/prcheck/internal/github"
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
	step string
	note string
	err  error
}

type reviewDoneMsg struct {
	url      string
	reviewID int64
	err      error
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
