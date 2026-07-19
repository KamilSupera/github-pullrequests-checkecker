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

func TestSelectModel(t *testing.T) {
	t.Cleanup(func() { _ = SelectAgent("claude"); SelectModel("default") })

	SelectModel("opus")
	if ModelName() != "opus" {
		t.Errorf("ModelName() = %q, want opus", ModelName())
	}
	args := agentCommand(t.Context(), "p").Args
	if args[len(args)-2] != "--model" || args[len(args)-1] != "opus" {
		t.Errorf("claude args missing --model opus: %v", args)
	}

	SelectModel("default")
	for _, a := range agentCommand(t.Context(), "p").Args {
		if a == "--model" {
			t.Errorf("default model must not pass --model: %v", args)
		}
	}

	// Switching agents resets the model — the lists differ.
	SelectModel("opus")
	_ = SelectAgent("cursor")
	if ModelName() != "default" {
		t.Errorf("model after agent switch = %q, want default", ModelName())
	}
	SelectModel("gpt-5")
	args = agentCommand(t.Context(), "p").Args
	// cursor keeps the prompt positional and last.
	if args[len(args)-1] != "p" || args[len(args)-3] != "--model" || args[len(args)-2] != "gpt-5" {
		t.Errorf("cursor args wrong: %v", args)
	}
}

func TestResolveDefaultModel(t *testing.T) {
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("HOME", t.TempDir()) // hide the real ~/.claude/settings.json
	t.Chdir(t.TempDir())

	if got := resolveDefaultModel(); got != "" {
		t.Errorf("no config: got %q, want empty", got)
	}

	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".claude", "settings.json"), []byte(`{"model":"opusplan"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveDefaultModel(); got != "opusplan" {
		t.Errorf("project settings: got %q, want opusplan", got)
	}

	t.Setenv("ANTHROPIC_MODEL", "claude-opus-4-8")
	if got := resolveDefaultModel(); got != "claude-opus-4-8" {
		t.Errorf("env should win: got %q", got)
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
