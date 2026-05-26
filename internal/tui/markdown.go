package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// renderMarkdown converts a markdown body to colored terminal text
// using a "dark" style. Width sets the wrap column; lower bound 40.
// Falls back to the plain stripped body when glamour fails.
func renderMarkdown(s string, width int) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	if width < 40 {
		width = 40
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return stripCommentMarkdown(s)
	}
	out, err := r.Render(stripCommentMarkdown(s))
	if err != nil {
		return stripCommentMarkdown(s)
	}
	// glamour wraps output in blank lines — trim outer whitespace.
	return strings.TrimSpace(out)
}
