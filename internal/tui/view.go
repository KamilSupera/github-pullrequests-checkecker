package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ksupera/prcheck/internal/github"
)

// Theme — Catppuccin Mocha-inspired palette. Hex colors degrade
// gracefully to 256-color when truecolor is unsupported.
var (
	colFG       = lipgloss.Color("#cdd6f4")
	colSubtext  = lipgloss.Color("#a6adc8")
	colOverlay  = lipgloss.Color("#6c7086")
	colMauve    = lipgloss.Color("#cba6f7")
	colBlue     = lipgloss.Color("#89b4fa")
	colTeal     = lipgloss.Color("#94e2d5")
	colGreen    = lipgloss.Color("#a6e3a1")
	colYellow   = lipgloss.Color("#f9e2af")
	colPeach    = lipgloss.Color("#fab387")
	colRed      = lipgloss.Color("#f38ba8")
	colPink     = lipgloss.Color("#f5c2e7")
	colSurface0 = lipgloss.Color("#313244")

	tabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(colFG).
			Background(colMauve).
			Padding(0, 1)
	tabInactive = lipgloss.NewStyle().
			Foreground(colSubtext).
			Padding(0, 1)
	errStyle    = lipgloss.NewStyle().Foreground(colRed).Bold(true)
	dim         = lipgloss.NewStyle().Foreground(colOverlay)
	pass        = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	fail        = lipgloss.NewStyle().Foreground(colRed).Bold(true)
	pending     = lipgloss.NewStyle().Foreground(colYellow)
	hint        = lipgloss.NewStyle().Foreground(colSubtext)
	repoHeader  = lipgloss.NewStyle().Bold(true).Foreground(colMauve)
	prNumStyle  = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	prTitle     = lipgloss.NewStyle().Foreground(colFG)
	authorTag   = lipgloss.NewStyle().Foreground(colPeach)
	keyCap      = lipgloss.NewStyle().
			Foreground(colFG).
			Background(colSurface0).
			Bold(true).
			Padding(0, 1)
	keyHelp = lipgloss.NewStyle().Foreground(colSubtext)

	borderColor       = colOverlay
	borderActiveColor = colMauve

	// Status badges (filled pills).
	badgeApproved = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#1e1e2e")).
			Background(colGreen).
			Padding(0, 1)
	badgeChanges = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#1e1e2e")).
			Background(colRed).
			Padding(0, 1)
	badgeCommented = lipgloss.NewStyle().
			Foreground(colFG).
			Background(colOverlay).
			Padding(0, 1)
)

// paneInnerSize returns the (width, height) of the CONTENT area inside
// each of the two panes given the full terminal size. Subtracts:
//   - 2 chars per pane for left+right border = 4 total
//   - rows for tab header (1) + footer (1) + breathing room (2) = 4
func paneInnerSize(termW, termH int) (w, h int) {
	w = termW/2 - 2
	if w < 20 {
		w = 20
	}
	h = termH - 4
	if h < 5 {
		h = 5
	}
	return
}

func (m Model) View() string {
	paneW, paneH := paneInnerSize(m.termW, m.termH)
	leftPane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderActiveColor).
		Width(paneW).
		Height(paneH)

	header := m.renderTabs()
	left := m.renderList()
	right := m.renderRightSplit(paneW, paneH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane.Render(left), right)

	footer := m.renderFooter()
	return strings.Join([]string{header, body, footer}, "\n")
}

// renderFooter shows keycap-style help, switching to a diff-specific
// set of keys when the diff viewer is up.
func (m Model) renderFooter() string {
	type kh struct{ key, help string }
	var items []kh
	if m.viewingDiff {
		items = []kh{
			{"j/k", "scroll"},
			{"PgUp/PgDn", "page"},
			{"d/Esc", "back"},
			{"q", "quit"},
		}
	} else {
		items = []kh{
			{"j/k", "move"},
			{"J/K", "comments"},
			{"Space", "detail"},
			{"d", "diff"},
			{"⏎", "review"},
			{"Tab", "tab"},
			{"o", "open"},
			{"r", "refresh"},
			{"q", "quit"},
		}
	}
	var parts []string
	for _, it := range items {
		parts = append(parts, keyCap.Render(it.key)+" "+keyHelp.Render(it.help))
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderTabs() string {
	var parts []string
	countStyle := lipgloss.NewStyle().Foreground(colYellow).Bold(true)
	for i := Tab(0); i < 3; i++ {
		name := i.Label()
		var count string
		if prs, loaded := m.prsByTab[i]; loaded {
			count = countStyle.Render(fmt.Sprintf("%d", len(prs)))
		} else if err := m.loadErr[i]; err != nil {
			_ = err
			count = errStyle.Render("!")
		} else {
			count = dim.Render("…")
		}
		label := fmt.Sprintf("%s %s", name, count)
		if i == m.tab {
			parts = append(parts, tabActive.Render(label))
		} else {
			parts = append(parts, tabInactive.Render(label))
		}
	}
	return strings.Join(parts, dim.Render(" │ "))
}

func (m Model) renderList() string {
	prs := m.prsByTab[m.tab]
	if err := m.loadErr[m.tab]; err != nil {
		return errStyle.Render("load error: ") + err.Error()
	}
	if prs == nil {
		return spinnerLine(m.spinner.View(), dim.Render("loading PRs..."))
	}
	if len(prs) == 0 {
		return dim.Render("(no PRs)")
	}

	lines, _ := m.listLines()
	overflow := len(lines) > m.listH
	maxRows := m.listH
	if overflow {
		// Reserve last row for the scroll indicator so the pane height
		// stays equal to listH (otherwise the bordered pane expands by
		// one row and pushes the tabs off the top of the screen).
		maxRows--
		if maxRows < 1 {
			maxRows = 1
		}
	}
	start := m.listOffset
	if start < 0 {
		start = 0
	}
	if start > len(lines) {
		start = len(lines)
	}
	end := start + maxRows
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[start:end]

	if overflow {
		up := " "
		if start > 0 {
			up = "↑"
		}
		down := " "
		if end < len(lines) {
			down = "↓"
		}
		visible = append(visible, dim.Render(fmt.Sprintf("  %s%s %d/%d", up, down, end, len(lines))))
	}
	return strings.Join(visible, "\n")
}

// listLines builds the full rendered PR list (including repo headers)
// and returns the display-line index of the cursor.
func (m Model) listLines() (lines []string, cursorLine int) {
	paneW, _ := paneInnerSize(m.termW, m.termH)
	titleW := paneW - 10
	if titleW < 10 {
		titleW = 10
	}
	prs := m.prsByTab[m.tab]
	groups := groupByRepo(prs)
	idx := 0
	cursorStyle := lipgloss.NewStyle().Foreground(colMauve).Bold(true)
	for _, g := range groups {
		lines = append(lines, repoHeader.Render("● "+truncate(g.name, paneW-4)))
		for _, pr := range g.prs {
			active := idx == m.cursor
			var prefix, num, title string
			if active {
				prefix = cursorStyle.Render("❯ ")
				num = prNumStyle.Render(fmt.Sprintf("#%d", pr.Number))
				title = prTitle.Bold(true).Render(truncate(pr.Title, titleW))
				cursorLine = len(lines)
			} else {
				prefix = "  "
				num = dim.Render(fmt.Sprintf("#%d", pr.Number))
				title = dim.Render(truncate(pr.Title, titleW))
			}
			lines = append(lines, fmt.Sprintf("%s%s %s", prefix, num, title))
			idx++
		}
	}
	return
}

type repoGroup struct {
	name string
	prs  []github.PR
}

// groupByRepo returns prs grouped by Repo, preserving the input order
// of PRs within each group. Group order is the order in which each
// repo first appeared in the slice — which, since prs is sorted by
// UpdatedAt desc, places the recently-updated repo first.
func groupByRepo(prs []github.PR) []repoGroup {
	idx := map[string]int{}
	var groups []repoGroup
	for _, pr := range prs {
		key := pr.Repo
		if key == "" {
			key = "(unknown)"
		}
		if i, ok := idx[key]; ok {
			groups[i].prs = append(groups[i].prs, pr)
			continue
		}
		idx[key] = len(groups)
		groups = append(groups, repoGroup{name: key, prs: []github.PR{pr}})
	}
	// Sort PRs within each group by UpdatedAt desc (re-sort because
	// caller's outer sort already did this — kept here for clarity).
	for i := range groups {
		sort.SliceStable(groups[i].prs, func(a, b int) bool {
			return groups[i].prs[a].UpdatedAt > groups[i].prs[b].UpdatedAt
		})
	}
	return groups
}

// renderRightSplit returns the right column as two stacked bordered
// boxes: detail on top, comments below. Total height equals paneH+2
// so it matches the left pane's outer height.
func (m Model) renderRightSplit(paneW, paneH int) string {
	if m.viewingDiff || m.running || m.lastReview != nil {
		fullPane := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderActiveColor).
			Width(paneW).
			Height(paneH)
		return fullPane.Render(m.renderRight())
	}

	detailH := 8
	commentsH := paneH - 2 - detailH
	if commentsH < 3 {
		detailH = paneH - 2 - 3
		if detailH < 3 {
			detailH = 3
		}
		commentsH = paneH - 2 - detailH
		if commentsH < 1 {
			commentsH = 1
		}
	}

	detailBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(paneW).
		Height(detailH).
		Render(m.renderDetail())

	commentsBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(paneW).
		Height(commentsH).
		Render(m.renderCommentsWindow(commentsH))

	return lipgloss.JoinVertical(lipgloss.Left, detailBox, commentsBox)
}

// renderCommentsWindow returns the comments content sliced to the
// available height, with a scroll indicator on the last row when the
// content overflows.
func (m Model) renderCommentsWindow(h int) string {
	full := m.renderComments()
	if full == "" {
		return ""
	}
	lines := strings.Split(full, "\n")
	overflow := len(lines) > h
	maxRows := h
	if overflow {
		maxRows--
		if maxRows < 1 {
			maxRows = 1
		}
	}
	start := m.commentsOffset
	if start > len(lines) {
		start = len(lines)
	}
	if start < 0 {
		start = 0
	}
	end := start + maxRows
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[start:end]
	if overflow {
		up := " "
		if start > 0 {
			up = "↑"
		}
		down := " "
		if end < len(lines) {
			down = "↓"
		}
		visible = append(visible, dim.Render(fmt.Sprintf("  %s%s %d/%d  J/K scroll", up, down, end, len(lines))))
	}
	return strings.Join(visible, "\n")
}

func (m Model) renderRight() string {
	if m.viewingDiff {
		return m.diffVP.View()
	}
	if m.running {
		return m.renderProgress()
	}
	if m.lastReview != nil {
		return m.renderReviewResult()
	}
	return m.renderDetail()
}

// spinnerLine combines an active spinner frame with a message.
func spinnerLine(frame, msg string) string {
	return frame + " " + msg
}

func (m Model) renderDetail() string {
	pr, ok := m.currentPR()
	if !ok {
		return dim.Render("(no selection)")
	}
	d := m.details[pr.URL]
	if d == nil {
		var b strings.Builder
		fmt.Fprintf(&b, "#%d %s\n", pr.Number, pr.Title)
		if pr.Repo != "" {
			fmt.Fprintf(&b, "%s\n", dim.Render(pr.Repo))
		}
		if m.loadingDetail[pr.URL] {
			fmt.Fprintf(&b, "\n%s\n", spinnerLine(m.spinner.View(), "loading detail..."))
		} else {
			fmt.Fprintf(&b, "\n%s\n", hint.Render("Press Space to load detail."))
		}
		return b.String()
	}
	label := lipgloss.NewStyle().Foreground(colMauve).Bold(true).Width(8)
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\n", label.Render("title"), prTitle.Render(d.Title))
	fmt.Fprintf(&b, "%s%s %s %s\n",
		label.Render("branch"),
		lipgloss.NewStyle().Foreground(colTeal).Render(d.HeadRefName),
		dim.Render("→"),
		lipgloss.NewStyle().Foreground(colTeal).Render(d.BaseRefName),
	)
	fmt.Fprintf(&b, "%s%s\n", label.Render("author"), authorTag.Render("@"+d.Author))
	var p, f, q int
	for _, c := range d.Checks {
		switch {
		case c.Status != "COMPLETED":
			q++
		case c.Conclusion == "SUCCESS":
			p++
		default:
			f++
		}
	}
	checks := fmt.Sprintf("%s %s %s",
		pass.Render(fmt.Sprintf("✓ %d", p)),
		fail.Render(fmt.Sprintf("✗ %d", f)),
		pending.Render(fmt.Sprintf("◷ %d", q)),
	)
	fmt.Fprintf(&b, "%s%s\n", label.Render("checks"), checks)
	fmt.Fprintf(&b, "\n%s %s", keyCap.Render("⏎"), hint.Render("run review"))
	return b.String()
}

// renderComments returns reviews + discussion + inline comments. Empty
// string when nothing to show (so the comments box can collapse).
func (m Model) renderComments() string {
	pr, ok := m.currentPR()
	if !ok {
		return ""
	}
	d := m.details[pr.URL]
	if d == nil {
		return ""
	}
	if len(d.Reviews) == 0 && len(d.Comments) == 0 && len(d.Inline) == 0 {
		return dim.Render("(no comments)")
	}

	paneW, _ := paneInnerSize(m.termW, m.termH)
	bodyW := paneW - 2
	var b strings.Builder

	if len(d.Reviews) > 0 {
		fmt.Fprintf(&b, "%s\n", repoHeader.Render("◆ Reviews"))
		for _, r := range d.Reviews {
			var state string
			switch r.State {
			case "APPROVED":
				state = badgeApproved.Render("APPROVED")
			case "CHANGES_REQUESTED":
				state = badgeChanges.Render("CHANGES")
			case "COMMENTED":
				state = badgeCommented.Render("COMMENT")
			default:
				state = badgeCommented.Render(r.State)
			}
			fmt.Fprintf(&b, "%s %s %s\n", state, authorTag.Render("@"+r.Author()), dim.Render(shortTime(r.SubmittedAt)))
			if strings.TrimSpace(r.Body) != "" {
				for _, line := range wrap(r.Body, bodyW) {
					fmt.Fprintf(&b, "  %s\n", line)
				}
			}
		}
	}

	if len(d.Comments) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s\n", repoHeader.Render(fmt.Sprintf("◆ Comments (%d)", len(d.Comments))))
		for _, c := range d.Comments {
			fmt.Fprintf(&b, "%s %s\n", authorTag.Render("@"+c.Author()), dim.Render(shortTime(c.CreatedAt)))
			for _, line := range wrap(c.Body, bodyW) {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
	}

	if len(d.Inline) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s\n", repoHeader.Render(fmt.Sprintf("◆ Inline (%d)", len(d.Inline))))
		for _, ic := range d.Inline {
			fmt.Fprintf(&b, "%s %s\n", lipgloss.NewStyle().Foreground(colPink).Render(fmt.Sprintf("%s:%d", ic.Path, ic.Line)), authorTag.Render("@"+ic.User.Login))
			for _, line := range wrap(ic.Body, bodyW) {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// shortTime returns the date portion of an ISO timestamp (YYYY-MM-DD).
func shortTime(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// wrap splits s into lines at most w chars wide, respecting any
// existing newlines. Long words are hard-cut.
func wrap(s string, w int) []string {
	if w < 10 {
		w = 10
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		for len(para) > w {
			cut := w
			// try to break at last space within window
			if sp := strings.LastIndex(para[:w], " "); sp > w/2 {
				cut = sp
			}
			out = append(out, para[:cut])
			para = strings.TrimLeft(para[cut:], " ")
		}
		if para != "" {
			out = append(out, para)
		}
	}
	return out
}

func (m Model) renderProgress() string {
	var b strings.Builder
	b.WriteString("Running review pipeline " + m.spinner.View() + "\n\n")
	for _, s := range m.steps {
		b.WriteString("  ✓ " + s + "\n")
	}
	b.WriteString("\n" + hint.Render("Press q to cancel."))
	return b.String()
}

func (m Model) renderReviewResult() string {
	if m.lastReview.err != nil {
		return errStyle.Render("Pipeline failed: ") + m.lastReview.err.Error()
	}
	return fmt.Sprintf("Pending review #%d posted.\n\n%s",
		m.lastReview.reviewID,
		hint.Render("Press o to open in browser, n for next PR (j/k)."))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
