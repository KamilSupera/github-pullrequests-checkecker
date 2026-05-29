package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// renderMarkdown converts a markdown body to colored terminal text
// using a "dark" style. Width sets the wrap column; lower bound 40.
// Falls back to the plain stripped body when glamour fails.
//
// We use WithStandardStyle("dark") rather than WithAutoStyle(). AutoStyle
// queries the terminal for its background color via an OSC escape and
// reads the response from the TTY — but bubbletea has already taken over
// stdin in AltScreen mode, so the response never reaches termenv and the
// call blocks forever, freezing the entire UI on the first markdown
// render (e.g. when Space loads PR detail).
func renderMarkdown(s string, width int) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	if width < 40 {
		width = 40
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return stripCommentMarkdown(s)
	}
	out, err := r.Render(stripCommentMarkdown(s))
	if err != nil {
		return stripCommentMarkdown(s)
	}
	return strings.TrimSpace(out)
}
