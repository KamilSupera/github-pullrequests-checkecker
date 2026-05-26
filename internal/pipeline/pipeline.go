package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ksupera/prcheck/internal/claude"
	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/jira"
)

var ErrEmptyDiff = errors.New("no diff returned for PR")

// Result is what Run produces on success: the GitHub review ID plus
// the summary, aspect checklist, and inline comments the TUI displays.
type Result struct {
	ID       int64
	Summary  string
	Aspects  []claude.Aspect
	Comments []github.ReviewComment
}

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

	// Focus is forwarded to the prompt to emphasize particular review
	// aspects. Empty means balanced review.
	Focus []string
}

type Event struct {
	Step   string // "diff", "detail", "jira", "claude", "post"
	Status string // "start", "done", "skip", "warn", "error"
	Note   string // human-readable result detail
	Err    error  // set when Status=="warn" or "error"
}

func Run(ctx context.Context, d Deps, prURL string, emit func(Event)) (*Result, error) {
	// diff
	emit(Event{Step: "diff", Status: "start", Note: "fetching unified diff"})
	diff, err := d.GH.FetchDiff(ctx, prURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(diff) == "" {
		return nil, ErrEmptyDiff
	}
	cappedDiff, truncated := CapDiff(diff, DefaultDiffCap)
	files, addLines, delLines := summarizeDiff(diff)
	diffNote := fmt.Sprintf("%d files, +%d / -%d lines", files, addLines, delLines)
	if truncated {
		diffNote += fmt.Sprintf(" (truncated at %d chars)", DefaultDiffCap)
	}
	emit(Event{Step: "diff", Status: "done", Note: diffNote})

	// detail + checks
	emit(Event{Step: "detail", Status: "start", Note: "fetching PR metadata and CI checks"})
	detail, err := d.GH.FetchPRDetail(ctx, prURL)
	if err != nil {
		return nil, err
	}
	checks := github.SummarizeChecks(detail.Checks)
	detailNote := fmt.Sprintf("%q by @%s — %d passed, %d failed, %d pending",
		truncateStr(detail.Title, 40), detail.Author, checks.Passed, checks.Failed, checks.Pending)
	emit(Event{Step: "detail", Status: "done", Note: detailNote})

	// jira
	jiraKey := jira.ExtractKey(detail.Body, detail.HeadRefName)
	var iss *JiraIssue
	if jiraKey == "" {
		emit(Event{Step: "jira", Status: "skip", Note: "no Jira key in PR body or branch name"})
	} else {
		emit(Event{Step: "jira", Status: "start", Note: "fetching " + jiraKey + " via Atlassian MCP"})
		issue, ferr := d.Jira.FetchIssue(ctx, jiraKey)
		if ferr != nil {
			emit(Event{Step: "jira", Status: "warn", Err: ferr, Note: "fetch failed; continuing without Jira context"})
			iss = nil
		} else {
			iss = issue
			emit(Event{Step: "jira", Status: "done", Note: fmt.Sprintf("%s: %s", iss.Key, truncateStr(iss.Summary, 60))})
		}
	}

	// claude
	in := claude.PromptInput{
		Title:         detail.Title,
		HeadRef:       detail.HeadRefName,
		BaseRef:       detail.BaseRefName,
		Author:        detail.Author,
		FailedChecks:  checks.FailedNames,
		PendingChecks: checks.PendingNames,
		Diff:          cappedDiff,
		DiffTruncated: truncated,
		Focus:         d.Focus,
	}
	if iss != nil {
		in.JiraKey = iss.Key
		in.JiraSummary = iss.Summary
		in.JiraDesc = iss.Description
	}
	prompt := claude.BuildPrompt(in)
	emit(Event{Step: "claude", Status: "start", Note: fmt.Sprintf("invoking claude (%d-char prompt) with caveman:caveman-review", len(prompt))})
	start := time.Now()
	review, err := d.Claude.Invoke(ctx, prompt)
	if err != nil {
		return nil, err
	}
	// Filter out low-priority comments (post-step keeps blocker/major).
	var kept []claude.ReviewComment
	droppedSeverity := 0
	for _, c := range review.Comments {
		if c.Severity == "blocker" || c.Severity == "major" {
			kept = append(kept, c)
		} else {
			droppedSeverity++
		}
	}

	// Filter against the diff so GitHub doesn't reject the POST as 422
	// for comments referencing lines outside any hunk.
	valid := ParseValidLines(diff)
	var addressable []claude.ReviewComment
	droppedOutOfDiff := 0
	for _, c := range kept {
		if valid.Allows(c.Path, c.Line, c.Side) {
			addressable = append(addressable, c)
		} else {
			droppedOutOfDiff++
		}
	}
	kept = addressable

	claudeNote := fmt.Sprintf("got review in %s: %d kept (%d nit/minor, %d out-of-diff)",
		time.Since(start).Truncate(time.Second), len(kept), droppedSeverity, droppedOutOfDiff)
	emit(Event{Step: "claude", Status: "done", Note: claudeNote})

	// post
	emit(Event{Step: "post", Status: "start", Note: fmt.Sprintf("POSTing %d inline comments as PENDING review", len(kept))})
	ghComments := make([]github.ReviewComment, len(kept))
	for i, c := range kept {
		ghComments[i] = github.ReviewComment{
			Path:     c.Path,
			Line:     c.Line,
			Side:     c.Side,
			Body:     stripDecorations(c.Body),
			Severity: c.Severity,
		}
	}
	id, err := d.Post.PostPendingReview(ctx, prURL, review.Summary, ghComments)
	if err != nil {
		// Retry once with no inline comments — most 422s are about a
		// single bad line reference; the summary alone is still useful.
		if strings.Contains(err.Error(), "HTTP 422") || strings.Contains(err.Error(), "Unprocessable Entity") {
			emit(Event{Step: "post", Status: "warn", Err: err, Note: "GitHub rejected inline comments (422); retrying summary-only"})
			id2, err2 := d.Post.PostPendingReview(ctx, prURL, review.Summary+
				fmt.Sprintf("\n\n_(%d inline comments dropped: GitHub rejected one or more line references.)_", len(ghComments)),
				nil)
			if err2 == nil {
				emit(Event{Step: "post", Status: "done", Note: fmt.Sprintf("review #%d posted (summary only)", id2)})
				return &Result{ID: id2, Summary: review.Summary, Aspects: review.Aspects, Comments: nil}, nil
			}
			dumpFailedReview(prURL, review.Summary, ghComments)
			return nil, err2
		}
		dumpFailedReview(prURL, review.Summary, ghComments)
		return nil, err
	}
	emit(Event{Step: "post", Status: "done", Note: fmt.Sprintf("review #%d posted as PENDING", id)})
	return &Result{ID: id, Summary: review.Summary, Aspects: review.Aspects, Comments: ghComments}, nil
}

// summarizeDiff counts the number of files touched and added/removed
// content lines (excluding diff headers and hunk markers).
func summarizeDiff(diff string) (files, adds, dels int) {
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git"):
			files++
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "@@"):
			// skip
		case strings.HasPrefix(line, "+"):
			adds++
		case strings.HasPrefix(line, "-"):
			dels++
		}
	}
	return
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// stripDecorations removes severity prefixes and leading emoji/icons
// Claude sometimes prepends to comment bodies (e.g. "[blocker] 🔴 ...",
// "**MAJOR**: ...", "⚠️ L28: ..."). We want clean prose; severity is
// tracked separately and the line is implicit from the comment anchor.
var (
	reSevPrefix  = regexp.MustCompile(`(?i)^\s*[\[(*]+\s*(blocker|major|minor|nit|warning|error|fixme|todo)\s*[\])*:]+\s*`)
	reLeadingEmoji = regexp.MustCompile(`^\s*[\p{So}\p{Sk}\p{Sm}\p{M}\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}\x{FE00}-\x{FE0F}\x{200D}]+\s*`)
	reLPrefix    = regexp.MustCompile(`(?i)^\s*L\d+\s*[:\-]?\s*`)
)

func stripDecorations(s string) string {
	prev := ""
	out := strings.TrimSpace(s)
	for prev != out {
		prev = out
		out = reSevPrefix.ReplaceAllString(out, "")
		out = reLeadingEmoji.ReplaceAllString(out, "")
		out = reLPrefix.ReplaceAllString(out, "")
		out = strings.TrimSpace(out)
	}
	return out
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
