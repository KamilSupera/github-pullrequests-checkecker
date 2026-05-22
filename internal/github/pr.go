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

func (q Query) gqString() string {
	switch q {
	case QueryAuthored:
		return "is:open is:pr author:@me"
	case QueryReviewRequested:
		return "is:open is:pr review-requested:@me"
	case QueryMentioned:
		return "is:open is:pr mentions:@me"
	}
	return ""
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

	AuthorRaw struct {
		Login string `json:"login"`
	} `json:"author"`
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
	out, err := runGH(ctx,
		"search", "prs",
		q.gqString(),
		"--json", "number,title,url,author,headRefName,baseRefName,updatedAt",
		"--limit", "50",
	)
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse gh search: %w", err)
	}
	for i := range prs {
		prs[i].Author = prs[i].AuthorRaw.Login
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
