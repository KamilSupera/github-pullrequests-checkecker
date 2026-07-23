package config

import (
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
}

// Load reads the runtime configuration from environment variables.
// All Jira access goes through the Atlassian MCP server reachable via
// `claude`, so no Jira credentials are required.
func Load() (*Config, error) {
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
	return cfg, nil
}
