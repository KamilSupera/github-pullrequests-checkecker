package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ksupera/prcheck/internal/claude"
	"github.com/ksupera/prcheck/internal/github"
)

var (
	reHiddenMarker = regexp.MustCompile(`(?m)^\s*\[//\]:\s*<>\s*\(.*\)\s*$`)
	reHTMLTag      = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	reBlankLines   = regexp.MustCompile(`\n{3,}`)
)

// Theme — Blade Runner amber. Warm yellows and oranges against a near-
// black backdrop, with a single cool neon-cyan accent for contrast.
// Hex colors degrade gracefully to 256-color when truecolor is off.
var (
	colFG       = lipgloss.Color("#f5e6c8") // warm off-white
	colSubtext  = lipgloss.Color("#b89e5a") // muted amber
	colOverlay  = lipgloss.Color("#6b5a2a") // dim amber
	colMauve    = lipgloss.Color("#ffb000") // PRIMARY: amber yellow
	colBlue     = lipgloss.Color("#00bfff") // neon cyan accent
	colTeal     = lipgloss.Color("#ff8800") // deep orange (branch refs)
	colGreen    = lipgloss.Color("#a0d060") // muted neon green
	colYellow   = lipgloss.Color("#ffc107") // brighter gold (counts)
	colPeach    = lipgloss.Color("#ff6f00") // burnt orange (author)
	colRed      = lipgloss.Color("#ff4444") // alarm red
	colPink     = lipgloss.Color("#e5a100") // dark amber (inline path)
	colSurface0 = lipgloss.Color("#1a1305") // near-black warm brown

	tabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#1a1305")). // dark text on amber bar
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

	// Status badges (filled pills) — dark text on neon.
	badgeApproved = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#1a1305")).
			Background(colGreen).
			Padding(0, 1)
	badgeChanges = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#1a1305")).
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
		Height(paneH).
		MaxHeight(paneH + 2)

	header := m.renderTabs()
	left := m.renderList()
	right := m.renderRightSplit(paneW, paneH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane.Render(left), right)

	footer := m.renderFooter()
	if m.filtering || m.filter != "" {
		footer = m.renderFilterLine() + "\n" + footer
	}
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderFilterLine() string {
	prompt := lipgloss.NewStyle().Foreground(colMauve).Bold(true).Render("/")
	q := lipgloss.NewStyle().Foreground(colFG).Render(m.filter)
	if m.filtering {
		q += lipgloss.NewStyle().Foreground(colMauve).Render("█")
	}
	tail := ""
	if m.filtering {
		tail = "  " + dim.Render("Enter accept · Esc clear · Ctrl+U wipe")
	} else if m.filter != "" {
		tail = "  " + dim.Render(fmt.Sprintf("(%d matches — / to edit, Esc to clear)", len(m.filteredPRs())))
	}
	return prompt + q + tail
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
			{"/", "filter"},
			{"Space", "load"},
			{"d", "diff"},
			{"c", "checks"},
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
	// Inactive tabs: bright gold count on the default background.
	inactiveCount := lipgloss.NewStyle().Foreground(colYellow).Bold(true)
	// Active tab: count rendered as a small dark-on-light pill that
	// pops against the amber bar.
	activeCount := lipgloss.NewStyle().
		Foreground(colMauve).
		Background(lipgloss.Color("#1a1305")).
		Bold(true).
		Padding(0, 1)
	for i := Tab(0); i < 3; i++ {
		name := i.Label()
		active := i == m.tab
		var count string
		if prs, loaded := m.prsByTab[i]; loaded {
			n := fmt.Sprintf("%d", len(prs))
			if active {
				count = activeCount.Render(n)
			} else {
				count = inactiveCount.Render(n)
			}
		} else if err := m.loadErr[i]; err != nil {
			_ = err
			count = errStyle.Render("!")
		} else {
			count = dim.Render("…")
		}
		label := fmt.Sprintf("%s %s", name, count)
		if active {
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
	prs := m.filteredPRs()
	groups := groupByRepo(prs)
	idx := 0
	cursorStyle := lipgloss.NewStyle().Foreground(colMauve).Bold(true)
	newDot := lipgloss.NewStyle().Foreground(colPeach).Bold(true).Render("•")
	for _, g := range groups {
		lines = append(lines, repoHeader.Render("● "+truncate(g.name, paneW-4)))
		for _, pr := range g.prs {
			active := idx == m.cursor
			isNew := m.isPRNew(pr)
			marker := " "
			if isNew {
				marker = newDot
			}
			var prefix, num, title string
			if active {
				prefix = cursorStyle.Render("❯ ")
				num = prNumStyle.Render(fmt.Sprintf("#%d", pr.Number))
				title = prTitle.Bold(true).Render(truncate(pr.Title, titleW-2))
				cursorLine = len(lines)
			} else if isNew {
				prefix = "  "
				num = lipgloss.NewStyle().Foreground(colFG).Render(fmt.Sprintf("#%d", pr.Number))
				title = lipgloss.NewStyle().Foreground(colFG).Render(truncate(pr.Title, titleW-2))
			} else {
				prefix = "  "
				num = dim.Render(fmt.Sprintf("#%d", pr.Number))
				title = dim.Render(truncate(pr.Title, titleW-2))
			}
			lines = append(lines, fmt.Sprintf("%s%s%s %s", prefix, marker, num, title))
			idx++
		}
	}
	return
}

// isPRNew reports whether the PR has updates since the user last
// loaded its detail. Unseen PRs are also reported as new.
func (m Model) isPRNew(pr github.PR) bool {
	if m.seen == nil {
		return false
	}
	t, _ := time.Parse(time.RFC3339, pr.UpdatedAt)
	return m.seen.IsNew(pr.URL, t)
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
	useFull := m.viewingDiff || m.viewingChecks || m.running
	if !useFull && m.lastReview != nil {
		if pr, ok := m.currentPR(); ok && pr.URL == m.lastReview.url {
			useFull = true
		}
	}
	if useFull {
		fullPane := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderActiveColor).
			Width(paneW).
			Height(paneH).
			MaxHeight(paneH + 2)
		return fullPane.Render(m.renderRight())
	}

	// Allocate ~60% of the right column to detail (it now carries
	// status, labels, assignees, reviewers, description preview), and
	// the rest to comments. Clamp so the smaller box still has room.
	detailH := paneH * 6 / 10
	if detailH < 10 {
		detailH = 10
	}
	commentsH := paneH - 2 - detailH
	if commentsH < 4 {
		detailH = paneH - 2 - 4
		if detailH < 4 {
			detailH = 4
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
		MaxHeight(detailH + 2).
		Render(m.renderDetailWindow(detailH, paneW))

	commentsBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(paneW).
		Height(commentsH).
		MaxHeight(commentsH + 2).
		Render(m.renderCommentsWindow(commentsH, paneW))

	return lipgloss.JoinVertical(lipgloss.Left, detailBox, commentsBox)
}

// renderCommentsWindow returns the comments content sliced to the
// available height, with a scroll indicator on the last row when the
// content overflows. Each line is clipped to the pane width so the
// bordered box never expands vertically due to terminal wrapping.
func (m Model) renderCommentsWindow(h, paneW int) string {
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
	for i, ln := range visible {
		if lipgloss.Width(ln) > paneW {
			visible[i] = clipToWidth(ln, paneW)
		}
	}
	return strings.Join(visible, "\n")
}

// clipToWidth truncates s so its visible width does not exceed w. It
// preserves ANSI escape sequences; only printable runes are counted.
func clipToWidth(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	// Re-render rune by rune until width exceeds w. We approximate by
	// trimming runes off the end and re-measuring; cheap for the rare
	// over-wide case.
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > w {
		runes = runes[:len(runes)-1]
	}
	return string(runes)
}

func (m Model) renderRight() string {
	if m.viewingDiff {
		return m.diffVP.View()
	}
	if m.viewingChecks {
		return m.checksVP.View()
	}
	if m.running {
		return m.renderProgress()
	}
	if m.lastReview != nil {
		// Show the result only while the cursor is still on the PR
		// it belongs to; moving the cursor reverts to detail view.
		if pr, ok := m.currentPR(); ok && pr.URL == m.lastReview.url {
			return m.renderResultWindow(m.resultH)
		}
	}
	return m.renderDetail()
}

// renderResultWindow slices the review-result content to fit resultH
// rows. Last row shows a scroll indicator when content overflows.
func (m Model) renderResultWindow(h int) string {
	full := m.renderReviewResult()
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
	start := m.resultOffset
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
		visible = append(visible, dim.Render(fmt.Sprintf("  %s%s %d/%d  j/k scroll", up, down, end, len(lines))))
	}
	return strings.Join(visible, "\n")
}

// spinnerLine combines an active spinner frame with a message.
func spinnerLine(frame, msg string) string {
	return frame + " " + msg
}

// renderDetailWindow slices the detail content to detailH rows.
// When content overflows, the last row holds a scroll indicator.
func (m Model) renderDetailWindow(h, paneW int) string {
	full := m.renderDetailFull()
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
	start := m.detailOffset
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
		visible = append(visible, dim.Render(fmt.Sprintf("  %s%s %d/%d  [/] scroll", up, down, end, len(lines))))
	}
	for i, ln := range visible {
		if lipgloss.Width(ln) > paneW {
			visible[i] = clipToWidth(ln, paneW)
		}
	}
	return strings.Join(visible, "\n")
}

// renderDetail is the public name kept for renderRight's fall-through;
// it returns a single-screen view (no scroll). Used when the right
// pane is showing the result of a finished pipeline.
func (m Model) renderDetail() string { return m.renderDetailFull() }

func (m Model) renderDetailFull() string {
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
	paneW, _ := paneInnerSize(m.termW, m.termH)
	bodyW := paneW - 2

	fmt.Fprintf(&b, "%s%s\n", label.Render("title"), prTitle.Render(d.Title))
	fmt.Fprintf(&b, "%s%s\n", label.Render("status"), renderStatusLine(d))
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

	if d.ChangedFiles > 0 || d.Additions > 0 || d.Deletions > 0 {
		fmt.Fprintf(&b, "%s%s %s %s\n", label.Render("diff"),
			dim.Render(fmt.Sprintf("%d files", d.ChangedFiles)),
			pass.Render(fmt.Sprintf("+%d", d.Additions)),
			fail.Render(fmt.Sprintf("-%d", d.Deletions)),
		)
	}

	if len(d.Labels) > 0 {
		fmt.Fprintf(&b, "%s%s\n", label.Render("labels"), renderLabels(d.Labels))
	}
	if len(d.Assignees) > 0 {
		fmt.Fprintf(&b, "%s%s\n", label.Render("assigned"), renderUserList(d.Assignees))
	}
	if len(d.ReviewRequests) > 0 {
		fmt.Fprintf(&b, "%s%s\n", label.Render("review"), renderUserList(d.ReviewRequests))
	}
	if d.UpdatedAt != "" {
		fmt.Fprintf(&b, "%s%s\n", label.Render("updated"), dim.Render(shortTime(d.UpdatedAt)))
	}

	if body := strings.TrimSpace(stripCommentMarkdown(d.Body)); body != "" {
		fmt.Fprintf(&b, "\n%s\n", repoHeader.Render("◆ Description"))
		for _, line := range wrap(body, bodyW) {
			fmt.Fprintf(&b, "%s\n", line)
		}
	}

	fmt.Fprintf(&b, "\n%s %s", keyCap.Render("⏎"), hint.Render("run review"))
	return b.String()
}

// renderStatusLine renders a row of compact pills: state, draft flag,
// review decision, and merge state. Only present ones render.
func renderStatusLine(d *github.PRDetail) string {
	var parts []string
	switch d.State {
	case "OPEN":
		parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1a1305")).Background(colGreen).Padding(0, 1).Render("OPEN"))
	case "CLOSED":
		parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1a1305")).Background(colRed).Padding(0, 1).Render("CLOSED"))
	case "MERGED":
		parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1a1305")).Background(colMauve).Padding(0, 1).Render("MERGED"))
	}
	if d.IsDraft {
		parts = append(parts, badgeCommented.Render("DRAFT"))
	}
	switch d.ReviewDecision {
	case "APPROVED":
		parts = append(parts, badgeApproved.Render("APPROVED"))
	case "CHANGES_REQUESTED":
		parts = append(parts, badgeChanges.Render("CHANGES"))
	case "REVIEW_REQUIRED":
		parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1a1305")).Background(colYellow).Padding(0, 1).Render("REVIEW REQ"))
	}
	switch d.Mergeable {
	case "CONFLICTING":
		parts = append(parts, badgeChanges.Render("CONFLICT"))
	case "MERGEABLE":
		// quiet — most of the time this is true
	}
	if len(parts) == 0 {
		return dim.Render("—")
	}
	return strings.Join(parts, " ")
}

func renderLabels(labels []github.Label) string {
	var parts []string
	for _, l := range labels {
		parts = append(parts, lipgloss.NewStyle().Foreground(colYellow).Render("#"+l.Name))
	}
	return strings.Join(parts, " ")
}

func renderUserList(us []github.User) string {
	var parts []string
	for _, u := range us {
		parts = append(parts, authorTag.Render("@"+u.Login))
	}
	return strings.Join(parts, " ")
}

// stripCommentMarkdown removes hidden markdown markers + HTML tags
// from a PR body so it reads cleanly in the detail pane.
func stripCommentMarkdown(s string) string {
	s = reHiddenMarker.ReplaceAllString(s, "")
	s = reHTMLTag.ReplaceAllString(s, "")
	s = reBlankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
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
			body := cleanCommentBody(r.Body)
			if body != "" {
				for _, line := range wrap(body, bodyW) {
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
			body := cleanCommentBody(c.Body)
			if body == "" {
				continue
			}
			fmt.Fprintf(&b, "%s %s\n", authorTag.Render("@"+c.Author()), dim.Render(shortTime(c.CreatedAt)))
			for _, line := range wrap(body, bodyW) {
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
			body := cleanCommentBody(ic.Body)
			fmt.Fprintf(&b, "%s %s\n", lipgloss.NewStyle().Foreground(colPink).Render(fmt.Sprintf("%s:%d", ic.Path, ic.Line)), authorTag.Render("@"+ic.User.Login))
			for _, line := range wrap(body, bodyW) {
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

// wrap splits s into lines at most w columns wide, respecting any
// existing newlines. Operates on runes (not bytes) so multibyte
// characters never get sliced mid-codepoint.
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
		runes := []rune(para)
		for len(runes) > w {
			cut := w
			// prefer breaking at the last space inside the window
			for i := w - 1; i > w/2; i-- {
				if runes[i] == ' ' {
					cut = i
					break
				}
			}
			out = append(out, string(runes[:cut]))
			// skip any leading spaces on the next slice
			j := cut
			for j < len(runes) && runes[j] == ' ' {
				j++
			}
			runes = runes[j:]
		}
		if len(runes) > 0 {
			out = append(out, string(runes))
		}
	}
	return out
}

// cleanCommentBody strips bot-noise so the comments box stays readable.
// Removes hidden markdown markers, HTML tags, and collapses runs of
// blank lines.
func cleanCommentBody(s string) string {
	s = reHiddenMarker.ReplaceAllString(s, "")
	s = reHTMLTag.ReplaceAllString(s, "")
	s = reBlankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func (m Model) renderProgress() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Foreground(colMauve).Bold(true).Render("Running review pipeline")
	b.WriteString(title + " " + m.spinner.View() + "\n\n")
	for _, s := range m.steps {
		b.WriteString(renderStepRow(m.spinner.View(), s) + "\n")
	}
	b.WriteString("\n" + keyCap.Render("q") + " " + hint.Render("cancel pipeline"))
	return b.String()
}

func renderStepRow(spin string, s stepRec) string {
	var icon string
	switch s.status {
	case "start":
		icon = lipgloss.NewStyle().Foreground(colMauve).Render(spin)
	case "done":
		icon = pass.Render("✓")
	case "skip":
		icon = dim.Render("∅")
	case "warn":
		icon = pending.Render("⚠")
	case "error":
		icon = fail.Render("✗")
	default:
		icon = dim.Render("·")
	}
	stepName := lipgloss.NewStyle().Foreground(colMauve).Bold(true).Width(8).Render(s.step)
	note := s.note
	if s.err != nil {
		note = fmt.Sprintf("%s (%v)", note, s.err)
	}
	if note == "" {
		note = dim.Render("…")
	} else {
		note = lipgloss.NewStyle().Foreground(colFG).Render(note)
	}
	return fmt.Sprintf("%s %s %s", icon, stepName, note)
}

func (m Model) renderReviewResult() string {
	if m.lastReview.err != nil {
		return errStyle.Render("Pipeline failed: ") + m.lastReview.err.Error()
	}
	paneW, _ := paneInnerSize(m.termW, m.termH)
	bodyW := paneW - 2

	var b strings.Builder
	header := lipgloss.NewStyle().Foreground(colMauve).Bold(true).Render(
		fmt.Sprintf("✓ Pending review #%d posted", m.lastReview.reviewID),
	)
	b.WriteString(header + "\n")
	b.WriteString(hint.Render("Open in GitHub to submit/edit.") + "\n\n")

	if s := strings.TrimSpace(m.lastReview.summary); s != "" {
		b.WriteString(repoHeader.Render("◆ Summary") + "\n")
		for _, line := range wrap(s, bodyW) {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	if len(m.lastReview.aspects) > 0 {
		b.WriteString(repoHeader.Render("◆ Aspects checked") + "\n")
		for _, a := range m.lastReview.aspects {
			b.WriteString(aspectLine(a, bodyW) + "\n")
		}
		b.WriteString("\n")
	}

	if n := len(m.lastReview.comments); n > 0 {
		b.WriteString(repoHeader.Render(fmt.Sprintf("◆ Inline comments (%d)", n)) + "\n")
		for _, c := range m.lastReview.comments {
			loc := lipgloss.NewStyle().Foreground(colPink).Render(fmt.Sprintf("%s:%d", c.Path, c.Line))
			sev := severityBadge(c.Severity)
			b.WriteString(sev + " " + loc + "\n")
			// Body may already include the "[severity] " prefix added in
			// the pipeline; strip it for the display since we render the
			// severity badge separately.
			body := strings.TrimPrefix(c.Body, "["+c.Severity+"] ")
			for _, line := range wrap(body, bodyW) {
				b.WriteString("  " + line + "\n")
			}
		}
	}

	b.WriteString("\n" + keyCap.Render("o") + " " + hint.Render("open in browser") + "  " +
		keyCap.Render("⏎") + " " + hint.Render("re-review") + "  " +
		keyCap.Render("Esc") + " " + hint.Render("close") + "  " +
		keyCap.Render("j/k") + " " + hint.Render("next PR"))
	return b.String()
}

// aspectLine renders one row of the aspects-checked checklist:
// icon + name + optional note. Note is wrapped underneath.
func aspectLine(a claude.Aspect, w int) string {
	var icon, name string
	switch a.Status {
	case "ok":
		icon = pass.Render("✓")
	case "issue":
		icon = fail.Render("✗")
	case "missing":
		icon = pending.Render("⚠")
	case "n/a":
		icon = dim.Render("·")
	default:
		icon = dim.Render("·")
	}
	name = lipgloss.NewStyle().Foreground(colFG).Render(strings.ReplaceAll(a.Name, "_", " "))
	row := fmt.Sprintf("  %s %s", icon, name)
	if a.Note != "" && a.Status != "ok" {
		row += "\n"
		for _, line := range wrap(a.Note, w-4) {
			row += "    " + dim.Render(line) + "\n"
		}
		row = strings.TrimRight(row, "\n")
	}
	return row
}

func severityBadge(sev string) string {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(lipgloss.Color("#1a1305"))
	switch sev {
	case "blocker":
		return style.Background(colRed).Render("BLOCKER")
	case "major":
		return style.Background(colPeach).Render("MAJOR")
	case "minor":
		return style.Background(colYellow).Render("MINOR")
	case "nit":
		return style.Background(colOverlay).Foreground(colFG).Render("nit")
	}
	return dim.Render(sev)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
