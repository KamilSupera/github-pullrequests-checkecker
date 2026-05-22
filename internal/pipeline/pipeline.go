package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ksupera/prcheck/internal/claude"
	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/jira"
)

var ErrEmptyDiff = errors.New("no diff returned for PR")

type JiraIssue = jira.Issue

type GHFetcher interface {
	FetchDiff(ctx context.Context, url string) (string, error)
	FetchPRDetail(ctx context.Context, url string) (*github.PRDetail, error)
}

type JiraClient interface {
	FetchIssue(ctx context.Context, key string) (*JiraIssue, error)
}

type ClaudeInvoker interface {
	Invoke(ctx context.Context, prompt string) (*claude.Review, error)
}

type Poster interface {
	PostPendingReview(ctx context.Context, prURL, summary string, comments []github.ReviewComment) (int64, error)
}

type Deps struct {
	GH     GHFetcher
	Jira   JiraClient
	Claude ClaudeInvoker
	Post   Poster
}

type Event struct {
	Step string // "diff", "detail", "jira", "claude", "post"
	Note string // optional human-readable detail
	Err  error  // non-nil if step failed (but non-fatal)
}

func Run(ctx context.Context, d Deps, prURL string, emit func(Event)) (int64, error) {
	emit(Event{Step: "diff"})
	diff, err := d.GH.FetchDiff(ctx, prURL)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(diff) == "" {
		return 0, ErrEmptyDiff
	}
	cappedDiff, truncated := CapDiff(diff, DefaultDiffCap)

	emit(Event{Step: "detail"})
	detail, err := d.GH.FetchPRDetail(ctx, prURL)
	if err != nil {
		return 0, err
	}
	checks := github.SummarizeChecks(detail.Checks)

	emit(Event{Step: "jira"})
	jiraKey := jira.ExtractKey(detail.Body, detail.HeadRefName)
	var iss *JiraIssue
	if jiraKey != "" {
		issue, ferr := d.Jira.FetchIssue(ctx, jiraKey)
		if ferr != nil {
			emit(Event{Step: "jira", Err: ferr, Note: "continuing without Jira"})
			iss = nil
		} else {
			iss = issue
		}
	}

	emit(Event{Step: "claude"})
	in := claude.PromptInput{
		Title:         detail.Title,
		HeadRef:       detail.HeadRefName,
		BaseRef:       detail.BaseRefName,
		Author:        detail.Author,
		FailedChecks:  checks.FailedNames,
		PendingChecks: checks.PendingNames,
		Diff:          cappedDiff,
		DiffTruncated: truncated,
	}
	if iss != nil {
		in.JiraKey = iss.Key
		in.JiraSummary = iss.Summary
		in.JiraDesc = iss.Description
	}
	prompt := claude.BuildPrompt(in)
	review, err := d.Claude.Invoke(ctx, prompt)
	if err != nil {
		return 0, err
	}

	emit(Event{Step: "post"})
	ghComments := make([]github.ReviewComment, len(review.Comments))
	for i, c := range review.Comments {
		ghComments[i] = github.ReviewComment{
			Path:     c.Path,
			Line:     c.Line,
			Side:     c.Side,
			Body:     "[" + c.Severity + "] " + c.Body,
			Severity: c.Severity,
		}
	}
	id, err := d.Post.PostPendingReview(ctx, prURL, review.Summary, ghComments)
	if err != nil {
		dumpFailedReview(prURL, review.Summary, ghComments)
		return 0, err
	}
	return id, nil
}

// dumpFailedReview persists the review payload to /tmp so a failed POST
// doesn't lose the Claude output. Best-effort: errors are swallowed.
func dumpFailedReview(prURL, summary string, comments []github.ReviewComment) {
	safe := strings.NewReplacer("/", "_", ":", "_").Replace(prURL)
	path := filepath.Join(os.TempDir(), fmt.Sprintf("prcheck-%s-%d.json", safe, time.Now().Unix()))
	body, mErr := github.BuildReviewBody(summary, comments)
	if mErr != nil {
		return
	}
	_ = os.WriteFile(path, body, 0o600)
}
