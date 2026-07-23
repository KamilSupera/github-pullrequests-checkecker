package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Debug bool

	// Focus is a comma-separated list of review aspects to emphasize in
	// the prompt. Common values: "security", "performance",
	// "requirements", "tests". Empty = balanced review.
	Focus []string

	// Agent selects the backend model CLI: "claude" (default) or "cursor".
	Agent string

	// WatchMinutes is the auto-refresh interval for Watch mode, in
	// minutes. Read from PRCHECK_WATCH; defaults to 5, clamped to >=1.
	WatchMinutes int

	// Theme selects the color scheme: "auto" (detect the terminal
	// background at startup, dark on no reply), "dark", or "light".
	// Read from the config file then PRCHECK_THEME (env overrides file);
	// unknown/empty values fall back to "auto".
	Theme string
}

// Load reads the runtime configuration from environment variables.
// All Jira access goes through the Atlassian MCP server reachable via
// `claude`, so no Jira credentials are required.
func Load() (*Config, error) {
	// Config file first; env vars below override it. A malformed file is
	// non-fatal — warn and carry on with defaults + environment.
	fc, ferr := loadFile()
	if ferr != nil {
		fmt.Fprintln(os.Stderr, "prcheck: ignoring config file:", ferr)
	}

	cfg := &Config{
		Debug: os.Getenv("PRCHECK_DEBUG") == "1",
		Agent: strings.TrimSpace(os.Getenv("PRCHECK_AGENT")),
	}
	if f := strings.TrimSpace(os.Getenv("PRCHECK_FOCUS")); f != "" {
		for _, part := range strings.Split(f, ",") {
			if p := strings.TrimSpace(part); p != "" {
				cfg.Focus = append(cfg.Focus, p)
			}
		}
	}
	cfg.WatchMinutes = 5
	if w := strings.TrimSpace(os.Getenv("PRCHECK_WATCH")); w != "" {
		if n, err := strconv.Atoi(w); err == nil && n >= 1 {
			cfg.WatchMinutes = n
		}
	}

	// Theme: config file value, then PRCHECK_THEME override, then validate.
	theme := ""
	if fc.Theme != nil {
		theme = *fc.Theme
	}
	if e := strings.TrimSpace(os.Getenv("PRCHECK_THEME")); e != "" {
		theme = e
	}
	cfg.Theme = normalizeTheme(theme)

	return cfg, nil
}

// normalizeTheme trims/lowercases the theme string and validates it.
// Unknown or empty values fall back to "auto".
func normalizeTheme(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "dark":
		return "dark"
	case "light":
		return "light"
	default:
		return "auto"
	}
}
