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
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "..." + string(r[len(r)-n:])
}
