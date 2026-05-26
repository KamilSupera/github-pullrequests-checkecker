package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ksupera/prcheck/internal/cache"
	"github.com/ksupera/prcheck/internal/github"
)

// renderStats computes a textual stats dashboard from cached state.
// Width controls the title/sep length only — no per-line wrapping.
func renderStats(m Model, w int) string {
	hl := lipgloss.NewStyle().Foreground(colMauve).Bold(true)
	num := lipgloss.NewStyle().Foreground(colYellow).Bold(true)

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", hl.Render("prcheck stats"))

	// PRs per tab.
	fmt.Fprintf(&b, "%s\n", hl.Render("Open PRs"))
	for _, t := range []Tab{TabMine, TabReview, TabMentioned} {
		fmt.Fprintf(&b, "  %-10s %s\n", t.Label(), num.Render(fmt.Sprintf("%d", len(m.prsByTab[t]))))
	}
	b.WriteString("\n")

	// Per-repo across all tabs (counted on TabMine only to avoid double-counting authored PRs).
	repoCounts := map[string]int{}
	staleCutoff := time.Now().Add(-7 * 24 * time.Hour)
	stale := 0
	for _, pr := range m.prsByTab[TabMine] {
		repo := pr.Repo
		if repo == "" {
			repo = "(unknown)"
		}
		repoCounts[repo]++
		if t, err := time.Parse(time.RFC3339, pr.UpdatedAt); err == nil && t.Before(staleCutoff) {
			stale++
		}
	}
	type kv struct {
		k string
		v int
	}
	var repos []kv
	for r, c := range repoCounts {
		repos = append(repos, kv{r, c})
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].v > repos[j].v })
	if len(repos) > 0 {
		fmt.Fprintf(&b, "%s\n", hl.Render("Mine — by repo"))
		for _, r := range repos {
			short := r.k
			if i := strings.LastIndex(short, "/"); i >= 0 {
				short = short[i+1:]
			}
			fmt.Fprintf(&b, "  %-30s %s\n", short, num.Render(fmt.Sprintf("%d", r.v)))
		}
		b.WriteString("\n")
	}

	// Stale.
	fmt.Fprintf(&b, "%s\n", hl.Render("Activity"))
	fmt.Fprintf(&b, "  %-22s %s\n", "stale (Mine, >7d)", num.Render(fmt.Sprintf("%d", stale)))

	// Bookmarks.
	if m.bookmarks != nil {
		fmt.Fprintf(&b, "  %-22s %s\n", "bookmarked", num.Render(fmt.Sprintf("%d", len(m.bookmarks.URLs))))
	}

	// Review history.
	hist := cache.LoadHistory(0)
	weekAgo := time.Now().Add(-7 * 24 * time.Hour)
	thisWeek := 0
	for _, h := range hist {
		if h.When.After(weekAgo) {
			thisWeek++
		}
	}
	fmt.Fprintf(&b, "  %-22s %s\n", "reviews total", num.Render(fmt.Sprintf("%d", len(hist))))
	fmt.Fprintf(&b, "  %-22s %s\n", "reviews this week", num.Render(fmt.Sprintf("%d", thisWeek)))

	// Recent history (last 5).
	if len(hist) > 0 {
		fmt.Fprintf(&b, "\n%s\n", hl.Render("Recent reviews"))
		start := 0
		if len(hist) > 5 {
			start = len(hist) - 5
		}
		for _, h := range hist[start:] {
			short := shortURL(h.PRURL)
			fmt.Fprintf(&b, "  %s  %s  %s\n",
				dim.Render(h.When.Format("2006-01-02 15:04")),
				lipgloss.NewStyle().Foreground(colFG).Render(short),
				dim.Render(fmt.Sprintf("(%d comments)", h.Comments)),
			)
		}
	}

	return b.String()
}

func shortURL(s string) string {
	// keep ".../repo#NNN" form
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' && i+1 < len(s) {
			// find /<owner>/<repo>/pull/<num>
			parts := strings.Split(s, "/")
			if len(parts) >= 4 && parts[len(parts)-2] == "pull" {
				return parts[len(parts)-3] + "#" + parts[len(parts)-1]
			}
			return s[i+1:]
		}
	}
	return s
}

// unused but keeps github import live for future per-repo refinements
var _ = github.PR{}
