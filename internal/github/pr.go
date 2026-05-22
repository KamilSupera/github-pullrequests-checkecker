package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type Query int

const (
	QueryAuthored Query = iota
	QueryReviewRequested
	QueryMentioned
)

// searchFlags returns the gh-search-prs flag args for this query.
// Positional query strings ("is:open is:pr author:@me") don't work
// reliably across gh versions; flag form is the supported path.
func (q Query) searchFlags() []string {
	switch q {
	case QueryAuthored:
		return []string{"--author=@me", "--state=open"}
	case QueryReviewRequested:
		return []string{"--review-requested=@me", "--state=open"}
	case QueryMentioned:
		return []string{"--mentions=@me", "--state=open"}
	}
	return nil
}

func (q Query) Label() string {
	switch q {
	case QueryAuthored:
		return "Mine"
	case QueryReviewRequested:
		return "Review"
	case QueryMentioned:
		return "Mentioned"
	}
	return "?"
}

type PR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Author      string
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	UpdatedAt   string `json:"updatedAt"`
	Repo        string // populated from RepositoryRaw.NameWithOwner

	AuthorRaw struct {
		Login string `json:"login"`
	} `json:"author"`
	RepositoryRaw struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

type Check struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type PRDetail struct {
	PR
	Body   string  `json:"body"`
	Checks []Check `json:"statusCheckRollup"`
}

func runGH(ctx context.Context, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %v: %w (stderr: %s)", args, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func SearchPRs(ctx context.Context, q Query) ([]PR, error) {
	args := []string{"search", "prs"}
	args = append(args, q.searchFlags()...)
	args = append(args,
		"--json", "number,title,url,author,repository,updatedAt",
		"--limit", "50",
	)
	out, err := runGH(ctx, args...)
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse gh search: %w", err)
	}
	for i := range prs {
		prs[i].Author = prs[i].AuthorRaw.Login
		prs[i].Repo = prs[i].RepositoryRaw.NameWithOwner
	}
	return prs, nil
}

func FetchPRDetail(ctx context.Context, url string) (*PRDetail, error) {
	out, err := runGH(ctx,
		"pr", "view", url,
		"--json", "number,title,url,body,headRefName,baseRefName,author,statusCheckRollup",
	)
	if err != nil {
		return nil, err
	}
	var d PRDetail
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, fmt.Errorf("parse gh pr view: %w", err)
	}
	d.Author = d.AuthorRaw.Login
	return &d, nil
}

func FetchDiff(ctx context.Context, url string) (string, error) {
	out, err := runGH(ctx, "pr", "diff", url)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CLIClient and Reviewer are thin adapters that satisfy the pipeline
// interfaces by delegating to the package-level functions.

type CLIClient struct{}

func (CLIClient) FetchDiff(ctx context.Context, url string) (string, error) {
	return FetchDiff(ctx, url)
}
func (CLIClient) FetchPRDetail(ctx context.Context, url string) (*PRDetail, error) {
	return FetchPRDetail(ctx, url)
}

type Reviewer struct{}

func (Reviewer) PostPendingReview(ctx context.Context, prURL, summary string, c []ReviewComment) (int64, error) {
	return PostPendingReview(ctx, prURL, summary, c)
}
