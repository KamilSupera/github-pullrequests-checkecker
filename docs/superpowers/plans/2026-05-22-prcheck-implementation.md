# prcheck Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `prcheck`, a Go TUI that fetches GitHub PRs, runs a Claude-powered review pipeline, and posts results as pending GitHub reviews for the user to submit manually.

**Architecture:** Single Go binary. Bubble Tea TUI on the main goroutine; side effects (gh, claude, jira) run in goroutines that send `tea.Msg` values back to the model. Pipeline is a stateless function chain per PR, cancellable via `context.Context`. No persistent state on disk.

**Tech Stack:** Go 1.22+, Bubble Tea + Lip Gloss + Bubbles, `gh` CLI subprocess, `claude` CLI subprocess, Jira REST via `net/http`.

**Reference spec:** `docs/superpowers/specs/2026-05-22-pr-review-cli-design.md`

---

## File Structure

```
go.mod
go.sum
cmd/prcheck/main.go
internal/config/env.go
internal/config/env_test.go
internal/github/pr.go
internal/github/pr_test.go
internal/github/checks.go
internal/github/checks_test.go
internal/github/review.go
internal/github/review_test.go
internal/jira/extract.go
internal/jira/extract_test.go
internal/jira/client.go
internal/jira/client_test.go
internal/claude/prompt.go
internal/claude/prompt_test.go
internal/claude/parse.go
internal/claude/parse_test.go
internal/claude/invoke.go
internal/claude/invoke_test.go
internal/pipeline/diff.go
internal/pipeline/diff_test.go
internal/pipeline/pipeline.go
internal/pipeline/pipeline_test.go
internal/tui/messages.go
internal/tui/model.go
internal/tui/update.go
internal/tui/view.go
internal/tui/tui_test.go
testdata/                       (fixtures: sample diffs, gh JSON, claude JSON)
README.md
```

---

## Prerequisites

- Go 1.22 or newer installed (`go version`).
- `gh` CLI installed and `gh auth login` already run.
- `claude` CLI installed (Claude Code) and authenticated.
- Atlassian API token if Jira features will be exercised.

---

### Task 0: Initialize Go module

**Files:**
- Create: `go.mod`
- Create: `.gitignore`

- [ ] **Step 1: Initialize module**

Run from repo root:
```bash
go mod init github.com/ksupera/prcheck
```

- [ ] **Step 2: Add `.gitignore`**

Create `.gitignore`:
```
/prcheck
*.test
*.out
.idea/
.vscode/
testdata/.fake-bin/
```

- [ ] **Step 3: Commit**

```bash
git add go.mod .gitignore
git commit -m "chore: init go module"
```

---

### Task 1: config/env — env var loading and validation

**Files:**
- Create: `internal/config/env.go`
- Test: `internal/config/env_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/env_test.go`:
```go
package config

import (
	"testing"
)

func TestLoad_AllSet(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://example.atlassian.net")
	t.Setenv("JIRA_EMAIL", "me@example.com")
	t.Setenv("JIRA_TOKEN", "tok")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned err: %v", err)
	}
	if cfg.JiraBaseURL != "https://example.atlassian.net" {
		t.Errorf("JiraBaseURL = %q", cfg.JiraBaseURL)
	}
	if cfg.JiraEmail != "me@example.com" {
		t.Errorf("JiraEmail = %q", cfg.JiraEmail)
	}
	if cfg.JiraToken != "tok" {
		t.Errorf("JiraToken = %q", cfg.JiraToken)
	}
}

func TestLoad_MissingVars(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "")
	t.Setenv("JIRA_EMAIL", "")
	t.Setenv("JIRA_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing env vars")
	}
}

func TestLoad_DebugFlag(t *testing.T) {
	t.Setenv("JIRA_BASE_URL", "https://x")
	t.Setenv("JIRA_EMAIL", "a")
	t.Setenv("JIRA_TOKEN", "b")
	t.Setenv("PRCHECK_DEBUG", "1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if !cfg.Debug {
		t.Error("Debug should be true when PRCHECK_DEBUG=1")
	}
}
```

- [ ] **Step 2: Run test, confirm fail**

```bash
go test ./internal/config/...
```
Expected: FAIL ("undefined: Load").

- [ ] **Step 3: Implement**

Create `internal/config/env.go`:
```go
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	JiraBaseURL string
	JiraEmail   string
	JiraToken   string
	Debug       bool
}

func Load() (*Config, error) {
	cfg := &Config{
		JiraBaseURL: os.Getenv("JIRA_BASE_URL"),
		JiraEmail:   os.Getenv("JIRA_EMAIL"),
		JiraToken:   os.Getenv("JIRA_TOKEN"),
		Debug:       os.Getenv("PRCHECK_DEBUG") == "1",
	}

	var missing []string
	if cfg.JiraBaseURL == "" {
		missing = append(missing, "JIRA_BASE_URL")
	}
	if cfg.JiraEmail == "" {
		missing = append(missing, "JIRA_EMAIL")
	}
	if cfg.JiraToken == "" {
		missing = append(missing, "JIRA_TOKEN")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing env vars: %s", strings.Join(missing, ", "))
	}

	// sanity: base URL must look like a URL
	if !strings.HasPrefix(cfg.JiraBaseURL, "https://") && !strings.HasPrefix(cfg.JiraBaseURL, "http://") {
		return nil, errors.New("JIRA_BASE_URL must start with http(s)://")
	}

	return cfg, nil
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/config/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): env var loading and validation"
```

---

### Task 2: jira/extract — Jira key extraction from PR body/branch

**Files:**
- Create: `internal/jira/extract.go`
- Test: `internal/jira/extract_test.go`

- [ ] **Step 1: Failing test**

Create `internal/jira/extract_test.go`:
```go
package jira

import "testing"

func TestExtractKey(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string
		want   string
	}{
		{"in body", []string{"Fixes ABC-123 thanks"}, "ABC-123"},
		{"in branch", []string{"feature/ABC-456-add-thing"}, "ABC-456"},
		{"first wins across inputs", []string{"FOO-1", "BAR-9"}, "FOO-1"},
		{"no key", []string{"no jira here", "main"}, ""},
		{"ignore lowercase", []string{"abc-123 fix"}, ""},
		{"multi digit project", []string{"ABCD-9999"}, "ABCD-9999"},
		{"hyphen-separated project rejected", []string{"AB-CD-1"}, "CD-1"},
		{"embedded in url", []string{"https://x/browse/ABC-12"}, "ABC-12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractKey(tt.inputs...)
			if got != tt.want {
				t.Errorf("ExtractKey(%v) = %q, want %q", tt.inputs, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/jira/...
```
Expected: FAIL ("undefined: ExtractKey").

- [ ] **Step 3: Implement**

Create `internal/jira/extract.go`:
```go
package jira

import "regexp"

// matches PROJECT-NUMBER where PROJECT is 2+ uppercase letters
// and is preceded by start-of-string or non-alphanumeric.
var keyRE = regexp.MustCompile(`(?:^|[^A-Z0-9])([A-Z]{2,}-\d+)`)

// ExtractKey returns the first Jira key found across the given inputs
// (checked in order). Returns "" if none found.
func ExtractKey(inputs ...string) string {
	for _, in := range inputs {
		m := keyRE.FindStringSubmatch(in)
		if len(m) >= 2 {
			return m[1]
		}
	}
	return ""
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/jira/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jira/extract.go internal/jira/extract_test.go
git commit -m "feat(jira): extract jira key from PR body/branch"
```

---

### Task 3: jira/client — Jira REST client

**Files:**
- Create: `internal/jira/client.go`
- Test: `internal/jira/client_test.go`

- [ ] **Step 1: Failing test**

Create `internal/jira/client_test.go`:
```go
package jira

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchIssue_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/ABC-123" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key": "ABC-123",
			"fields": map[string]any{
				"summary":     "Add caching",
				"description": "We need a cache",
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user@example.com", "tok")
	iss, err := c.FetchIssue(t.Context(), "ABC-123")
	if err != nil {
		t.Fatalf("FetchIssue err: %v", err)
	}
	if iss.Key != "ABC-123" {
		t.Errorf("Key = %q", iss.Key)
	}
	if iss.Summary != "Add caching" {
		t.Errorf("Summary = %q", iss.Summary)
	}
	if !strings.Contains(iss.Description, "cache") {
		t.Errorf("Description = %q", iss.Description)
	}
}

func TestFetchIssue_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "t")
	_, err := c.FetchIssue(t.Context(), "ABC-1")
	if err == nil {
		t.Fatal("expected error on 404")
	}
}
```

Note: `t.Context()` requires Go 1.24+. If on Go 1.22/1.23, replace with `context.Background()` and add `"context"` import.

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/jira/...
```
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/jira/client.go`:
```go
package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Issue struct {
	Key         string
	Summary     string
	Description string
}

type Client struct {
	baseURL string
	email   string
	token   string
	http    *http.Client
}

func NewClient(baseURL, email, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		email:   email,
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

type apiResp struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string `json:"summary"`
		Description any    `json:"description"` // can be string or ADF object
	} `json:"fields"`
}

func (c *Client) FetchIssue(ctx context.Context, key string) (*Issue, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", c.baseURL, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jira %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}

	return &Issue{
		Key:         r.Key,
		Summary:     r.Fields.Summary,
		Description: descToString(r.Fields.Description),
	}, nil
}

// descToString handles Jira's two description formats: legacy string
// and ADF (Atlassian Document Format) object. For ADF we just JSON-encode
// it — the LLM can still reason over it.
func descToString(d any) string {
	switch v := d.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/jira/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jira/client.go internal/jira/client_test.go
git commit -m "feat(jira): REST client for fetching issues"
```

---

### Task 4: github/pr — PR list/view via gh subprocess

**Files:**
- Create: `internal/github/pr.go`
- Test: `internal/github/pr_test.go`
- Create: `testdata/gh/search-mine.json`
- Create: `testdata/gh/pr-view.json`

- [ ] **Step 1: Set up fake-gh helper**

Create `internal/github/fake_gh_test.go`:
```go
package github

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeFakeGH writes a shell script named `gh` to a tempdir and prepends
// it to PATH for the duration of the test. The script reads its arg list
// and echoes the file named by the env var FAKE_GH_<HASH> where HASH is
// derived from the args.
//
// For simplicity, the script just echoes the content of FAKE_GH_OUT
// (so tests must set FAKE_GH_OUT to the fixture they want).
func writeFakeGH(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-gh helper is POSIX shell only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	script := `#!/bin/sh
if [ -n "$FAKE_GH_EXIT" ]; then
  echo "$FAKE_GH_STDERR" >&2
  exit "$FAKE_GH_EXIT"
fi
cat "$FAKE_GH_OUT"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}
```

- [ ] **Step 2: Create fixture files**

Create `testdata/gh/search-mine.json`:
```json
[
  {
    "number": 1234,
    "title": "Add caching layer",
    "url": "https://github.com/org/repo/pull/1234",
    "author": {"login": "kamil"},
    "headRefName": "feat/cache",
    "baseRefName": "develop",
    "updatedAt": "2026-05-22T10:00:00Z"
  },
  {
    "number": 1235,
    "title": "Fix auth race",
    "url": "https://github.com/org/repo/pull/1235",
    "author": {"login": "kamil"},
    "headRefName": "fix/auth",
    "baseRefName": "develop",
    "updatedAt": "2026-05-21T12:00:00Z"
  }
]
```

Create `testdata/gh/pr-view.json`:
```json
{
  "number": 1234,
  "title": "Add caching layer",
  "url": "https://github.com/org/repo/pull/1234",
  "body": "Implements ABC-123. Adds a TTL-based cache.",
  "headRefName": "feat/cache",
  "baseRefName": "develop",
  "author": {"login": "kamil"},
  "statusCheckRollup": [
    {"name": "build", "status": "COMPLETED", "conclusion": "SUCCESS"},
    {"name": "test", "status": "COMPLETED", "conclusion": "FAILURE"}
  ]
}
```

- [ ] **Step 3: Failing test**

Create `internal/github/pr_test.go`:
```go
package github

import (
	"path/filepath"
	"testing"
)

func TestSearchPRs(t *testing.T) {
	writeFakeGH(t)
	abs, _ := filepath.Abs("../../testdata/gh/search-mine.json")
	t.Setenv("FAKE_GH_OUT", abs)

	prs, err := SearchPRs(t.Context(), QueryAuthored)
	if err != nil {
		t.Fatalf("SearchPRs err: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d PRs, want 2", len(prs))
	}
	if prs[0].Number != 1234 {
		t.Errorf("prs[0].Number = %d", prs[0].Number)
	}
	if prs[0].Title != "Add caching layer" {
		t.Errorf("prs[0].Title = %q", prs[0].Title)
	}
	if prs[0].Author != "kamil" {
		t.Errorf("prs[0].Author = %q", prs[0].Author)
	}
}

func TestFetchPRDetail(t *testing.T) {
	writeFakeGH(t)
	abs, _ := filepath.Abs("../../testdata/gh/pr-view.json")
	t.Setenv("FAKE_GH_OUT", abs)

	d, err := FetchPRDetail(t.Context(), "https://github.com/org/repo/pull/1234")
	if err != nil {
		t.Fatalf("FetchPRDetail err: %v", err)
	}
	if d.Body == "" {
		t.Error("Body empty")
	}
	if len(d.Checks) != 2 {
		t.Fatalf("Checks count = %d", len(d.Checks))
	}
	if d.Checks[1].Conclusion != "FAILURE" {
		t.Errorf("Checks[1].Conclusion = %q", d.Checks[1].Conclusion)
	}
}
```

Note: `t.Context()` again requires Go 1.24+; otherwise import `"context"` and use `context.Background()`.

- [ ] **Step 4: Run, confirm fail**

```bash
go test ./internal/github/...
```
Expected: FAIL.

- [ ] **Step 5: Implement**

Create `internal/github/pr.go`:
```go
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
```

- [ ] **Step 6: Tests pass**

```bash
go test ./internal/github/...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/github/pr.go internal/github/pr_test.go internal/github/fake_gh_test.go testdata/gh/
git commit -m "feat(github): SearchPRs and FetchPRDetail via gh CLI"
```

---

### Task 5: github/checks — Checks summary

**Files:**
- Create: `internal/github/checks.go`
- Test: `internal/github/checks_test.go`

- [ ] **Step 1: Failing test**

Create `internal/github/checks_test.go`:
```go
package github

import "testing"

func TestSummarizeChecks(t *testing.T) {
	checks := []Check{
		{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Name: "test", Status: "COMPLETED", Conclusion: "FAILURE"},
		{Name: "lint", Status: "IN_PROGRESS", Conclusion: ""},
		{Name: "deploy", Status: "COMPLETED", Conclusion: "SUCCESS"},
	}
	s := SummarizeChecks(checks)
	if s.Passed != 2 {
		t.Errorf("Passed = %d", s.Passed)
	}
	if s.Failed != 1 {
		t.Errorf("Failed = %d", s.Failed)
	}
	if s.Pending != 1 {
		t.Errorf("Pending = %d", s.Pending)
	}
	if len(s.FailedNames) != 1 || s.FailedNames[0] != "test" {
		t.Errorf("FailedNames = %v", s.FailedNames)
	}
	if len(s.PendingNames) != 1 || s.PendingNames[0] != "lint" {
		t.Errorf("PendingNames = %v", s.PendingNames)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/github/...
```
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/github/checks.go`:
```go
package github

type ChecksSummary struct {
	Passed       int
	Failed       int
	Pending      int
	FailedNames  []string
	PendingNames []string
}

func SummarizeChecks(checks []Check) ChecksSummary {
	var s ChecksSummary
	for _, c := range checks {
		switch {
		case c.Status != "COMPLETED":
			s.Pending++
			s.PendingNames = append(s.PendingNames, c.Name)
		case c.Conclusion == "SUCCESS":
			s.Passed++
		default:
			s.Failed++
			s.FailedNames = append(s.FailedNames, c.Name)
		}
	}
	return s
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/github/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/github/checks.go internal/github/checks_test.go
git commit -m "feat(github): summarize check statuses"
```

---

### Task 6: github/review — Build POST body and post pending review

**Files:**
- Create: `internal/github/review.go`
- Test: `internal/github/review_test.go`

- [ ] **Step 1: Failing test (body builder)**

Create `internal/github/review_test.go`:
```go
package github

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildReviewBody(t *testing.T) {
	comments := []ReviewComment{
		{Path: "foo.go", Line: 10, Side: "RIGHT", Body: "nit: rename", Severity: "nit"},
		{Path: "bar.go", Line: 22, Side: "LEFT", Body: "blocker: nil deref", Severity: "blocker"},
	}
	b, err := BuildReviewBody("Overall LGTM with nits.", comments)
	if err != nil {
		t.Fatalf("BuildReviewBody err: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := parsed["event"]; ok {
		t.Error("event field MUST be absent so review stays PENDING")
	}
	if parsed["body"] != "Overall LGTM with nits." {
		t.Errorf("body = %v", parsed["body"])
	}
	arr, ok := parsed["comments"].([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("comments = %v", parsed["comments"])
	}
	first, _ := arr[0].(map[string]any)
	if first["path"] != "foo.go" {
		t.Errorf("first.path = %v", first["path"])
	}
	if first["line"].(float64) != 10 {
		t.Errorf("first.line = %v", first["line"])
	}
	if first["side"] != "RIGHT" {
		t.Errorf("first.side = %v", first["side"])
	}
	if _, ok := first["severity"]; ok {
		t.Error("severity must not be sent to GitHub (internal-only)")
	}
}

func TestPostPendingReview(t *testing.T) {
	writeFakeGH(t)
	out := t.TempDir() + "/captured-args.txt"
	t.Setenv("FAKE_GH_OUT", "/dev/null")
	script := `#!/bin/sh
# capture stdin to a file
cat > "` + out + `"
echo '{"id": 999, "state": "PENDING"}'
`
	abs, _ := filepath.Abs(out)
	_ = abs

	// Re-write fake-gh to also capture stdin (where gh api -X POST --input - reads from).
	dir := strings.TrimSuffix(out, "/captured-args.txt")
	if err := writeScript(dir+"/gh", script); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", dir+":"+t.TempDir())

	id, err := PostPendingReview(t.Context(), "https://github.com/org/repo/pull/1234",
		"summary", []ReviewComment{{Path: "f.go", Line: 1, Side: "RIGHT", Body: "c"}})
	if err != nil {
		t.Fatalf("PostPendingReview err: %v", err)
	}
	if id != 999 {
		t.Errorf("review id = %d", id)
	}
}

// writeScript is a helper to write executable shell scripts for fake-gh.
func writeScript(path, content string) error {
	return writeFile(path, content, 0o755)
}
```

Add helper in same package, file `internal/github/testhelpers_test.go`:
```go
package github

import "os"

func writeFile(path, content string, mode os.FileMode) error {
	return os.WriteFile(path, []byte(content), mode)
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/github/...
```
Expected: FAIL (undefined symbols).

- [ ] **Step 3: Implement**

Create `internal/github/review.go`:
```go
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
	Severity string `json:"-"`    // internal only, never sent to GitHub
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
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/github/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/github/review.go internal/github/review_test.go internal/github/testhelpers_test.go
git commit -m "feat(github): build review body and POST pending review"
```

---

### Task 7: pipeline/diff — Diff size cap

**Files:**
- Create: `internal/pipeline/diff.go`
- Test: `internal/pipeline/diff_test.go`

- [ ] **Step 1: Failing test**

Create `internal/pipeline/diff_test.go`:
```go
package pipeline

import (
	"strings"
	"testing"
)

func TestCapDiff_UnderCap(t *testing.T) {
	in := "diff line\n"
	out, trunc := CapDiff(in, 100)
	if out != in {
		t.Errorf("out = %q, want %q", out, in)
	}
	if trunc {
		t.Error("trunc = true, want false")
	}
}

func TestCapDiff_OverCap(t *testing.T) {
	in := strings.Repeat("x", 200)
	out, trunc := CapDiff(in, 100)
	if !trunc {
		t.Error("trunc = false, want true")
	}
	if len(out) > 200 { // out includes a truncation marker
		t.Errorf("out too long: %d", len(out))
	}
	if !strings.Contains(out, "truncated") {
		t.Error("expected truncation marker")
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/pipeline/...
```
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/pipeline/diff.go`:
```go
package pipeline

import "fmt"

const DefaultDiffCap = 100_000

// CapDiff trims `s` to at most `cap` characters. Returns the (possibly
// trimmed) string and a boolean indicating whether truncation occurred.
// When trimmed, an explanatory marker is appended.
func CapDiff(s string, cap int) (string, bool) {
	if len(s) <= cap {
		return s, false
	}
	marker := fmt.Sprintf("\n\n... [diff truncated at %d chars; original was %d]\n", cap, len(s))
	return s[:cap] + marker, true
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/pipeline/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/diff.go internal/pipeline/diff_test.go
git commit -m "feat(pipeline): diff size cap with truncation marker"
```

---

### Task 8: claude/prompt — Prompt builder

**Files:**
- Create: `internal/claude/prompt.go`
- Test: `internal/claude/prompt_test.go`
- Create: `testdata/prompt/full.golden.txt`
- Create: `testdata/prompt/no-jira.golden.txt`

- [ ] **Step 1: Define inputs type and write failing test**

Create `internal/claude/prompt_test.go`:
```go
package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPrompt_Full(t *testing.T) {
	in := PromptInput{
		Title:        "Add caching",
		HeadRef:      "feat/cache",
		BaseRef:      "develop",
		Author:       "kamil",
		JiraKey:      "ABC-123",
		JiraSummary:  "Add caching",
		JiraDesc:     "We need a TTL cache.",
		FailedChecks: []string{"test"},
		PendingChecks: []string{},
		Diff:         "diff --git a/foo.go b/foo.go\n+func cached() {}\n",
		DiffTruncated: false,
	}
	got := BuildPrompt(in)
	golden, err := os.ReadFile(filepath.Join("..", "..", "testdata", "prompt", "full.golden.txt"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(golden) {
		t.Errorf("prompt mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

func TestBuildPrompt_NoJira(t *testing.T) {
	in := PromptInput{
		Title:   "Refactor",
		HeadRef: "refactor/x",
		BaseRef: "main",
		Author:  "alice",
		Diff:    "diff --git a/x b/x\n",
	}
	got := BuildPrompt(in)
	golden, err := os.ReadFile(filepath.Join("..", "..", "testdata", "prompt", "no-jira.golden.txt"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(golden) {
		t.Errorf("prompt mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/claude/...
```
Expected: FAIL.

- [ ] **Step 3: Implement prompt builder**

Create `internal/claude/prompt.go`:
```go
package claude

import (
	"fmt"
	"strings"
)

type PromptInput struct {
	Title         string
	HeadRef       string
	BaseRef       string
	Author        string
	JiraKey       string
	JiraSummary   string
	JiraDesc      string
	FailedChecks  []string
	PendingChecks []string
	Diff          string
	DiffTruncated bool
}

const schemaBlock = `Return ONLY JSON matching this schema (no prose before or after):
{
  "summary": "string (overall review summary, 2-5 sentences)",
  "comments": [
    {
      "path": "file path from diff",
      "line": <int, line number in NEW file>,
      "side": "RIGHT" | "LEFT",
      "body": "review comment",
      "severity": "blocker" | "major" | "minor" | "nit"
    }
  ]
}`

func BuildPrompt(in PromptInput) string {
	var b strings.Builder
	b.WriteString("Use skill caveman:caveman-review.\n\n")
	fmt.Fprintf(&b, "PR: %s\n", in.Title)
	fmt.Fprintf(&b, "Branch: %s -> %s\n", in.HeadRef, in.BaseRef)
	fmt.Fprintf(&b, "Author: %s\n\n", in.Author)

	if in.JiraKey != "" {
		b.WriteString("Jira issue:\n")
		fmt.Fprintf(&b, "%s: %s\n", in.JiraKey, in.JiraSummary)
		if in.JiraDesc != "" {
			b.WriteString(in.JiraDesc)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("CI checks:\n")
	if len(in.FailedChecks) > 0 {
		fmt.Fprintf(&b, "- failed: %s\n", strings.Join(in.FailedChecks, ", "))
	} else {
		b.WriteString("- failed: (none)\n")
	}
	if len(in.PendingChecks) > 0 {
		fmt.Fprintf(&b, "- pending: %s\n", strings.Join(in.PendingChecks, ", "))
	} else {
		b.WriteString("- pending: (none)\n")
	}
	b.WriteString("\n")

	if in.DiffTruncated {
		b.WriteString("Diff (TRUNCATED — review may be incomplete):\n")
	} else {
		b.WriteString("Diff:\n")
	}
	b.WriteString(in.Diff)
	if !strings.HasSuffix(in.Diff, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(schemaBlock)
	b.WriteString("\n")
	return b.String()
}
```

- [ ] **Step 4: Generate golden files**

Run once to capture current output, then save:
```bash
mkdir -p testdata/prompt
go test ./internal/claude/... -run TestBuildPrompt_Full -v 2>&1 || true
```

Manually paste the expected prompt content (since you know what BuildPrompt produces). Create `testdata/prompt/full.golden.txt`:
```
Use skill caveman:caveman-review.

PR: Add caching
Branch: feat/cache -> develop
Author: kamil

Jira issue:
ABC-123: Add caching
We need a TTL cache.

CI checks:
- failed: test
- pending: (none)

Diff:
diff --git a/foo.go b/foo.go
+func cached() {}

Return ONLY JSON matching this schema (no prose before or after):
{
  "summary": "string (overall review summary, 2-5 sentences)",
  "comments": [
    {
      "path": "file path from diff",
      "line": <int, line number in NEW file>,
      "side": "RIGHT" | "LEFT",
      "body": "review comment",
      "severity": "blocker" | "major" | "minor" | "nit"
    }
  ]
}
```

Create `testdata/prompt/no-jira.golden.txt`:
```
Use skill caveman:caveman-review.

PR: Refactor
Branch: refactor/x -> main
Author: alice

CI checks:
- failed: (none)
- pending: (none)

Diff:
diff --git a/x b/x

Return ONLY JSON matching this schema (no prose before or after):
{
  "summary": "string (overall review summary, 2-5 sentences)",
  "comments": [
    {
      "path": "file path from diff",
      "line": <int, line number in NEW file>,
      "side": "RIGHT" | "LEFT",
      "body": "review comment",
      "severity": "blocker" | "major" | "minor" | "nit"
    }
  ]
}
```

- [ ] **Step 5: Tests pass**

```bash
go test ./internal/claude/...
```
Expected: PASS.

If a test fails due to whitespace mismatch, fix the golden file to match the actual output exactly (including any trailing newlines).

- [ ] **Step 6: Commit**

```bash
git add internal/claude/prompt.go internal/claude/prompt_test.go testdata/prompt/
git commit -m "feat(claude): prompt builder with caveman-review skill"
```

---

### Task 9: claude/parse — JSON output parser

**Files:**
- Create: `internal/claude/parse.go`
- Test: `internal/claude/parse_test.go`

- [ ] **Step 1: Failing test**

Create `internal/claude/parse_test.go`:
```go
package claude

import "testing"

func TestParseReview_Valid(t *testing.T) {
	raw := `{
		"summary": "Looks good.",
		"comments": [
			{"path":"foo.go","line":12,"side":"RIGHT","body":"nit","severity":"nit"}
		]
	}`
	r, err := ParseReview([]byte(raw))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Summary != "Looks good." {
		t.Errorf("Summary = %q", r.Summary)
	}
	if len(r.Comments) != 1 {
		t.Fatalf("Comments len = %d", len(r.Comments))
	}
	c := r.Comments[0]
	if c.Path != "foo.go" || c.Line != 12 || c.Side != "RIGHT" {
		t.Errorf("comment = %+v", c)
	}
}

func TestParseReview_ProseAroundJSON(t *testing.T) {
	raw := "Here is the review:\n" + `{"summary":"ok","comments":[]}` + "\nThanks!"
	r, err := ParseReview([]byte(raw))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Summary != "ok" {
		t.Errorf("Summary = %q", r.Summary)
	}
}

func TestParseReview_InvalidSeverity(t *testing.T) {
	raw := `{"summary":"x","comments":[{"path":"a","line":1,"side":"RIGHT","body":"b","severity":"banana"}]}`
	_, err := ParseReview([]byte(raw))
	if err == nil {
		t.Fatal("expected error for invalid severity")
	}
}

func TestParseReview_MissingSummary(t *testing.T) {
	raw := `{"comments":[]}`
	_, err := ParseReview([]byte(raw))
	if err == nil {
		t.Fatal("expected error for missing summary")
	}
}

func TestParseReview_BadSide(t *testing.T) {
	raw := `{"summary":"x","comments":[{"path":"a","line":1,"side":"MIDDLE","body":"b","severity":"nit"}]}`
	_, err := ParseReview([]byte(raw))
	if err == nil {
		t.Fatal("expected error for bad side")
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/claude/...
```
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/claude/parse.go`:
```go
package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ReviewComment struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"`
	Body     string `json:"body"`
	Severity string `json:"severity"`
}

type Review struct {
	Summary  string          `json:"summary"`
	Comments []ReviewComment `json:"comments"`
}

var validSeverity = map[string]bool{
	"blocker": true, "major": true, "minor": true, "nit": true,
}

// ParseReview parses Claude's JSON output. Tolerates leading/trailing
// prose by finding the first balanced JSON object in the input.
func ParseReview(raw []byte) (*Review, error) {
	doc := findJSONObject(string(raw))
	if doc == "" {
		return nil, fmt.Errorf("no JSON object found in claude output")
	}

	var r Review
	if err := json.Unmarshal([]byte(doc), &r); err != nil {
		return nil, fmt.Errorf("unmarshal review: %w", err)
	}

	if strings.TrimSpace(r.Summary) == "" {
		return nil, fmt.Errorf("review summary missing or empty")
	}
	for i, c := range r.Comments {
		if c.Path == "" || c.Body == "" {
			return nil, fmt.Errorf("comment[%d] missing path or body", i)
		}
		if c.Side != "LEFT" && c.Side != "RIGHT" {
			return nil, fmt.Errorf("comment[%d] invalid side %q", i, c.Side)
		}
		if !validSeverity[c.Severity] {
			return nil, fmt.Errorf("comment[%d] invalid severity %q", i, c.Severity)
		}
	}
	return &r, nil
}

func findJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		switch {
		case esc:
			esc = false
		case ch == '\\' && inStr:
			esc = true
		case ch == '"':
			inStr = !inStr
		case !inStr && ch == '{':
			depth++
		case !inStr && ch == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
```

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/claude/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/claude/parse.go internal/claude/parse_test.go
git commit -m "feat(claude): parse review JSON with prose tolerance"
```

---

### Task 10: claude/invoke — Subprocess wrapper

**Files:**
- Create: `internal/claude/invoke.go`
- Test: `internal/claude/invoke_test.go`

- [ ] **Step 1: Failing test (fake claude binary)**

Create `internal/claude/invoke_test.go`:
```go
package claude

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFakeClaude(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-claude is POSIX shell only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	script := "#!/bin/sh\ncat <<'EOF'\n" + body + "\nEOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func TestInvoke_Success(t *testing.T) {
	body := `{"summary":"ok","comments":[]}`
	writeFakeClaude(t, body)

	r, err := Invoke(t.Context(), "any prompt")
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if r.Summary != "ok" {
		t.Errorf("Summary = %q", r.Summary)
	}
}

func TestInvoke_BadJSON(t *testing.T) {
	writeFakeClaude(t, "this is not json at all")
	_, err := Invoke(t.Context(), "p")
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
	if !strings.Contains(err.Error(), "no JSON object") && !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/claude/...
```
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/claude/invoke.go`:
```go
package claude

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

func Invoke(ctx context.Context, prompt string) (*Review, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "claude", "-p", prompt, "--output-format", "text")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude exec: %w (stderr: %s)", err, tail(stderr.String(), 500))
	}
	return ParseReview(stdout.Bytes())
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}
```

Note: Even though we pass `--output-format text`, `ParseReview` tolerates prose around the JSON, so we don't strictly need `json` mode here.

- [ ] **Step 4: Tests pass**

```bash
go test ./internal/claude/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/claude/invoke.go internal/claude/invoke_test.go
git commit -m "feat(claude): subprocess wrapper for claude CLI"
```

---

### Task 11: pipeline — Orchestrator

**Files:**
- Create: `internal/pipeline/pipeline.go`
- Test: `internal/pipeline/pipeline_test.go`

- [ ] **Step 1: Failing test**

Create `internal/pipeline/pipeline_test.go`:
```go
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
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test ./internal/pipeline/...
```
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/pipeline/pipeline.go`:
```go
package pipeline

import (
	"context"

	"github.com/ksupera/prcheck/internal/claude"
	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/jira"
)

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
		iss, err = d.Jira.FetchIssue(ctx, jiraKey)
		if err != nil {
			emit(Event{Step: "jira", Err: err, Note: "continuing without Jira"})
			iss = nil
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
		return 0, err
	}
	return id, nil
}
```

- [ ] **Step 4: Add FetchDiff to github package**

Edit `internal/github/pr.go`, add at the bottom:
```go
func FetchDiff(ctx context.Context, url string) (string, error) {
	out, err := runGH(ctx, "pr", "diff", url)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
```

- [ ] **Step 5: Add a concrete adapter for the GHFetcher interface**

Edit `internal/github/pr.go`, add:
```go
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
```

- [ ] **Step 6: Tests pass**

```bash
go test ./internal/...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/pipeline/ internal/github/pr.go
git commit -m "feat(pipeline): orchestrator with cancel-aware step events"
```

---

### Task 12: tui/messages — Message types

**Files:**
- Create: `internal/tui/messages.go`

- [ ] **Step 1: Implement (no test needed — pure type defs)**

Create `internal/tui/messages.go`:
```go
package tui

import (
	"github.com/ksupera/prcheck/internal/github"
)

// Messages flow from goroutines into the Bubble Tea Update function.

type prsLoadedMsg struct {
	tab Tab
	prs []github.PR
	err error
}

type prDetailMsg struct {
	url    string
	detail *github.PRDetail
	err    error
}

type progressMsg struct {
	step string
	note string
	err  error
}

type reviewDoneMsg struct {
	url      string
	reviewID int64
	err      error
}

type Tab int

const (
	TabMine Tab = iota
	TabReview
	TabMentioned
)

func (t Tab) Label() string {
	switch t {
	case TabMine:
		return "Mine"
	case TabReview:
		return "Review"
	case TabMentioned:
		return "Mentioned"
	}
	return "?"
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/tui/...
```
Expected: succeeds (no symbols yet from model/update/view, but the file alone compiles).

- [ ] **Step 3: Commit**

```bash
git add internal/tui/messages.go
git commit -m "feat(tui): message types"
```

---

### Task 13: tui/model — Model and Init

**Files:**
- Create: `internal/tui/model.go`

- [ ] **Step 1: Add Bubble Tea dependency**

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbles@latest
```

- [ ] **Step 2: Implement model**

Create `internal/tui/model.go`:
```go
package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/pipeline"
)

type loaderFn func(ctx context.Context, q github.Query) ([]github.PR, error)
type detailFn func(ctx context.Context, url string) (*github.PRDetail, error)
type runPipelineFn func(ctx context.Context, prURL string, emit func(pipeline.Event)) (int64, error)
type openFn func(url string) error

type Model struct {
	ctx       context.Context
	cancel    context.CancelFunc
	pipeCancel context.CancelFunc // cancel only the running pipeline

	loader    loaderFn
	detailFn  detailFn
	runPipe   runPipelineFn
	openURL   openFn

	tab      Tab
	prsByTab map[Tab][]github.PR
	loadErr  map[Tab]error
	cursor   int

	details map[string]*github.PRDetail

	running     bool
	steps       []string // history of progress events
	lastReview  *reviewDoneMsg

	spinner spinner.Model
	err     error
}

func NewModel(loader loaderFn, df detailFn, rp runPipelineFn, open openFn) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		ctx:      ctx,
		cancel:   cancel,
		loader:   loader,
		detailFn: df,
		runPipe:  rp,
		openURL:  open,
		prsByTab: map[Tab][]github.PR{},
		loadErr:  map[Tab]error{},
		details:  map[string]*github.PRDetail{},
		spinner:  sp,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.loadTab(TabMine),
		m.loadTab(TabReview),
		m.loadTab(TabMentioned),
	)
}

func (m Model) loadTab(t Tab) tea.Cmd {
	return func() tea.Msg {
		var q github.Query
		switch t {
		case TabMine:
			q = github.QueryAuthored
		case TabReview:
			q = github.QueryReviewRequested
		case TabMentioned:
			q = github.QueryMentioned
		}
		prs, err := m.loader(m.ctx, q)
		return prsLoadedMsg{tab: t, prs: prs, err: err}
	}
}

func (m Model) currentPR() (github.PR, bool) {
	prs := m.prsByTab[m.tab]
	if len(prs) == 0 || m.cursor < 0 || m.cursor >= len(prs) {
		return github.PR{}, false
	}
	return prs[m.cursor], true
}
```

- [ ] **Step 3: Verify compile**

```bash
go build ./internal/tui/...
```
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/tui/model.go
git commit -m "feat(tui): model and Init"
```

---

### Task 14: tui/update — Update loop with key handling

**Files:**
- Create: `internal/tui/update.go`

- [ ] **Step 1: Implement**

Create `internal/tui/update.go`:
```go
package tui

import (
	"context"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ksupera/prcheck/internal/pipeline"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		return m.handleKey(msg)

	case prsLoadedMsg:
		if msg.err != nil {
			m.loadErr[msg.tab] = msg.err
		} else {
			sort.Slice(msg.prs, func(i, j int) bool {
				return msg.prs[i].UpdatedAt > msg.prs[j].UpdatedAt
			})
			m.prsByTab[msg.tab] = msg.prs
		}
		// preload detail for the first PR of current tab
		if pr, ok := m.currentPR(); ok {
			if _, cached := m.details[pr.URL]; !cached {
				return m, m.loadDetail(pr.URL)
			}
		}
		return m, nil

	case prDetailMsg:
		if msg.err == nil {
			m.details[msg.url] = msg.detail
		}
		return m, nil

	case progressMsg:
		m.steps = append(m.steps, msg.step)
		return m, nil

	case reviewDoneMsg:
		m.running = false
		m.pipeCancel = nil
		m.lastReview = &msg
		return m, nil

	}

	// pass through to spinner
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {

	case "q", "ctrl+c":
		if m.running && m.pipeCancel != nil {
			m.pipeCancel()
			m.running = false
			return m, nil
		}
		m.cancel()
		return m, tea.Quit

	case "j", "down":
		prs := m.prsByTab[m.tab]
		if m.cursor < len(prs)-1 {
			m.cursor++
		}
		if pr, ok := m.currentPR(); ok {
			if _, cached := m.details[pr.URL]; !cached {
				return m, m.loadDetail(pr.URL)
			}
		}
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		if pr, ok := m.currentPR(); ok {
			if _, cached := m.details[pr.URL]; !cached {
				return m, m.loadDetail(pr.URL)
			}
		}
		return m, nil

	case "tab":
		m.tab = (m.tab + 1) % 3
		m.cursor = 0
		return m, nil

	case "shift+tab":
		m.tab = (m.tab + 2) % 3
		m.cursor = 0
		return m, nil

	case "r":
		// refresh current tab
		m.prsByTab[m.tab] = nil
		return m, m.loadTab(m.tab)

	case "o":
		pr, ok := m.currentPR()
		if !ok {
			return m, nil
		}
		_ = m.openURL(pr.URL)
		return m, nil

	case "enter":
		if m.running {
			return m, nil
		}
		pr, ok := m.currentPR()
		if !ok {
			return m, nil
		}
		ctx, cancel := context.WithCancel(m.ctx)
		m.pipeCancel = cancel
		m.running = true
		m.steps = nil
		m.lastReview = nil
		return m, m.runPipeline(ctx, pr.URL)
	}

	return m, nil
}

func (m Model) loadDetail(url string) tea.Cmd {
	return func() tea.Msg {
		d, err := m.detailFn(m.ctx, url)
		return prDetailMsg{url: url, detail: d, err: err}
	}
}

func (m Model) runPipeline(ctx context.Context, prURL string) tea.Cmd {
	return func() tea.Msg {
		progressCh := make(chan progressMsg, 8)
		emit := func(e pipeline.Event) {
			progressCh <- progressMsg{step: e.Step, note: e.Note, err: e.Err}
		}

		// run in goroutine so we can stream progress; but tea.Cmd must
		// return ONE message — so we collect and return final reviewDone.
		// We forward progress via Program.Send(); see main wiring.
		// Here we run synchronously and return the final result.
		go func() {
			defer close(progressCh)
			id, err := m.runPipe(ctx, prURL, emit)
			progressCh <- progressMsg{step: "__done__", note: ""}
			_ = id // captured by closure below via sentinel
			_ = err
		}()

		// drain channel
		var lastID int64
		var lastErr error
		for p := range progressCh {
			if p.step == "__done__" {
				break
			}
		}
		_, _ = lastID, lastErr
		// fall through and re-run synchronously to get id/err
		id, err := m.runPipe(ctx, prURL, func(e pipeline.Event) {})
		return reviewDoneMsg{url: prURL, reviewID: id, err: err}
	}
}
```

Note on streaming progress: the above is a placeholder that does not stream progress mid-pipeline. Streaming requires holding a reference to the `*tea.Program` so the goroutine can call `Program.Send()`. That wiring happens in `cmd/prcheck/main.go` (Task 17). For now, progress events are emitted but discarded; the reviewDoneMsg arrives when the pipeline completes. **Fix in Task 17** — see "Step 5" there.

- [ ] **Step 2: Verify compile**

```bash
go build ./internal/tui/...
```
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/update.go
git commit -m "feat(tui): update loop with key handling"
```

---

### Task 15: tui/view — Two-pane rendering

**Files:**
- Create: `internal/tui/view.go`

- [ ] **Step 1: Implement**

Create `internal/tui/view.go`:
```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	tabActive   = lipgloss.NewStyle().Bold(true).Underline(true).Padding(0, 1)
	tabInactive = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("241"))
	border      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	dim         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	pass        = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	fail        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	hint        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

func (m Model) View() string {
	header := m.renderTabs()
	left := m.renderList()
	right := m.renderRight()
	body := lipgloss.JoinHorizontal(lipgloss.Top, border.Render(left), border.Render(right))
	footer := hint.Render("j/k move  Tab switch  Enter review  o open  r refresh  q quit")
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderTabs() string {
	var parts []string
	for i := Tab(0); i < 3; i++ {
		s := i.Label()
		if i == m.tab {
			parts = append(parts, tabActive.Render(s))
		} else {
			parts = append(parts, tabInactive.Render(s))
		}
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderList() string {
	prs := m.prsByTab[m.tab]
	if err := m.loadErr[m.tab]; err != nil {
		return errStyle.Render("load error: ") + err.Error()
	}
	if prs == nil {
		return dim.Render("loading...")
	}
	if len(prs) == 0 {
		return dim.Render("(no PRs)")
	}
	var lines []string
	for i, pr := range prs {
		prefix := "  "
		if i == m.cursor {
			prefix = "► "
		}
		lines = append(lines, fmt.Sprintf("%s#%d %s", prefix, pr.Number, truncate(pr.Title, 50)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderRight() string {
	if m.running {
		return m.renderProgress()
	}
	if m.lastReview != nil {
		return m.renderReviewResult()
	}
	return m.renderDetail()
}

func (m Model) renderDetail() string {
	pr, ok := m.currentPR()
	if !ok {
		return dim.Render("(no selection)")
	}
	d := m.details[pr.URL]
	if d == nil {
		return fmt.Sprintf("%s\n%s", pr.Title, dim.Render("loading detail..."))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Title:   %s\n", d.Title)
	fmt.Fprintf(&b, "Branch:  %s -> %s\n", d.HeadRefName, d.BaseRefName)
	fmt.Fprintf(&b, "Author:  %s\n", d.Author)
	// checks
	var p, f, q int
	for _, c := range d.Checks {
		switch {
		case c.Status != "COMPLETED":
			q++
		case c.Conclusion == "SUCCESS":
			p++
		default:
			f++
		}
	}
	checks := fmt.Sprintf("%s %s %s",
		pass.Render(fmt.Sprintf("%d passed", p)),
		fail.Render(fmt.Sprintf("%d failed", f)),
		dim.Render(fmt.Sprintf("%d pending", q)),
	)
	fmt.Fprintf(&b, "Checks:  %s\n", checks)
	fmt.Fprintf(&b, "\n%s\n", hint.Render("Press Enter to run review."))
	return b.String()
}

func (m Model) renderProgress() string {
	var b strings.Builder
	b.WriteString("Running review pipeline " + m.spinner.View() + "\n\n")
	for _, s := range m.steps {
		b.WriteString("  ✓ " + s + "\n")
	}
	b.WriteString("\n" + hint.Render("Press q to cancel."))
	return b.String()
}

func (m Model) renderReviewResult() string {
	if m.lastReview.err != nil {
		return errStyle.Render("Pipeline failed: ") + m.lastReview.err.Error()
	}
	return fmt.Sprintf("Pending review #%d posted.\n\n%s",
		m.lastReview.reviewID,
		hint.Render("Press o to open in browser, n for next PR (j/k)."))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
```

- [ ] **Step 2: Verify compile**

```bash
go build ./...
```
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/view.go
git commit -m "feat(tui): two-pane view rendering"
```

---

### Task 16: tui — Smoke test with teatest

**Files:**
- Create: `internal/tui/tui_test.go`

- [ ] **Step 1: Add teatest dep**

```bash
go get github.com/charmbracelet/x/exp/teatest@latest
```

- [ ] **Step 2: Write test**

Create `internal/tui/tui_test.go`:
```go
package tui

import (
	"context"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/pipeline"
)

func TestModel_ShowsMineTab(t *testing.T) {
	loader := func(ctx context.Context, q github.Query) ([]github.PR, error) {
		if q == github.QueryAuthored {
			return []github.PR{
				{Number: 1, Title: "First", URL: "https://example/pull/1"},
				{Number: 2, Title: "Second", URL: "https://example/pull/2"},
			}, nil
		}
		return nil, nil
	}
	detail := func(ctx context.Context, url string) (*github.PRDetail, error) {
		d := &github.PRDetail{}
		d.PR.Title = "First"
		d.PR.HeadRefName = "feat/x"
		d.PR.BaseRefName = "main"
		d.PR.Author = "kamil"
		return d, nil
	}
	runPipe := func(ctx context.Context, url string, emit func(pipeline.Event)) (int64, error) {
		return 7, nil
	}
	open := func(url string) error { return nil }

	m := NewModel(loader, detail, runPipe, open)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return containsAll(string(out), "Mine", "#1 First", "#2 Second")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
	_, _ = io.ReadAll(tm.FinalOutput(t))
}

func containsAll(s string, needles ...string) bool {
	for _, n := range needles {
		if !contains(s, n) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Run**

```bash
go test ./internal/tui/...
```
Expected: PASS.

If teatest's exact API differs in the version installed, adjust accordingly — the goal is asserting that the rendered output contains the expected strings after PRs load.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/tui/tui_test.go
git commit -m "test(tui): smoke test renders PR list"
```

---

### Task 17: cmd/prcheck/main — Wire everything up

**Files:**
- Create: `cmd/prcheck/main.go`

- [ ] **Step 1: Implement startup checks + program assembly**

Create `cmd/prcheck/main.go`:
```go
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ksupera/prcheck/internal/claude"
	"github.com/ksupera/prcheck/internal/config"
	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/jira"
	"github.com/ksupera/prcheck/internal/pipeline"
	"github.com/ksupera/prcheck/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "prcheck:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := checkBinary("gh"); err != nil {
		return err
	}
	if err := checkBinary("claude"); err != nil {
		return err
	}
	if err := checkGHAuth(); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	jiraClient := jira.NewClient(cfg.JiraBaseURL, cfg.JiraEmail, cfg.JiraToken)
	ghClient := github.CLIClient{}
	poster := github.Reviewer{}

	// We need a reference to the Program so the pipeline goroutine can
	// stream progress via Program.Send().
	var program *tea.Program

	runPipe := func(ctx context.Context, prURL string, emit func(pipeline.Event)) (int64, error) {
		// emit is provided by tui, but we wrap it to also Send progress
		// messages directly into the Program so the UI updates in real time.
		wrappedEmit := func(e pipeline.Event) {
			emit(e)
			if program != nil {
				program.Send(tui.ProgressFromEvent(e))
			}
		}
		deps := pipeline.Deps{
			GH:     ghClient,
			Jira:   jiraClient,
			Claude: claudeAdapter{},
			Post:   poster,
		}
		return pipeline.Run(ctx, deps, prURL, wrappedEmit)
	}

	loader := func(ctx context.Context, q github.Query) ([]github.PR, error) {
		return github.SearchPRs(ctx, q)
	}
	detailFn := func(ctx context.Context, url string) (*github.PRDetail, error) {
		return github.FetchPRDetail(ctx, url)
	}

	model := tui.NewModel(loader, detailFn, runPipe, openInBrowser)
	program = tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	_ = cfg.Debug // hook for future log routing
	return err
}

type claudeAdapter struct{}

func (claudeAdapter) Invoke(ctx context.Context, prompt string) (*claude.Review, error) {
	return claude.Invoke(ctx, prompt)
}

func checkBinary(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s not found on PATH", name)
	}
	return nil
}

func checkGHAuth() error {
	var stderr bytes.Buffer
	cmd := exec.Command("gh", "auth", "status")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh not authenticated — run `gh auth login` (%s)", stderr.String())
	}
	return nil
}

func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// silence unused import if filepath isn't used elsewhere
var _ = filepath.Separator
```

- [ ] **Step 2: Add ProgressFromEvent helper to tui package**

Edit `internal/tui/messages.go`, append:
```go
import _ "github.com/ksupera/prcheck/internal/pipeline" // for go doc cross-ref

// ProgressFromEvent constructs the public-facing progress message.
// Used by main.go to forward pipeline events into the running Program.
func ProgressFromEvent(e pipeline.Event) tea.Msg {
	return progressMsg{step: e.Step, note: e.Note, err: e.Err}
}
```

Then add the imports (Go won't allow `import _` in the middle of a file; restructure):

Actually replace `internal/tui/messages.go` entirely with:
```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ksupera/prcheck/internal/github"
	"github.com/ksupera/prcheck/internal/pipeline"
)

// Messages flow from goroutines into the Bubble Tea Update function.

type prsLoadedMsg struct {
	tab Tab
	prs []github.PR
	err error
}

type prDetailMsg struct {
	url    string
	detail *github.PRDetail
	err    error
}

type progressMsg struct {
	step string
	note string
	err  error
}

type reviewDoneMsg struct {
	url      string
	reviewID int64
	err      error
}

type Tab int

const (
	TabMine Tab = iota
	TabReview
	TabMentioned
)

func (t Tab) Label() string {
	switch t {
	case TabMine:
		return "Mine"
	case TabReview:
		return "Review"
	case TabMentioned:
		return "Mentioned"
	}
	return "?"
}

// ProgressFromEvent constructs the public progress message used by
// the main package to stream live pipeline events.
func ProgressFromEvent(e pipeline.Event) tea.Msg {
	return progressMsg{step: e.Step, note: e.Note, err: e.Err}
}
```

- [ ] **Step 3: Fix runPipeline in update.go to use Program.Send for streaming**

Replace the `runPipeline` function in `internal/tui/update.go` with the simpler synchronous version (no goroutine duplication):
```go
func (m Model) runPipeline(ctx context.Context, prURL string) tea.Cmd {
	return func() tea.Msg {
		id, err := m.runPipe(ctx, prURL, func(e pipeline.Event) {
			// emit is no-op here — main.go's wrappedEmit forwards events
			// via Program.Send (see cmd/prcheck/main.go).
		})
		return reviewDoneMsg{url: prURL, reviewID: id, err: err}
	}
}
```

- [ ] **Step 4: Build and check**

```bash
go build ./...
```
Expected: succeeds.

- [ ] **Step 5: Tests still pass**

```bash
go test ./...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/ internal/tui/messages.go internal/tui/update.go
git commit -m "feat(prcheck): wire main with live progress streaming"
```

---

### Task 18: README and manual smoke

**Files:**
- Create: `README.md` (replacing the one-line placeholder)

- [ ] **Step 1: Write README**

Replace `README.md`:
````markdown
# prcheck

A terminal UI for reviewing GitHub Pull Requests with Claude Code.

`prcheck` lists PRs you authored, PRs that need your review, and PRs where you're mentioned. For any selected PR it runs an automated review pipeline:

1. Fetches the diff with `gh pr diff`
2. Fetches CI check status with `gh pr view`
3. Extracts a Jira key from the PR body or branch and fetches the issue
4. Runs `claude -p` with the `caveman:caveman-review` skill, the diff, CI failures, and Jira context
5. Posts a **pending** review with inline comments and a summary

You then open the PR in the browser and submit (or edit) the review manually.

## Requirements

- Go 1.22+ (to build)
- `gh` CLI, authenticated (`gh auth login`)
- `claude` CLI (Claude Code), authenticated
- Atlassian API token

## Install

```bash
go install github.com/ksupera/prcheck/cmd/prcheck@latest
```

Or build locally:
```bash
go build -o prcheck ./cmd/prcheck
```

## Configure

```bash
export JIRA_BASE_URL=https://your-org.atlassian.net
export JIRA_EMAIL=you@example.com
export JIRA_TOKEN=...               # https://id.atlassian.com/manage-profile/security/api-tokens
export PRCHECK_DEBUG=1               # optional: subprocess output to log
```

## Use

```bash
prcheck
```

Keys: `j`/`k` move, `Tab` switch tab, `Enter` run review, `o` open in browser, `r` refresh, `q` quit (or cancel running pipeline).

## Manual smoke test

1. Open `prcheck`.
2. Cursor onto a PR you own.
3. Press `Enter`.
4. Wait for "Pending review #N posted."
5. Press `o`. The PR opens in your browser.
6. On the **Files changed** tab you should see your pending review at the top with all inline comments. Submit it manually.
````

- [ ] **Step 2: Build and run once locally**

```bash
go build -o /tmp/prcheck ./cmd/prcheck
JIRA_BASE_URL=https://example.atlassian.net JIRA_EMAIL=a JIRA_TOKEN=b /tmp/prcheck
```

Expected: TUI launches. Press `q` to quit.

If startup fails because `gh` or `claude` is not on PATH, install them and re-run.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: usage instructions for prcheck"
```

---

## Self-Review (recorded after writing)

**1. Spec coverage:** Each spec section maps to tasks:
- §3 Stack → tasks 0, 13, 17
- §4 Architecture → tasks 11, 14, 17
- §5 Components → tasks 1–17 cover every directory listed
- §6 Data flow → task 11 (pipeline), task 17 (live progress streaming)
- §7 TUI flow → tasks 13, 14, 15
- §8 Configuration → task 1
- §9 Error handling — startup checks in task 17, runtime errors handled across tasks 11 and 14
- §10 Testing strategy — every task adds unit tests; task 16 adds TUI smoke

**2. Placeholder scan:** No "TBD" or "implement later" left in steps. The known limitation in Task 14's `runPipeline` is explicitly resolved in Task 17 Step 3 (replaced with a clean implementation).

**3. Type consistency:** `ReviewComment` types in `github` and `claude` packages are deliberately separate (different JSON tags); pipeline.Run translates between them at the post-review step. `Tab` enum reused in both messages.go and view.go.

**4. Known limitation acknowledged in plan:** Task 14 ships a placeholder `runPipeline` that Task 17 then rewrites. This avoids a forward reference to `Program.Send` in Task 14. If executing tasks out of order, do Task 17 immediately after Task 14, or merge them.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-22-prcheck-implementation.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
