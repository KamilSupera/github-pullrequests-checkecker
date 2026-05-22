package pipeline

import (
	"context"
	"testing"

	"github.com/ksupera/prcheck/internal/claude"
	"github.com/ksupera/prcheck/internal/github"
)

// Fakes implementing the Deps interface.
type fakeFetcher struct{}

func (fakeFetcher) FetchDiff(ctx context.Context, url string) (string, error) {
	return "diff --git a/x b/x\n+content\n", nil
}
func (fakeFetcher) FetchPRDetail(ctx context.Context, url string) (*github.PRDetail, error) {
	d := &github.PRDetail{Body: "Implements ABC-123", Checks: []github.Check{
		{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
	}}
	d.PR.Title = "Add cache"
	d.PR.HeadRefName = "feat/cache"
	d.PR.BaseRefName = "develop"
	d.PR.Author = "kamil"
	return d, nil
}

type fakeJira struct{}

func (fakeJira) FetchIssue(ctx context.Context, key string) (*JiraIssue, error) {
	return &JiraIssue{Key: key, Summary: "Add cache", Description: "Need cache"}, nil
}

type fakeClaude struct{}

func (fakeClaude) Invoke(ctx context.Context, prompt string) (*claude.Review, error) {
	return &claude.Review{
		Summary: "LGTM",
		Comments: []claude.ReviewComment{
			{Path: "x", Line: 1, Side: "RIGHT", Body: "nit", Severity: "nit"},
		},
	}, nil
}

type fakePoster struct {
	lastSummary  string
	lastComments []github.ReviewComment
}

func (p *fakePoster) PostPendingReview(ctx context.Context, url, summary string, c []github.ReviewComment) (int64, error) {
	p.lastSummary = summary
	p.lastComments = c
	return 42, nil
}

func TestRun_HappyPath(t *testing.T) {
	poster := &fakePoster{}
	deps := Deps{
		GH:     fakeFetcher{},
		Jira:   fakeJira{},
		Claude: fakeClaude{},
		Post:   poster,
	}

	var events []string
	id, err := Run(t.Context(), deps, "https://github.com/o/r/pull/1", func(e Event) {
		events = append(events, e.Step)
	})
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if id != 42 {
		t.Errorf("id = %d", id)
	}
	if poster.lastSummary != "LGTM" {
		t.Errorf("summary = %q", poster.lastSummary)
	}
	if len(poster.lastComments) != 1 {
		t.Errorf("comments = %v", poster.lastComments)
	}
	wantSteps := []string{"diff", "detail", "jira", "claude", "post"}
	if len(events) != len(wantSteps) {
		t.Fatalf("events = %v", events)
	}
	for i, w := range wantSteps {
		if events[i] != w {
			t.Errorf("events[%d] = %q, want %q", i, events[i], w)
		}
	}
}
