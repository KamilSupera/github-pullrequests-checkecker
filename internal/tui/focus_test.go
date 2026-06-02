package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/github"
)

// populatedModel builds a Model with one loaded PR + detail so the
// split detail/comments view renders, sized for the given terminal.
func populatedModel(termW, termH int) Model {
	m := Model{termW: termW, termH: termH}
	_, paneH := paneInnerSize(termW, termH)
	m.detailH = paneH * 6 / 10
	if m.detailH < 10 {
		m.detailH = 10
	}
	m.commentsH = paneH - 2 - m.detailH
	if m.commentsH < 4 {
		m.commentsH = 4
	}
	m.listH = paneH
	m.resultH = paneH
	m.prsByTab = map[Tab][]github.PR{TabMine: {{Number: 1, Title: "X", URL: "u"}}}
	d := &github.PRDetail{}
	d.PR.Title = "X"
	m.details = map[string]*github.PRDetail{"u": d}
	return m
}

// Issue 1 regression: the rendered View must never exceed the terminal
// height, even when a status message and/or active filter add footer rows.
func TestView_NeverExceedsTermHeight(t *testing.T) {
	for _, termH := range []int{40, 30, 24, 20} {
		base := func() Model { return populatedModel(120, termH) }
		cases := map[string]Model{
			"plain":  base(),
			"status": func() Model { m := base(); m.statusMsg = "approving..."; return m }(),
			"filter": func() Model { m := base(); m.filter = "feat"; return m }(),
			"both":   func() Model { m := base(); m.statusMsg = "x"; m.filter = "y"; return m }(),
		}
		for name, m := range cases {
			if h := lipgloss.Height(m.View()); h > termH {
				t.Errorf("termH=%d %s: View height %d exceeds terminal height %d", termH, name, h, termH)
			}
		}
	}
}

func TestFocus_CycleKeys(t *testing.T) {
	m := populatedModel(120, 40)
	if m.focus != focusList {
		t.Fatalf("default focus = %v, want focusList", m.focus)
	}
	step := func(key string) {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = nm.(Model)
	}
	step("l")
	if m.focus != focusDetail {
		t.Errorf("after l: focus = %v, want focusDetail", m.focus)
	}
	step("l")
	if m.focus != focusComments {
		t.Errorf("after l l: focus = %v, want focusComments", m.focus)
	}
	step("l")
	if m.focus != focusList {
		t.Errorf("after l l l: focus = %v, want wrap to focusList", m.focus)
	}
	step("h")
	if m.focus != focusComments {
		t.Errorf("after h: focus = %v, want focusComments (wrap back)", m.focus)
	}
}

func TestFocus_JKRoutesToFocusedPane(t *testing.T) {
	m := populatedModel(120, 40)

	// focusList: j moves the list cursor, not the detail offset.
	m.prsByTab[TabMine] = []github.PR{
		{Number: 1, Title: "A", URL: "u1"},
		{Number: 2, Title: "B", URL: "u2"},
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = nm.(Model)
	if m.cursor != 1 {
		t.Errorf("focusList j: cursor = %d, want 1", m.cursor)
	}
	if m.detailOffset != 0 {
		t.Errorf("focusList j: detailOffset = %d, want 0", m.detailOffset)
	}

	// focusDetail: j scrolls detail, leaves the cursor put. Give the
	// detail body enough lines that there is room to scroll.
	d := &github.PRDetail{}
	d.PR.Title = "A"
	for i := 0; i < 80; i++ {
		d.Body += "line of description text\n"
	}
	m.details["u1"] = d
	m.focus = focusDetail
	m.cursor = 0
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = nm.(Model)
	if m.cursor != 0 {
		t.Errorf("focusDetail j: cursor moved to %d, want 0", m.cursor)
	}
	if m.detailOffset != 1 {
		t.Errorf("focusDetail j: detailOffset = %d, want 1", m.detailOffset)
	}
}
