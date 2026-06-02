package claude

import (
	"context"
	"fmt"
	"os/exec"
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
	agentMu  sync.RWMutex
	selected = agentClaude
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
	selected = k
	agentMu.Unlock()
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
	if currentAgent() == agentCursor {
		// cursor-agent: -p/--print is boolean, prompt is positional,
		// -f forces headless execution (no tool-approval prompts).
		return exec.CommandContext(ctx, "cursor-agent", "-p", "--output-format", "json", "-f", prompt)
	}
	return exec.CommandContext(ctx, "claude", "-p", prompt, "--output-format", "json")
}
