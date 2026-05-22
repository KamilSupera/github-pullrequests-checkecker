package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	JiraBaseURL string
	JiraEmail   string
	JiraToken   string
	Debug       bool
}

func Load() (*Config, error) {
	cfg := &Config{
		JiraBaseURL: os.Getenv("JIRA_BASE_URL"),
		JiraEmail:   os.Getenv("JIRA_EMAIL"),
		JiraToken:   os.Getenv("JIRA_TOKEN"),
		Debug:       os.Getenv("PRCHECK_DEBUG") == "1",
	}

	var missing []string
	if cfg.JiraBaseURL == "" {
		missing = append(missing, "JIRA_BASE_URL")
	}
	if cfg.JiraEmail == "" {
		missing = append(missing, "JIRA_EMAIL")
	}
	if cfg.JiraToken == "" {
		missing = append(missing, "JIRA_TOKEN")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing env vars: %s", strings.Join(missing, ", "))
	}

	// sanity: base URL must look like a URL
	if !strings.HasPrefix(cfg.JiraBaseURL, "https://") && !strings.HasPrefix(cfg.JiraBaseURL, "http://") {
		return nil, errors.New("JIRA_BASE_URL must start with http(s)://")
	}

	return cfg, nil
}
