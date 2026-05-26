package config

import (
	"os"
	"strings"
)

type Config struct {
	Debug bool

	// Focus is a comma-separated list of review aspects to emphasize in
	// the prompt. Common values: "security", "performance",
	// "requirements", "tests". Empty = balanced review.
	Focus []string
}

// Load reads the runtime configuration from environment variables.
// All Jira access goes through the Atlassian MCP server reachable via
// `claude`, so no Jira credentials are required.
func Load() (*Config, error) {
	cfg := &Config{
		Debug: os.Getenv("PRCHECK_DEBUG") == "1",
	}
	if f := strings.TrimSpace(os.Getenv("PRCHECK_FOCUS")); f != "" {
		for _, part := range strings.Split(f, ",") {
			if p := strings.TrimSpace(part); p != "" {
				cfg.Focus = append(cfg.Focus, p)
			}
		}
	}
	return cfg, nil
}
