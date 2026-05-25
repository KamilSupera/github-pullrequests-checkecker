package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ksupera/prcheck/internal/github"
)

var (
	tabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).      // bright white
			Background(lipgloss.Color("33")).       // blue bar
			Padding(0, 1)
	tabInactive = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).      // light grey, always visible
			Padding(0, 1)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	dim         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	pass        = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	fail        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	hint        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	repoHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
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
		Width(paneW).
		Height(paneH)

	header := m.renderTabs()
	left := m.renderList()
	right := m.renderRightSplit(paneW, paneH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane.Render(left), right)
	footerText := "j/k move  Space detail  d diff  Enter review  Tab switch  g/G top/bot  o open  r refresh  q quit"
	if m.viewingDiff {
		footerText = "j/k scroll  pgup/pgdn page  d/Esc back  q quit"
	}
	footer := hint.Render(footerText)
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderTabs() string {
	var parts []string
	for i := Tab(0); i < 3; i++ {
		label := i.Label()
		if prs, loaded := m.prsByTab[i]; loaded {
			label = fmt.Sprintf("%s (%d)", label, len(prs))
		} else if err := m.loadErr[i]; err != nil {
			label = fmt.Sprintf("%s (!)", label)
		} else {
			label = fmt.Sprintf("%s (…)", label)
		}
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
		return dim.Render("loading...")
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
	// reserve chars for prefix "► " and "#NNN "
	titleW := paneW - 10
	if titleW < 10 {
		titleW = 10
	}
	prs := m.prsByTab[m.tab]
	groups := groupByRepo(prs)
	idx := 0
	for _, g := range groups {
		lines = append(lines, repoHeader.Render(truncate(g.name, paneW-2)))
		for _, pr := range g.prs {
			prefix := "  "
			if idx == m.cursor {
				prefix = "► "
				cursorLine = len(lines)
			}
			lines = append(lines, fmt.Sprintf("%s#%d %s", prefix, pr.Number, truncate(pr.Title, titleW)))
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
	// Diff / progress / review-result keep the original full-height pane.
	if m.viewingDiff || m.running || m.lastReview != nil {
		fullPane := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Width(paneW).
			Height(paneH)
		return fullPane.Render(m.renderRight())
	}

	// Allocate: detail gets 8 inner rows, comments gets the rest.
	// Each bordered box adds 2 rows; both boxes together: (d+2)+(c+2)
	// must equal paneH+2, so d+c = paneH-2.
	detailH := 8
	commentsH := paneH - 2 - detailH
	if commentsH < 3 {
		// not enough room — give comments at least 3 rows by shrinking detail
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
		Width(paneW).
		Height(detailH).
		Render(m.renderDetail())

	commentsBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(paneW).
		Height(commentsH).
		Render(m.renderComments())

	return lipgloss.JoinVertical(lipgloss.Left, detailBox, commentsBox)
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
		fmt.Fprintf(&b, "\n%s\n", hint.Render("Press Space to load detail."))
		return b.String()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Title:   %s\n", d.Title)
	fmt.Fprintf(&b, "Branch:  %s -> %s\n", d.HeadRefName, d.BaseRefName)
	fmt.Fprintf(&b, "Author:  %s\n", d.Author)
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
		pass.Render(fmt.Sprintf("%d passed", p)),
		fail.Render(fmt.Sprintf("%d failed", f)),
		dim.Render(fmt.Sprintf("%d pending", q)),
	)
	fmt.Fprintf(&b, "Checks:  %s\n", checks)
	fmt.Fprintf(&b, "\n%s", hint.Render("Press Enter to run review."))
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
		fmt.Fprintf(&b, "%s\n", repoHeader.Render("Reviews"))
		for _, r := range d.Reviews {
			state := r.State
			switch state {
			case "APPROVED":
				state = pass.Render("✓ APPROVED")
			case "CHANGES_REQUESTED":
				state = fail.Render("✗ CHANGES_REQUESTED")
			case "COMMENTED":
				state = dim.Render("• COMMENTED")
			default:
				state = dim.Render(state)
			}
			fmt.Fprintf(&b, "%s @%s %s\n", state, r.Author(), dim.Render(shortTime(r.SubmittedAt)))
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
		fmt.Fprintf(&b, "%s\n", repoHeader.Render(fmt.Sprintf("Comments (%d)", len(d.Comments))))
		for _, c := range d.Comments {
			fmt.Fprintf(&b, "@%s %s\n", c.Author(), dim.Render(shortTime(c.CreatedAt)))
			for _, line := range wrap(c.Body, bodyW) {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
	}

	if len(d.Inline) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s\n", repoHeader.Render(fmt.Sprintf("Inline (%d)", len(d.Inline))))
		for _, ic := range d.Inline {
			fmt.Fprintf(&b, "%s @%s\n", dim.Render(fmt.Sprintf("%s:%d", ic.Path, ic.Line)), ic.User.Login)
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
