package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// agentKind is the backend CLI prcheck shells out to for model calls.
type agentKind int

const (
	agentClaude agentKind = iota
	agentCursor
)

var (
	agentMu       sync.RWMutex
	selected      = agentClaude
	selectedModel string // "" = agent CLI's default model
)

// SelectAgent sets the backend CLI by name. Empty or "claude" selects the
// Claude CLI (default); "cursor" or "cursor-agent" selects Cursor's CLI.
// Unknown names return an error.
func SelectAgent(name string) error {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "claude":
		setAgent(agentClaude)
	case "cursor", "cursor-agent":
		setAgent(agentCursor)
	default:
		return fmt.Errorf("unknown agent %q (want \"claude\" or \"cursor\")", name)
	}
	return nil
}

func setAgent(k agentKind) {
	agentMu.Lock()
	if k != selected {
		selectedModel = "" // model lists differ per agent
	}
	selected = k
	agentMu.Unlock()
}

// SelectModel sets the model passed to the agent CLI via --model.
// "default" or "" clears the override so the CLI picks its own.
func SelectModel(name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "default" {
		name = ""
	}
	agentMu.Lock()
	selectedModel = name
	agentMu.Unlock()
}

func currentModel() string {
	agentMu.RLock()
	defer agentMu.RUnlock()
	return selectedModel
}

// ModelName returns a short human label for the selected model.
func ModelName() string {
	if m := currentModel(); m != "" {
		return m
	}
	return "default"
}

// Models lists the selectable model names for the current agent, in
// display order. The first entry leaves the choice to the CLI. Claude
// entries are full model IDs so the picker shows the exact version.
func Models() []string {
	if currentAgent() == agentCursor {
		return []string{"default", "sonnet-4.5", "sonnet-4.5-thinking", "opus-4.1", "gpt-5"}
	}
	return []string{"default", "claude-opus-4-8", "claude-sonnet-5", "claude-haiku-4-5"}
}

var (
	defaultModelOnce sync.Once
	defaultModelVal  string
)

// DefaultModel reports which model the claude CLI uses when no --model
// flag is passed, resolved from the same sources the CLI reads. Empty
// when undetectable (cursor agent, or nothing configured — the CLI then
// falls back to its account default).
func DefaultModel() string {
	if currentAgent() == agentCursor {
		return ""
	}
	defaultModelOnce.Do(func() { defaultModelVal = resolveDefaultModel() })
	return defaultModelVal
}

// resolveDefaultModel mirrors the claude CLI's model resolution order:
// ANTHROPIC_MODEL env, project .claude/settings, user ~/.claude/settings.
// ponytail: config files only; shell out to `claude config get model` if
// this ever misses a source.
func resolveDefaultModel() string {
	if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
		return m
	}
	paths := []string{
		filepath.Join(".claude", "settings.local.json"),
		filepath.Join(".claude", "settings.json"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".claude", "settings.json"))
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(raw, &s) == nil && s.Model != "" {
			return s.Model
		}
	}
	return ""
}

// ModelDesc returns a short human description for a model option, or ""
// when there is nothing useful to add.
func ModelDesc(name string) string {
	switch name {
	case "default":
		if m := DefaultModel(); m != "" {
			return "CLI default: " + m
		}
		return "whatever the " + AgentBinary() + " CLI is configured to use"
	case "claude-opus-4-8":
		return "Opus 4.8 — most capable"
	case "claude-sonnet-5":
		return "Sonnet 5 — balanced speed/quality"
	case "claude-haiku-4-5":
		return "Haiku 4.5 — fastest, cheapest"
	}
	return ""
}

func currentAgent() agentKind {
	agentMu.RLock()
	defer agentMu.RUnlock()
	return selected
}

// AgentBinary returns the executable name for the selected agent, for
// PATH validation at startup.
func AgentBinary() string {
	if currentAgent() == agentCursor {
		return "cursor-agent"
	}
	return "claude"
}

// AgentName returns a short human label for the selected agent.
func AgentName() string {
	if currentAgent() == agentCursor {
		return "cursor"
	}
	return "claude"
}

// AgentRecordsUsage reports whether the selected agent's JSON output
// includes token/cost data. Cursor does not.
func AgentRecordsUsage() bool {
	return currentAgent() == agentClaude
}

// Agents lists the selectable agent names, in display order.
func Agents() []string { return []string{"claude", "cursor"} }

// BinaryForAgent returns the executable name a given agent name resolves
// to (for PATH checks). Unknown names fall back to the claude binary.
func BinaryForAgent(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "cursor", "cursor-agent":
		return "cursor-agent"
	default:
		return "claude"
	}
}

// agentCommand builds the exec command for the selected agent, requesting
// JSON output so the result (and, for claude, usage) can be parsed.
func agentCommand(ctx context.Context, prompt string) *exec.Cmd {
	model := currentModel()
	if currentAgent() == agentCursor {
		// cursor-agent: -p/--print is boolean, prompt is positional,
		// -f forces headless execution (no tool-approval prompts).
		args := []string{"-p", "--output-format", "json", "-f"}
		if model != "" {
			args = append(args, "--model", model)
		}
		return exec.CommandContext(ctx, "cursor-agent", append(args, prompt)...)
	}
	args := []string{"-p", prompt, "--output-format", "json"}
	if model != "" {
		args = append(args, "--model", model)
	}
	return exec.CommandContext(ctx, "claude", args...)
}
