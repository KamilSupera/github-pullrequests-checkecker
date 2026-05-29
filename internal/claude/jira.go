package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/jira"
)

const jiraPromptTemplate = `Use Atlassian MCP tools to fetch Jira issue %s.

Steps:
1. Call mcp__atlassian__getAccessibleAtlassianResources to obtain the cloudId.
2. Call mcp__atlassian__getJiraIssue with that cloudId and issueIdOrKey=%q.
3. Return ONLY a single JSON object on stdout, no prose, no markdown:
{"key":"%s","summary":"","description":""}

The description may be Atlassian Document Format (ADF). If so, flatten it
to plain text. Keep it under 2000 characters.`

// JiraMCPFetcher calls the claude subprocess and asks it to fetch a Jira
// issue via the Atlassian MCP tools the user has configured.
type JiraMCPFetcher struct{}

func (JiraMCPFetcher) FetchIssue(ctx context.Context, key string) (*jira.Issue, error) {
	prompt := fmt.Sprintf(jiraPromptTemplate, key, key, key)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "claude", "-p", prompt, "--output-format", "text")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude jira fetch: %w (stderr: %s)", err, tail(stderr.String(), 500))
	}

	doc := findJSONObject(stdout.String())
	if doc == "" {
		return nil, fmt.Errorf("no JSON object in claude jira response")
	}

	var iss jira.Issue
	if err := json.Unmarshal([]byte(doc), &iss); err != nil {
		return nil, fmt.Errorf("parse jira issue JSON: %w", err)
	}
	if strings.TrimSpace(iss.Key) == "" {
		return nil, fmt.Errorf("jira issue missing key")
	}
	return &iss, nil
}
