package cache

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/ksupera/prcheck/internal/github"
)

// DetectChanges compares an old snapshot to a fresh PR list and
// returns URLs that look new or freshly updated. Old==nil returns
// no changes (the very first launch shouldn't burst notifications).
func DetectChanges(old *Snapshot, fresh []github.PR) []string {
	if old == nil {
		return nil
	}
	byURL := map[string]string{}
	for _, pr := range old.PRs {
		byURL[pr.URL] = pr.UpdatedAt
	}
	var out []string
	for _, pr := range fresh {
		prev, seen := byURL[pr.URL]
		if !seen || pr.UpdatedAt != prev {
			out = append(out, pr.URL)
		}
	}
	return out
}

// Notify shows a desktop notification using the platform-native
// command. Best-effort; errors are silently ignored.
func Notify(title, body string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		cmd = exec.Command("osascript", "-e", script)
	case "linux":
		cmd = exec.Command("notify-send", title, body)
	default:
		return
	}
	_ = cmd.Start()
}
