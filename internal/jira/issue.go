package jira

// Issue is the minimal Jira issue shape prcheck needs for prompt context.
type Issue struct {
	Key         string `json:"key"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}
