// Package cache persists PR list snapshots between launches so the
// first frame can show cached data while a background refresh runs.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/KamilSupera/github-pullrequests-checkecker/internal/github"
)

// Snapshot is one tab's cached state.
type Snapshot struct {
	FetchedAt time.Time   `json:"fetched_at"`
	PRs       []github.PR `json:"prs"`
}

// path returns the on-disk path for a tab's snapshot.
func path(tabKey string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, tabKey+".json"), nil
}

func cacheDir() (string, error) {
	if d := os.Getenv("PRCHECK_CACHE_DIR"); d != "" {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", err
		}
		return d, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = filepath.Join(os.TempDir(), "prcheck-cache")
	}
	dir := filepath.Join(base, "prcheck")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Load returns the cached snapshot for the given tab, or nil if none.
func Load(tabKey string) *Snapshot {
	p, err := path(tabKey)
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil
	}
	return &s
}

// Save writes a snapshot to disk. Best-effort: errors are swallowed.
func Save(tabKey string, prs []github.PR) {
	p, err := path(tabKey)
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(Snapshot{FetchedAt: time.Now(), PRs: prs}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o600)
}
