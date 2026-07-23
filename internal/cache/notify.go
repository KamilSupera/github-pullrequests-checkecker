package cache

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/github"
)

// Change describes one PR that differs from the previous snapshot.
// IsNew is true when the URL was absent from the old snapshot;
// otherwise the URL existed and its UpdatedAt changed.
type Change struct {
	PR    github.PR
	IsNew bool
}

// DetectChanges compares an old snapshot to a fresh PR list and returns
// the PRs that are new or freshly updated. Old==nil returns no changes
// (the very first launch shouldn't burst notifications).
func DetectChanges(old *Snapshot, fresh []github.PR) []Change {
	if old == nil {
		return nil
	}
	byURL := map[string]string{}
	for _, pr := range old.PRs {
		byURL[pr.URL] = pr.UpdatedAt
	}
	var out []Change
	for _, pr := range fresh {
		prev, seen := byURL[pr.URL]
		switch {
		case !seen:
			out = append(out, Change{PR: pr, IsNew: true})
		case pr.UpdatedAt != prev:
			out = append(out, Change{PR: pr, IsNew: false})
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
