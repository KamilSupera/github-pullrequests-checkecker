package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

func Invoke(ctx context.Context, prompt string) (*Review, error) {
	out, err := runAgent(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return ParseReview([]byte(out))
}

// cliEnvelope is the JSON wrapper `claude --output-format json` emits. The
// fields we care about are the result text, the error flag, and usage.
type cliEnvelope struct {
	Type      string  `json:"type"`
	IsError   bool    `json:"is_error"`
	Result    string  `json:"result"`
	TotalCost float64 `json:"total_cost_usd"`
	Usage     struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// runAgent runs the selected agent CLI and returns its result text. It
// requests JSON output so token usage can be recorded into the session
// total. When stdout is a real result envelope its usage is folded in;
// otherwise (a stubbed plain-text response in tests or the demo) the raw
// stdout is returned unchanged and no usage is recorded.
func runAgent(ctx context.Context, prompt string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := agentCommand(ctx, prompt)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s exec: %w (stderr: %s)", AgentBinary(), err, tail(stderr.String(), 500))
	}

	// Both claude and cursor envelopes set "type"; bare stub/legacy JSON
	// does not, so it falls through to the raw-stdout path below.
	var env cliEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err == nil && env.Type != "" {
		addUsage(Usage{
			InputTokens:         env.Usage.InputTokens,
			OutputTokens:        env.Usage.OutputTokens,
			CacheCreationTokens: env.Usage.CacheCreationInputTokens,
			CacheReadTokens:     env.Usage.CacheReadInputTokens,
			CostUSD:             env.TotalCost,
		})
		if env.IsError {
			return "", fmt.Errorf("%s reported error: %s", AgentName(), tail(env.Result, 500))
		}
		return env.Result, nil
	}

	// Not an envelope — treat stdout as the result directly.
	return stdout.String(), nil
}

func tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "..." + string(r[len(r)-n:])
}
