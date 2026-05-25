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
	// GitHub rejects payloads where "comments" is null. Ensure the
	// field marshals as an empty array when the caller passed nil.
	if comments == nil {
		comments = []ReviewComment{}
	}
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

	// Clear any existing PENDING review from the current user; otherwise
	// GitHub refuses to create a new one.
	_ = deletePendingReviews(ctx, owner, repo, num)

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

// deletePendingReviews removes any review on the PR that is owned by
// the current user and still in PENDING state. Best-effort: errors
// are not fatal because GitHub may legitimately have no such review.
func deletePendingReviews(ctx context.Context, owner, repo, num string) error {
	me, err := currentUser(ctx)
	if err != nil {
		return err
	}

	listOut, err := runGH(ctx, "api", fmt.Sprintf("repos/%s/%s/pulls/%s/reviews?per_page=100", owner, repo, num))
	if err != nil {
		return err
	}
	var reviews []struct {
		ID   int64 `json:"id"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(listOut, &reviews); err != nil {
		return err
	}
	for _, r := range reviews {
		if r.State == "PENDING" && strings.EqualFold(r.User.Login, me) {
			_, _ = runGH(ctx, "api", "-X", "DELETE",
				fmt.Sprintf("repos/%s/%s/pulls/%s/reviews/%d", owner, repo, num, r.ID))
		}
	}
	return nil
}

// currentUser returns the authenticated GitHub username.
func currentUser(ctx context.Context) (string, error) {
	out, err := runGH(ctx, "api", "user")
	if err != nil {
		return "", err
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(out, &u); err != nil {
		return "", err
	}
	return u.Login, nil
}
