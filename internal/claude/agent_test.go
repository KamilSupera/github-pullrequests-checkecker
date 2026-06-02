package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectAgent(t *testing.T) {
	t.Cleanup(func() { _ = SelectAgent("claude") })

	cases := map[string]struct {
		bin     string
		records bool
		name    string
	}{
		"":             {"claude", true, "claude"},
		"claude":       {"claude", true, "claude"},
		"cursor":       {"cursor-agent", false, "cursor"},
		"cursor-agent": {"cursor-agent", false, "cursor"},
		"CURSOR":       {"cursor-agent", false, "cursor"},
	}
	for name, want := range cases {
		if err := SelectAgent(name); err != nil {
			t.Fatalf("SelectAgent(%q) err: %v", name, err)
		}
		if AgentBinary() != want.bin {
			t.Errorf("SelectAgent(%q): binary = %q, want %q", name, AgentBinary(), want.bin)
		}
		if AgentRecordsUsage() != want.records {
			t.Errorf("SelectAgent(%q): recordsUsage = %v, want %v", name, AgentRecordsUsage(), want.records)
		}
		if AgentName() != want.name {
			t.Errorf("SelectAgent(%q): name = %q, want %q", name, AgentName(), want.name)
		}
	}

	if err := SelectAgent("gpt"); err == nil {
		t.Error("SelectAgent(\"gpt\") should error")
	}
}

// writeFakeBin drops an executable on PATH that echoes body, under an
// arbitrary name (so we can stub cursor-agent as well as claude).
func writeFakeBin(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncat <<'EOF'\n" + body + "\nEOF\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestInvoke_CursorAgent(t *testing.T) {
	resetUsage()
	if err := SelectAgent("cursor"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SelectAgent("claude"); resetUsage() })

	// Cursor envelope: has type+result, no usage/cost.
	env := `{"type":"result","subtype":"success","is_error":false,` +
		`"result":"{\"summary\":\"ok\",\"comments\":[]}","session_id":"s1","request_id":"r1"}`
	writeFakeBin(t, "cursor-agent", env)

	r, err := Invoke(t.Context(), "p")
	if err != nil {
		t.Fatalf("Invoke (cursor) err: %v", err)
	}
	if r.Summary != "ok" {
		t.Errorf("Summary = %q, want ok", r.Summary)
	}
	u := SessionUsage()
	if u.Calls != 1 {
		t.Errorf("calls = %d, want 1", u.Calls)
	}
	if u.InputTokens != 0 || u.OutputTokens != 0 || u.CostUSD != 0 {
		t.Errorf("cursor should report no token usage, got %+v", u)
	}
	if AgentRecordsUsage() {
		t.Error("AgentRecordsUsage() should be false for cursor")
	}
}
