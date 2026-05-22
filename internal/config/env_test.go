package config

import (
	"testing"
)

func TestLoad_AllSet(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://example.atlassian.net")
	t.Setenv("JIRA_EMAIL", "me@example.com")
	t.Setenv("JIRA_TOKEN", "tok")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned err: %v", err)
	}
	if cfg.JiraBaseURL != "https://example.atlassian.net" {
		t.Errorf("JiraBaseURL = %q", cfg.JiraBaseURL)
	}
	if cfg.JiraEmail != "me@example.com" {
		t.Errorf("JiraEmail = %q", cfg.JiraEmail)
	}
	if cfg.JiraToken != "tok" {
		t.Errorf("JiraToken = %q", cfg.JiraToken)
	}
}

func TestLoad_MissingVars(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "")
	t.Setenv("JIRA_EMAIL", "")
	t.Setenv("JIRA_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing env vars")
	}
}

func TestLoad_DebugFlag(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://x")
	t.Setenv("JIRA_EMAIL", "a")
	t.Setenv("JIRA_TOKEN", "b")
	t.Setenv("PRCHECK_DEBUG", "1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if !cfg.Debug {
		t.Error("Debug should be true when PRCHECK_DEBUG=1")
	}
}
