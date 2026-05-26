package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ksupera/prcheck/internal/cache"
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
		m.checksVP.Width = paneW
		m.checksVP.Height = paneH
		m.statsVP.Width = paneW * 2
		m.statsVP.Height = paneH
		m.listH = paneH
		// Detail box gets ~60% of the right column; the rest is comments.
		detailH := paneH * 6 / 10
		if detailH < 10 {
			detailH = 10
		}
		ch := paneH - 2 - detailH
		if ch < 4 {
			ch = 4
		}
		m.commentsH = ch
		m.detailH = detailH
		m.resultH = paneH
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

	case quickReviewDoneMsg:
		if msg.err != nil {
			m.statusMsg = "quick-review failed: " + msg.err.Error()
		} else {
			m.statusMsg = fmt.Sprintf("%s posted as review #%d", msg.event, msg.id)
		}
		return m, nil

	case checksLoadedMsg:
		if msg.err == nil {
			m.checks[msg.url] = msg.checks
			if m.viewingChecks {
				m.checksVP.SetContent(msg.checks)
				m.checksVP.GotoTop()
			}
		} else if m.viewingChecks {
			m.checksVP.SetContent(errStyle.Render("checks failed: ") + msg.err.Error())
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
			cache.Save(tabCacheKey(msg.tab), msg.prs)
		}
		m = m.scrollListIntoView()
		return m, nil

	case prDetailMsg:
		delete(m.loadingDetail, msg.url)
		if msg.err == nil {
			m.details[msg.url] = msg.detail
			m.commentsOffset = 0
			m.detailOffset = 0
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
		if msg.err == nil && msg.reviewID != 0 {
			cache.AppendHistory(cache.HistoryEntry{
				When:     time.Now(),
				PRURL:    msg.url,
				ReviewID: msg.reviewID,
				Summary:  msg.summary,
				Comments: len(msg.comments),
			})
		}
		return m, nil

	}

	// pass through to spinner
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Filter input mode captures most keystrokes for typing the query.
	if m.filtering {
		switch key {
		case "esc":
			m.filtering = false
			m.filter = ""
			m.cursor = 0
			return m, nil
		case "enter":
			m.filtering = false
			return m, nil
		case "backspace":
			if len(m.filter) > 0 {
				r := []rune(m.filter)
				m.filter = string(r[:len(r)-1])
				m.cursor = 0
			}
			return m, nil
		case "ctrl+u":
			m.filter = ""
			m.cursor = 0
			return m, nil
		default:
			if len(key) == 1 {
				m.filter += key
				m.cursor = 0
			}
			return m, nil
		}
	}

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
		case "n":
			if pr, ok := m.currentPR(); ok {
				if diff, cached := m.diffs[pr.URL]; cached {
					boundaries := fileBoundaries(diff)
					cur := m.diffVP.YOffset
					for _, b := range boundaries {
						if b > cur {
							m.diffVP.SetYOffset(b)
							return m, nil
						}
					}
				}
			}
			return m, nil
		case "p":
			if pr, ok := m.currentPR(); ok {
				if diff, cached := m.diffs[pr.URL]; cached {
					boundaries := fileBoundaries(diff)
					cur := m.diffVP.YOffset
					target := 0
					for _, b := range boundaries {
						if b < cur {
							target = b
						} else {
							break
						}
					}
					m.diffVP.SetYOffset(target)
				}
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.diffVP, cmd = m.diffVP.Update(msg)
		return m, cmd
	}

	if m.viewingChecks {
		switch key {
		case "c", "esc":
			m.viewingChecks = false
			return m, nil
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.checksVP, cmd = m.checksVP.Update(msg)
		return m, cmd
	}

	if m.viewingStats {
		switch key {
		case "s", "esc":
			m.viewingStats = false
			return m, nil
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.statsVP, cmd = m.statsVP.Update(msg)
		return m, cmd
	}

	// When the review-result pane is on screen for the current PR,
	// j/k scroll it; Enter re-reviews; Esc dismisses.
	if m.inResultView() {
		switch key {
		case "esc":
			m.lastReview = nil
			m.steps = nil
			m.resultOffset = 0
			return m, nil
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "j", "down":
			m.resultOffset++
			m = m.clampResultOffset()
			return m, nil
		case "k", "up":
			m.resultOffset--
			if m.resultOffset < 0 {
				m.resultOffset = 0
			}
			return m, nil
		case "J", "pgdown":
			m.resultOffset += m.resultH - 1
			m = m.clampResultOffset()
			return m, nil
		case "K", "pgup":
			m.resultOffset -= m.resultH - 1
			if m.resultOffset < 0 {
				m.resultOffset = 0
			}
			return m, nil
		case "g", "home":
			m.resultOffset = 0
			return m, nil
		case "G", "end":
			m.resultOffset = 1 << 20
			m = m.clampResultOffset()
			return m, nil
		case "o":
			pr, ok := m.currentPR()
			if !ok {
				return m, nil
			}
			_ = m.openURL(pr.URL)
			return m, nil
		case "enter":
			// fall through to the standard enter handler below by clearing
			// the lastReview so re-review is initiated.
			pr, ok := m.currentPR()
			if !ok {
				return m, nil
			}
			ctx, cancel := context.WithCancel(m.ctx)
			m.pipeCancel = cancel
			m.running = true
			m.steps = nil
			m.lastReview = nil
			m.resultOffset = 0
			return m, m.runPipeline(ctx, pr.URL)
		}
		return m, nil
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
			m.detailOffset = 0
		}
		m = m.scrollListIntoView()
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.commentsOffset = 0
			m.detailOffset = 0
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

	case "]":
		m.detailOffset += m.detailH - 1
		m = m.clampDetailOffset()
		return m, nil

	case "[":
		m.detailOffset -= m.detailH - 1
		m = m.clampDetailOffset()
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

	case "esc":
		// First esc clears an active filter; second dismisses the result view.
		if m.filter != "" {
			m.filter = ""
			m.cursor = 0
			return m, nil
		}
		m.lastReview = nil
		m.steps = nil
		return m, nil

	case " ", "space":
		pr, ok := m.currentPR()
		if !ok {
			return m, nil
		}
		if m.seen != nil {
			m.seen.Mark(pr.URL)
			m.seen.Save()
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

	case "a":
		pr, ok := m.currentPR()
		if !ok {
			return m, nil
		}
		m.statusMsg = "approving..."
		return m, m.quickReviewCmd(pr.URL, "APPROVE", "LGTM")

	case "s":
		m.viewingStats = true
		paneW, _ := paneInnerSize(m.termW, m.termH)
		m.statsVP.SetContent(renderStats(m, paneW))
		m.statsVP.GotoTop()
		return m, nil

	case "b":
		pr, ok := m.currentPR()
		if !ok || m.bookmarks == nil {
			return m, nil
		}
		on := m.bookmarks.Toggle(pr.URL)
		m.bookmarks.Save()
		if on {
			m.statusMsg = "bookmarked"
		} else {
			m.statusMsg = "removed bookmark"
		}
		return m, nil

	case "c":
		pr, ok := m.currentPR()
		if !ok {
			return m, nil
		}
		m.viewingChecks = true
		if c, cached := m.checks[pr.URL]; cached {
			m.checksVP.SetContent(c)
			m.checksVP.GotoTop()
			return m, nil
		}
		m.checksVP.SetContent(spinnerLine(m.spinner.View(), "loading checks..."))
		return m, tea.Batch(m.spinner.Tick, m.loadChecks(pr.URL))

	case "tab":
		m.tab = (m.tab + 1) % 3
		m.cursor = 0
		m.listOffset = 0
		m.commentsOffset = 0
		m.detailOffset = 0
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

	case "/":
		m.filtering = true
		return m, nil

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

// inResultView reports whether the right column is currently showing
// the review-result panel for the highlighted PR.
func (m Model) inResultView() bool {
	if m.viewingDiff || m.running || m.lastReview == nil {
		return false
	}
	pr, ok := m.currentPR()
	return ok && pr.URL == m.lastReview.url
}

// clampResultOffset keeps m.resultOffset within [0, totalLines-visible].
func (m Model) clampResultOffset() Model {
	if m.lastReview == nil {
		m.resultOffset = 0
		return m
	}
	full := m.renderReviewResult()
	total := strings.Count(full, "\n") + 1
	visible := m.resultH - 1
	if visible < 1 {
		visible = 1
	}
	max := total - visible
	if max < 0 {
		max = 0
	}
	if m.resultOffset > max {
		m.resultOffset = max
	}
	if m.resultOffset < 0 {
		m.resultOffset = 0
	}
	return m
}

// clampDetailOffset keeps m.detailOffset within [0, totalLines-visible].
func (m Model) clampDetailOffset() Model {
	full := m.renderDetailFull()
	if full == "" {
		m.detailOffset = 0
		return m
	}
	total := strings.Count(full, "\n") + 1
	visible := m.detailH - 1
	if visible < 1 {
		visible = 1
	}
	max := total - visible
	if max < 0 {
		max = 0
	}
	if m.detailOffset > max {
		m.detailOffset = max
	}
	if m.detailOffset < 0 {
		m.detailOffset = 0
	}
	return m
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

func (m Model) loadChecks(url string) tea.Cmd {
	return func() tea.Msg {
		c, err := m.checksFn(m.ctx, url)
		return checksLoadedMsg{url: url, checks: c, err: err}
	}
}

func (m Model) quickReviewCmd(prURL, event, body string) tea.Cmd {
	return func() tea.Msg {
		id, err := m.quickReview(m.ctx, prURL, event, body)
		return quickReviewDoneMsg{url: prURL, event: event, id: id, err: err}
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
			msg.aspects = res.Aspects
			msg.comments = res.Comments
		}
		return msg
	}
}
