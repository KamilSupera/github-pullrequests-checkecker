package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

type emptyDiffFetcher struct{ fakeFetcher }

func (emptyDiffFetcher) FetchDiff(ctx context.Context, url string) (string, error) {
	return "", nil
}

func TestRun_EmptyDiffAborts(t *testing.T) {
	deps := Deps{
		GH:     emptyDiffFetcher{},
		Jira:   fakeJira{},
		Claude: fakeClaude{},
		Post:   &fakePoster{},
	}
	_, err := Run(t.Context(), deps, "https://github.com/o/r/pull/1", func(Event) {})
	if !errors.Is(err, ErrEmptyDiff) {
		t.Fatalf("want ErrEmptyDiff, got %v", err)
	}
}

type failingPoster struct{}

func (failingPoster) PostPendingReview(ctx context.Context, url, summary string, c []github.ReviewComment) (int64, error) {
	return 0, errors.New("boom")
}

func TestRun_PostFailureDumpsReview(t *testing.T) {
	deps := Deps{
		GH:     fakeFetcher{},
		Jira:   fakeJira{},
		Claude: fakeClaude{},
		Post:   failingPoster{},
	}
	_, err := Run(t.Context(), deps, "https://github.com/o/r/pull/99", func(Event) {})
	if err == nil {
		t.Fatal("expected post failure error")
	}

	entries, _ := os.ReadDir(os.TempDir())
	var found string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "prcheck-") && strings.Contains(e.Name(), "pull_99") && strings.HasSuffix(e.Name(), ".json") {
			found = filepath.Join(os.TempDir(), e.Name())
			break
		}
	}
	if found == "" {
		t.Fatal("expected dumped review JSON in TempDir matching prcheck-*pull_99*.json")
	}
	defer os.Remove(found)

	body, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	if !strings.Contains(string(body), `"body":"LGTM"`) {
		t.Errorf("dump missing review summary: %s", body)
	}
}
