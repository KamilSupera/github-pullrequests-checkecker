package tui

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// Diff line colors adapt to the terminal background: the Dark variants
// are the original ANSI-256 tones; the Light variants are darkened so
// they stay legible on a white background.
var (
	diffAddLine  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "42", Light: "#2e7d32"})  // green
	diffDelLine  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "203", Light: "#c62828"}) // red
	diffHunkLine = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "39", Light: "#1565c0"})  // blue
	diffFileLine = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Dark: "214", Light: "#b25e00"})

	// chromaStyle defaults to the dark syntax theme; SetTheme() swaps it
	// to a light one when the terminal background is light.
	chromaStyle     = pickChromaStyle(true)
	chromaFormatter chroma.Formatter
)

func init() {
	chromaFormatter = formatters.Get("terminal256")
	if chromaFormatter == nil {
		chromaFormatter = formatters.Fallback
	}
}

// pickChromaStyle returns the syntax-highlight style for the active
// theme. For dark it prefers a warm/amber-leaning style matching the
// Blade Runner palette; for light it prefers styles designed for a white
// background. Falls through the preference list, then to chroma's default
// if none of the preferred names exist in the installed version.
func pickChromaStyle(dark bool) *chroma.Style {
	prefs := []string{"gruvbox", "tango", "solarized-dark", "monokai"}
	if !dark {
		prefs = []string{"github", "solarized-light", "friendly", "tango"}
	}
	for _, name := range prefs {
		if s := styles.Get(name); s != nil && s.Name != "" {
			return s
		}
	}
	return styles.Fallback
}

// fileBoundaries returns the 0-indexed line numbers (within the
// rendered diff) where each new file section starts, identified by
// the "diff --git" header. Used by the TUI's n/p keys to jump
// between files in the diff viewport.
func fileBoundaries(rendered string) []int {
	var out []int
	for i, line := range strings.Split(rendered, "\n") {
		// strip ANSI codes
		clean := ansiStrip(line)
		if strings.HasPrefix(clean, "diff --git") {
			out = append(out, i)
		}
	}
	return out
}

// ansiStrip removes ANSI escape sequences so we can prefix-match.
func ansiStrip(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			// skip until letter terminator
			for i < len(s) && (s[i] < 'A' || (s[i] > 'Z' && s[i] < 'a') || s[i] > 'z') {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// colorizeDiff takes a unified diff and returns the same text with ANSI
// color codes applied. File/hunk headers are colored distinctly;
// added/removed lines are tinted; code on those lines is syntax-
// highlighted via chroma based on the most recent "+++ b/<path>" entry.
func colorizeDiff(raw string) string {
	var out bytes.Buffer
	currentLang := ""

	for _, line := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git"),
			strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "--- "),
			strings.HasPrefix(line, "new file mode"),
			strings.HasPrefix(line, "deleted file mode"),
			strings.HasPrefix(line, "similarity index"),
			strings.HasPrefix(line, "rename from"),
			strings.HasPrefix(line, "rename to"):
			out.WriteString(diffFileLine.Render(line))

		case strings.HasPrefix(line, "+++ "):
			out.WriteString(diffFileLine.Render(line))
			path := strings.TrimPrefix(line, "+++ ")
			path = strings.TrimPrefix(path, "b/")
			currentLang = langFromPath(path)

		case strings.HasPrefix(line, "@@"):
			out.WriteString(diffHunkLine.Render(line))

		case strings.HasPrefix(line, "+"):
			out.WriteString(diffAddLine.Render("+"))
			out.WriteString(highlight(line[1:], currentLang))

		case strings.HasPrefix(line, "-"):
			out.WriteString(diffDelLine.Render("-"))
			out.WriteString(highlight(line[1:], currentLang))

		default:
			out.WriteString(highlight(line, currentLang))
		}
		out.WriteByte('\n')
	}
	return out.String()
}

func langFromPath(p string) string {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".rb":
		return "ruby"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".php":
		return "php"
	case ".sh", ".bash", ".zsh":
		return "bash"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".md":
		return "markdown"
	case ".sql":
		return "sql"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".xml":
		return "xml"
	case ".dockerfile":
		return "dockerfile"
	case ".tf":
		return "terraform"
	}
	switch filepath.Base(p) {
	case "Dockerfile":
		return "dockerfile"
	case "Makefile":
		return "makefile"
	}
	return ""
}

func highlight(code, lang string) string {
	if lang == "" || code == "" {
		return code
	}
	lexer := lexers.Get(lang)
	if lexer == nil {
		return code
	}
	iter, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var buf bytes.Buffer
	if err := chromaFormatter.Format(&buf, chromaStyle, iter); err != nil {
		return code
	}
	// chroma may append a trailing newline; trim because the diff parser
	// already adds its own.
	return strings.TrimRight(buf.String(), "\n")
}
