package tui

import (
	"context"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ksupera/prcheck/internal/pipeline"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		// Right pane is ~ half the width; subtract some chrome.
		w := msg.Width/2 - 4
		if w < 20 {
			w = 20
		}
		h := msg.Height - 6
		if h < 5 {
			h = 5
		}
		m.diffVP.Width = w
		m.diffVP.Height = h
		m.listH = h
		m = m.scrollListIntoView()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case diffLoadedMsg:
		if msg.err == nil {
			m.diffs[msg.url] = msg.diff
			if m.viewingDiff {
				m.diffVP.SetContent(msg.diff)
				m.diffVP.GotoTop()
			}
		}
		return m, nil

	case prsLoadedMsg:
		if msg.err != nil {
			m.loadErr[msg.tab] = msg.err
		} else {
			sort.Slice(msg.prs, func(i, j int) bool {
				return msg.prs[i].UpdatedAt > msg.prs[j].UpdatedAt
			})
			m.prsByTab[msg.tab] = msg.prs
		}
		m = m.scrollListIntoView()
		// preload detail for the first PR of current tab
		if pr, ok := m.currentPR(); ok {
			if _, cached := m.details[pr.URL]; !cached {
				return m, m.loadDetail(pr.URL)
			}
		}
		return m, nil

	case prDetailMsg:
		if msg.err == nil {
			m.details[msg.url] = msg.detail
		}
		return m, nil

	case progressMsg:
		m.steps = append(m.steps, msg.step)
		return m, nil

	case reviewDoneMsg:
		m.running = false
		m.pipeCancel = nil
		m.lastReview = &msg
		return m, nil

	}

	// pass through to spinner
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// When viewing a diff, j/k/pgup/pgdn scroll the viewport;
	// d or esc returns to the detail pane.
	if m.viewingDiff {
		switch key {
		case "d", "esc":
			m.viewingDiff = false
			return m, nil
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.diffVP, cmd = m.diffVP.Update(msg)
		return m, cmd
	}

	switch key {

	case "q", "ctrl+c":
		if m.running && m.pipeCancel != nil {
			m.pipeCancel()
			m.running = false
			return m, nil
		}
		m.cancel()
		return m, tea.Quit

	case "j", "down":
		prs := m.prsByTab[m.tab]
		if m.cursor < len(prs)-1 {
			m.cursor++
		}
		m = m.scrollListIntoView()
		if pr, ok := m.currentPR(); ok {
			if _, cached := m.details[pr.URL]; !cached {
				return m, m.loadDetail(pr.URL)
			}
		}
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		m = m.scrollListIntoView()
		if pr, ok := m.currentPR(); ok {
			if _, cached := m.details[pr.URL]; !cached {
				return m, m.loadDetail(pr.URL)
			}
		}
		return m, nil

	case "g", "home":
		m.cursor = 0
		m.listOffset = 0
		if pr, ok := m.currentPR(); ok {
			if _, cached := m.details[pr.URL]; !cached {
				return m, m.loadDetail(pr.URL)
			}
		}
		return m, nil

	case "G", "end":
		prs := m.prsByTab[m.tab]
		if len(prs) > 0 {
			m.cursor = len(prs) - 1
		}
		m = m.scrollListIntoView()
		if pr, ok := m.currentPR(); ok {
			if _, cached := m.details[pr.URL]; !cached {
				return m, m.loadDetail(pr.URL)
			}
		}
		return m, nil

	case "d":
		pr, ok := m.currentPR()
		if !ok {
			return m, nil
		}
		m.viewingDiff = true
		if diff, cached := m.diffs[pr.URL]; cached {
			m.diffVP.SetContent(diff)
			m.diffVP.GotoTop()
			return m, nil
		}
		m.diffVP.SetContent("loading diff...")
		return m, m.loadDiff(pr.URL)

	case "tab":
		m.tab = (m.tab + 1) % 3
		m.cursor = 0
		m.listOffset = 0
		return m, nil

	case "shift+tab":
		m.tab = (m.tab + 2) % 3
		m.cursor = 0
		m.listOffset = 0
		return m, nil

	case "r":
		// refresh current tab
		m.prsByTab[m.tab] = nil
		m.cursor = 0
		m.listOffset = 0
		return m, m.loadTab(m.tab)

	case "o":
		pr, ok := m.currentPR()
		if !ok {
			return m, nil
		}
		_ = m.openURL(pr.URL)
		return m, nil

	case "enter":
		if m.running {
			return m, nil
		}
		pr, ok := m.currentPR()
		if !ok {
			return m, nil
		}
		ctx, cancel := context.WithCancel(m.ctx)
		m.pipeCancel = cancel
		m.running = true
		m.steps = nil
		m.lastReview = nil
		return m, m.runPipeline(ctx, pr.URL)
	}

	return m, nil
}

// scrollListIntoView adjusts m.listOffset so the cursor's display line
// is within the visible window of m.listH lines.
func (m Model) scrollListIntoView() Model {
	if m.listH <= 0 {
		return m
	}
	lines, cursorLine := m.listLines()
	if len(lines) == 0 {
		m.listOffset = 0
		return m
	}
	if cursorLine < m.listOffset {
		m.listOffset = cursorLine
	}
	if cursorLine >= m.listOffset+m.listH {
		m.listOffset = cursorLine - m.listH + 1
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}
	return m
}

func (m Model) loadDetail(url string) tea.Cmd {
	return func() tea.Msg {
		d, err := m.detailFn(m.ctx, url)
		return prDetailMsg{url: url, detail: d, err: err}
	}
}

func (m Model) loadDiff(url string) tea.Cmd {
	return func() tea.Msg {
		d, err := m.diffFn(m.ctx, url)
		return diffLoadedMsg{url: url, diff: d, err: err}
	}
}

// runPipeline executes the pipeline synchronously and returns the
// final reviewDoneMsg. Live progress events are streamed through
// Program.Send (wired up in cmd/prcheck/main.go via ProgressFromEvent).
func (m Model) runPipeline(ctx context.Context, prURL string) tea.Cmd {
	return func() tea.Msg {
		id, err := m.runPipe(ctx, prURL, func(e pipeline.Event) {
			// no-op here; main.go's wrappedEmit forwards events via Program.Send
		})
		return reviewDoneMsg{url: prURL, reviewID: id, err: err}
	}
}
