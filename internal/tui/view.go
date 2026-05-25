package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ksupera/prcheck/internal/github"
)

var (
	tabActive   = lipgloss.NewStyle().Bold(true).Underline(true).Padding(0, 1)
	tabInactive = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("241"))
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
	pane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(paneW).
		Height(paneH)

	header := m.renderTabs()
	left := m.renderList()
	right := m.renderRight()
	body := lipgloss.JoinHorizontal(lipgloss.Top, pane.Render(left), pane.Render(right))
	footerText := "j/k move  g/G top/bot  Tab switch  Enter review  d diff  o open  r refresh  q quit"
	if m.viewingDiff {
		footerText = "j/k scroll  pgup/pgdn page  d/Esc back  q quit"
	}
	footer := hint.Render(footerText)
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderTabs() string {
	var parts []string
	for i := Tab(0); i < 3; i++ {
		s := i.Label()
		if i == m.tab {
			parts = append(parts, tabActive.Render(s))
		} else {
			parts = append(parts, tabInactive.Render(s))
		}
	}
	return strings.Join(parts, "  ")
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
	start := m.listOffset
	if start < 0 {
		start = 0
	}
	if start > len(lines) {
		start = len(lines)
	}
	end := start + m.listH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[start:end]

	// Show simple scrollbar hint when content overflows.
	if len(lines) > m.listH {
		extra := ""
		if start > 0 {
			extra += "↑"
		} else {
			extra += " "
		}
		if end < len(lines) {
			extra += "↓"
		} else {
			extra += " "
		}
		visible = append(visible, dim.Render(fmt.Sprintf("  %s %d/%d", extra, end, len(lines))))
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
		return fmt.Sprintf("%s\n%s", pr.Title, dim.Render("loading detail..."))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Title:   %s\n", d.Title)
	fmt.Fprintf(&b, "Branch:  %s -> %s\n", d.HeadRefName, d.BaseRefName)
	fmt.Fprintf(&b, "Author:  %s\n", d.Author)
	// checks
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
	fmt.Fprintf(&b, "\n%s\n", hint.Render("Press Enter to run review."))
	return b.String()
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
