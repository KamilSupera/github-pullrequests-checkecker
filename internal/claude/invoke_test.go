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

// Full issue #12 scenario through the exec + envelope path: the CLI
// envelope's result text has prose with a brace snippet before the JSON.
func TestInvoke_EnvelopeProseBraces(t *testing.T) {
	body := `{"type":"result","is_error":false,"result":"The diff changes ` +
		"`Result{err}`" + ` handling.\n\n{\"summary\":\"ok\",\"comments\":[]}"}`
	writeFakeClaude(t, body)

	r, err := Invoke(t.Context(), "p")
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
	if !strings.Contains(err.Error(), "no parseable review JSON") {
		t.Errorf("unexpected error: %v", err)
	}
}
