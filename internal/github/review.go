package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

type ReviewComment struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"` // "LEFT" or "RIGHT"
	Body     string `json:"body"`
	Severity string `json:"-"` // internal only, never sent to GitHub
}

type apiBody struct {
	Body     string          `json:"body"`
	Comments []ReviewComment `json:"comments"`
	// No `event` field → review remains PENDING.
}

func BuildReviewBody(summary string, comments []ReviewComment) ([]byte, error) {
	return json.Marshal(apiBody{Body: summary, Comments: comments})
}

// parseRepoFromURL parses "https://github.com/owner/repo/pull/123"
// and returns (owner, repo, number).
func parseRepoFromURL(prURL string) (string, string, string, error) {
	u, err := url.Parse(prURL)
	if err != nil {
		return "", "", "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return "", "", "", fmt.Errorf("not a PR URL: %s", prURL)
	}
	return parts[0], parts[1], parts[3], nil
}

func PostPendingReview(ctx context.Context, prURL, summary string, comments []ReviewComment) (int64, error) {
	owner, repo, num, err := parseRepoFromURL(prURL)
	if err != nil {
		return 0, err
	}
	body, err := BuildReviewBody(summary, comments)
	if err != nil {
		return 0, err
	}

	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%s/reviews", owner, repo, num)
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", "api", "-X", "POST", endpoint, "--input", "-")
	cmd.Stdin = bytes.NewReader(body)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("gh api: %w (stderr: %s)", err, stderr.String())
	}

	var resp struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return 0, fmt.Errorf("parse gh api response: %w", err)
	}
	return resp.ID, nil
}
