package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	tabActive   = lipgloss.NewStyle().Bold(true).Underline(true).Padding(0, 1)
	tabInactive = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("241"))
	border      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	dim         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	pass        = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	fail        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	hint        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

func (m Model) View() string {
	header := m.renderTabs()
	left := m.renderList()
	right := m.renderRight()
	body := lipgloss.JoinHorizontal(lipgloss.Top, border.Render(left), border.Render(right))
	footer := hint.Render("j/k move  Tab switch  Enter review  o open  r refresh  q quit")
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
	var lines []string
	for i, pr := range prs {
		prefix := "  "
		if i == m.cursor {
			prefix = "► "
		}
		lines = append(lines, fmt.Sprintf("%s#%d %s", prefix, pr.Number, truncate(pr.Title, 50)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderRight() string {
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
