package tui

import (
	"context"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ksupera/prcheck/internal/pipeline"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height
		paneW, paneH := paneInnerSize(msg.Width, msg.Height)
		m.diffVP.Width = paneW
		m.diffVP.Height = paneH
		m.listH = paneH
		// detail box uses 8 inner rows by default; the rest goes to comments.
		ch := paneH - 2 - 8
		if ch < 3 {
			ch = 3
		}
		m.commentsH = ch
		m = m.scrollListIntoView()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case diffLoadedMsg:
		delete(m.loadingDiff, msg.url)
		if msg.err == nil {
			colored := colorizeDiff(msg.diff)
			m.diffs[msg.url] = colored
			if m.viewingDiff {
				m.diffVP.SetContent(colored)
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
		return m, nil

	case prDetailMsg:
		delete(m.loadingDetail, msg.url)
		if msg.err == nil {
			m.details[msg.url] = msg.detail
			m.commentsOffset = 0
		}
		return m, nil

	case progressMsg:
		rec := stepRec{step: msg.step, status: msg.status, note: msg.note, err: msg.err}
		// If we just got a "done|warn|skip|error" event for a step that
		// already has a "start" record, replace that record in-place so
		// we show one row per step.
		replaced := false
		for i := len(m.steps) - 1; i >= 0; i-- {
			if m.steps[i].step == rec.step && m.steps[i].status == "start" && (rec.status == "done" || rec.status == "warn" || rec.status == "error" || rec.status == "skip") {
				m.steps[i] = rec
				replaced = true
				break
			}
		}
		if !replaced {
			m.steps = append(m.steps, rec)
		}
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
			m.commentsOffset = 0
		}
		m = m.scrollListIntoView()
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.commentsOffset = 0
		}
		m = m.scrollListIntoView()
		return m, nil

	case "J", "pgdown":
		m.commentsOffset += m.commentsH - 1
		m = m.clampCommentsOffset()
		return m, nil

	case "K", "pgup":
		m.commentsOffset -= m.commentsH - 1
		m = m.clampCommentsOffset()
		return m, nil

	case "g", "home":
		m.cursor = 0
		m.listOffset = 0
		return m, nil

	case "G", "end":
		prs := m.prsByTab[m.tab]
		if len(prs) > 0 {
			m.cursor = len(prs) - 1
		}
		m = m.scrollListIntoView()
		return m, nil

	case " ", "space":
		pr, ok := m.currentPR()
		if !ok {
			return m, nil
		}
		if _, cached := m.details[pr.URL]; cached {
			return m, nil
		}
		if m.loadingDetail[pr.URL] {
			return m, nil
		}
		m.loadingDetail[pr.URL] = true
		return m, tea.Batch(m.spinner.Tick, m.loadDetail(pr.URL))

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
		m.loadingDiff[pr.URL] = true
		m.diffVP.SetContent(spinnerLine(m.spinner.View(), "loading diff..."))
		return m, tea.Batch(m.spinner.Tick, m.loadDiff(pr.URL))

	case "tab":
		m.tab = (m.tab + 1) % 3
		m.cursor = 0
		m.listOffset = 0
		m.commentsOffset = 0
		return m, nil

	case "shift+tab":
		m.tab = (m.tab + 2) % 3
		m.cursor = 0
		m.listOffset = 0
		m.commentsOffset = 0
		return m, nil

	case "r":
		// refresh current tab
		m.prsByTab[m.tab] = nil
		m.cursor = 0
		m.listOffset = 0
		m.commentsOffset = 0
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

// clampCommentsOffset keeps m.commentsOffset within [0, maxOffset]
// where maxOffset is total comment lines minus the visible window.
func (m Model) clampCommentsOffset() Model {
	full := m.renderComments()
	if full == "" {
		m.commentsOffset = 0
		return m
	}
	total := strings.Count(full, "\n") + 1
	visible := m.commentsH - 1
	if visible < 1 {
		visible = 1
	}
	max := total - visible
	if max < 0 {
		max = 0
	}
	if m.commentsOffset > max {
		m.commentsOffset = max
	}
	if m.commentsOffset < 0 {
		m.commentsOffset = 0
	}
	return m
}

// scrollListIntoView adjusts m.listOffset so the cursor's display line
// is within the visible window. Visible window is listH normally, or
// listH-1 when content overflows (the last row hosts the scroll
// indicator and must stay reserved).
func (m Model) scrollListIntoView() Model {
	if m.listH <= 0 {
		return m
	}
	lines, cursorLine := m.listLines()
	if len(lines) == 0 {
		m.listOffset = 0
		return m
	}
	visible := m.listH
	if len(lines) > m.listH {
		visible = m.listH - 1
		if visible < 1 {
			visible = 1
		}
	}
	if cursorLine < m.listOffset {
		m.listOffset = cursorLine
	}
	if cursorLine >= m.listOffset+visible {
		m.listOffset = cursorLine - visible + 1
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
		res, err := m.runPipe(ctx, prURL, func(e pipeline.Event) {
			// no-op here; main.go's wrappedEmit forwards events via Program.Send
		})
		msg := reviewDoneMsg{url: prURL, err: err}
		if res != nil {
			msg.reviewID = res.ID
			msg.summary = res.Summary
			msg.comments = res.Comments
		}
		return msg
	}
}
