package claude

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeFakeClaudeForJira(t *testing.T, body string) {
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
}

func TestJiraMCPFetcher_Success(t *testing.T) {
	writeFakeClaudeForJira(t, `Here you go: {"key":"ABC-123","summary":"Add cache","description":"TTL cache please"}`)

	iss, err := JiraMCPFetcher{}.FetchIssue(t.Context(), "ABC-123")
	if err != nil {
		t.Fatalf("FetchIssue err: %v", err)
	}
	if iss.Key != "ABC-123" {
		t.Errorf("Key = %q", iss.Key)
	}
	if iss.Summary != "Add cache" {
		t.Errorf("Summary = %q", iss.Summary)
	}
	if iss.Description != "TTL cache please" {
		t.Errorf("Description = %q", iss.Description)
	}
}

func TestJiraMCPFetcher_NoJSON(t *testing.T) {
	writeFakeClaudeForJira(t, "I could not find the issue, sorry.")

	_, err := JiraMCPFetcher{}.FetchIssue(t.Context(), "ABC-1")
	if err == nil {
		t.Fatal("expected error for missing JSON")
	}
}

func TestJiraMCPFetcher_MissingKey(t *testing.T) {
	writeFakeClaudeForJira(t, `{"summary":"x","description":""}`)

	_, err := JiraMCPFetcher{}.FetchIssue(t.Context(), "ABC-1")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}
