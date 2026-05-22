package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPrompt_Full(t *testing.T) {
	in := PromptInput{
		Title:         "Add caching",
		HeadRef:       "feat/cache",
		BaseRef:       "develop",
		Author:        "kamil",
		JiraKey:       "ABC-123",
		JiraSummary:   "Add caching",
		JiraDesc:      "We need a TTL cache.",
		FailedChecks:  []string{"test"},
		PendingChecks: []string{},
		Diff:          "diff --git a/foo.go b/foo.go\n+func cached() {}\n",
		DiffTruncated: false,
	}
	got := BuildPrompt(in)
	goldenPath := filepath.Join("..", "..", "testdata", "prompt", "full.golden.txt")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		_ = os.MkdirAll(filepath.Dir(goldenPath), 0o755)
		_ = os.WriteFile(goldenPath, []byte(got), 0o644)
		return
	}
	golden, err := os.ReadFile(goldenPath)
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
	goldenPath := filepath.Join("..", "..", "testdata", "prompt", "no-jira.golden.txt")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		_ = os.MkdirAll(filepath.Dir(goldenPath), 0o755)
		_ = os.WriteFile(goldenPath, []byte(got), 0o644)
		return
	}
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(golden) {
		t.Errorf("prompt mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}
