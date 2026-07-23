package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// renderMarkdown converts a markdown body to colored terminal text using
// glamour's built-in "dark" or "light" standard style, chosen from the
// theme resolved at startup (see internal/tui/theme.go). Width sets the
// wrap column; lower bound 40. Falls back to the plain stripped body when
// glamour fails.
//
// We pass the style explicitly rather than using WithAutoStyle().
// AutoStyle queries the terminal for its background color via an OSC
// escape and reads the response from the TTY — but bubbletea has already
// taken over stdin in AltScreen mode, so the response never reaches
// termenv and the call blocks forever, freezing the entire UI on the
// first markdown render (e.g. when Space loads PR detail). SetTheme is
// therefore called once before the program starts, never mid-run.
func renderMarkdown(s string, width int) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	if width < 40 {
		width = 40
	}
	style := "dark"
	if !useDarkTheme {
		style = "light"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
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
