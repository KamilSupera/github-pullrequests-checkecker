package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// BookmarkStore is a persisted set of starred PR URLs.
type BookmarkStore struct {
	URLs map[string]bool `json:"urls"`
}

func bookmarksPath() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bookmarks.json"), nil
}

func LoadBookmarks() *BookmarkStore {
	s := &BookmarkStore{URLs: map[string]bool{}}
	p, err := bookmarksPath()
	if err != nil {
		return s
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, s)
	if s.URLs == nil {
		s.URLs = map[string]bool{}
	}
	return s
}

func (s *BookmarkStore) Toggle(url string) bool {
	if s.URLs[url] {
		delete(s.URLs, url)
		return false
	}
	s.URLs[url] = true
	return true
}

func (s *BookmarkStore) Has(url string) bool {
	return s.URLs[url]
}

func (s *BookmarkStore) Save() {
	p, err := bookmarksPath()
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o600)
}
