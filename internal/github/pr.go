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
	Body            string          `json:"body"`
	Checks          []Check         `json:"statusCheckRollup"`
	Comments        []Comment       `json:"comments"`
	Reviews         []Review        `json:"reviews"`
	State           string          `json:"state"`     // OPEN, CLOSED, MERGED
	IsDraft         bool            `json:"isDraft"`
	ReviewDecision  string          `json:"reviewDecision"` // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	MergeStateStatus string         `json:"mergeStateStatus"`
	Mergeable       string          `json:"mergeable"` // MERGEABLE, CONFLICTING, UNKNOWN
	Labels          []Label         `json:"labels"`
	Assignees       []User          `json:"assignees"`
	ReviewRequests  []User          `json:"reviewRequests"`
	UpdatedAt       string          `json:"updatedAt"`
	ChangedFiles    int             `json:"changedFiles"`
	Additions       int             `json:"additions"`
	Deletions       int             `json:"deletions"`
	Inline          []InlineComment `json:"-"`
}

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type User struct {
	Login string `json:"login"`
}

type Comment struct {
	AuthorRaw struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

func (c Comment) Author() string { return c.AuthorRaw.Login }

type Review struct {
	AuthorRaw struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string `json:"body"`
	State     string `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED, PENDING
	SubmittedAt string `json:"submittedAt"`
}

func (r Review) Author() string { return r.AuthorRaw.Login }

// InlineComment is a per-line review comment fetched via `gh api`.
type InlineComment struct {
	ID        int64  `json:"id"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
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
		"--json", "number,title,url,body,headRefName,baseRefName,author,statusCheckRollup,comments,reviews,state,isDraft,reviewDecision,mergeStateStatus,mergeable,labels,assignees,reviewRequests,updatedAt,changedFiles,additions,deletions",
	)
	if err != nil {
		return nil, err
	}
	var d PRDetail
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, fmt.Errorf("parse gh pr view: %w", err)
	}
	d.Author = d.AuthorRaw.Login

	// Best-effort inline review comments via gh api. If it fails we
	// still return the detail; inline comments aren't critical.
	if inline, err := fetchInlineComments(ctx, url); err == nil {
		// Inline comments are attached as synthetic top-level "comments"
		// in the detail. They render below the discussion section.
		d.Inline = inline
	}
	return &d, nil
}

func fetchInlineComments(ctx context.Context, prURL string) ([]InlineComment, error) {
	owner, repo, num, err := parseRepoFromURL(prURL)
	if err != nil {
		return nil, err
	}
	out, err := runGH(ctx, "api", fmt.Sprintf("repos/%s/%s/pulls/%s/comments?per_page=100", owner, repo, num))
	if err != nil {
		return nil, err
	}
	var c []InlineComment
	if err := json.Unmarshal(out, &c); err != nil {
		return nil, fmt.Errorf("parse inline comments: %w", err)
	}
	return c, nil
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
