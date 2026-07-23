package tui

// useDarkTheme mirrors the resolved dark/light choice for the two
// subsystems that do NOT read the lipgloss col* palette: glamour markdown
// (markdown.go) and chroma syntax highlighting (diffcolor.go). The
// palette itself adapts automatically via lipgloss.AdaptiveColor and the
// renderer's HasDarkBackground flag, which main() sets alongside SetTheme.
//
// It defaults to true (dark) so any code path or test that never calls
// SetTheme keeps the original dark behavior.
var useDarkTheme = true

// SetTheme records the resolved dark/light choice and refreshes the
// chroma syntax-highlight style to match. Call it once at startup, before
// the Bubble Tea program runs and before anything renders — never mid-run
// (see the AltScreen note in markdown.go).
func SetTheme(dark bool) {
	useDarkTheme = dark
	chromaStyle = pickChromaStyle(dark)
}
