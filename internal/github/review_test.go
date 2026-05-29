package github

import (
	"encoding/json"
	"os"
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
	// Write a custom fake-gh that captures stdin and echoes a canned response.
	dir := t.TempDir()
	out := dir + "/captured.txt"
	script := `#!/bin/sh
cat > "` + out + `"
echo '{"id": 999, "state": "PENDING"}'
`
	if err := os.WriteFile(dir+"/gh", []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	id, err := PostPendingReview(t.Context(), "https://github.com/org/repo/pull/1234",
		"summary", []ReviewComment{{Path: "f.go", Line: 1, Side: "RIGHT", Body: "c", Severity: "nit"}})
	if err != nil {
		t.Fatalf("PostPendingReview err: %v", err)
	}
	if id != 999 {
		t.Errorf("review id = %d", id)
	}

	// Verify the captured stdin contains the expected body.
	captured, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read captured: %v", err)
	}
	if !strings.Contains(string(captured), `"body":"summary"`) {
		t.Errorf("captured body missing summary: %s", captured)
	}
}
