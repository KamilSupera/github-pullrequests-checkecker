package config

import "os"

type Config struct {
	Debug bool
}

// Load reads the runtime configuration from environment variables.
// All Jira access goes through the Atlassian MCP server reachable via
// `claude`, so no Jira credentials are required.
func Load() (*Config, error) {
	return &Config{
		Debug: os.Getenv("PRCHECK_DEBUG") == "1",
	}, nil
}
