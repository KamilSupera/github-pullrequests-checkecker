package claude

import (
	"context"
	"encoding/json"
	"fmt"
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

	out, err := runAgent(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("claude jira fetch: %w", err)
	}

	var iss jira.Issue
	found := decodeFirstObject(out, func(doc string) bool {
		var cand jira.Issue
		if err := json.Unmarshal([]byte(doc), &cand); err != nil {
			return false
		}
		if strings.TrimSpace(cand.Key) == "" {
			return false
		}
		iss = cand
		return true
	})
	if !found {
		return nil, fmt.Errorf("no parseable jira issue JSON in %s output (tail: %q)", AgentName(), tail(out, 300))
	}
	return &iss, nil
}
