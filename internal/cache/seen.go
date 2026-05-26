package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SeenStore tracks the last time the user pressed Space (loaded detail)
// on each PR URL. Used to highlight PRs with newer activity in the
// list view.
type SeenStore struct {
	URLs map[string]time.Time `json:"urls"`
}

func seenPath() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "seen.json"), nil
}

// LoadSeen returns the persisted seen map (never nil).
func LoadSeen() *SeenStore {
	s := &SeenStore{URLs: map[string]time.Time{}}
	p, err := seenPath()
	if err != nil {
		return s
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, s)
	if s.URLs == nil {
		s.URLs = map[string]time.Time{}
	}
	return s
}

// Mark records that the user viewed the PR at the given URL right now.
func (s *SeenStore) Mark(url string) {
	s.URLs[url] = time.Now()
}

// IsNew reports whether the PR's updatedAt is newer than the last
// recorded view (or the user has never opened it).
func (s *SeenStore) IsNew(url string, updatedAt time.Time) bool {
	t, ok := s.URLs[url]
	if !ok {
		return true
	}
	return updatedAt.After(t)
}

// Save persists the store. Best-effort.
func (s *SeenStore) Save() {
	p, err := seenPath()
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o600)
}
