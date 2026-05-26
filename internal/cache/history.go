package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// HistoryEntry records one completed pipeline run.
type HistoryEntry struct {
	When     time.Time `json:"when"`
	PRURL    string    `json:"pr_url"`
	ReviewID int64     `json:"review_id"`
	Summary  string    `json:"summary"`
	Comments int       `json:"comments"`
}

func historyPath() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

// AppendHistory writes one JSON-Lines entry to the history file.
// Best-effort: errors are swallowed.
func AppendHistory(e HistoryEntry) {
	p, err := historyPath()
	if err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	if data, err := json.Marshal(e); err == nil {
		_, _ = f.Write(append(data, '\n'))
	}
}

// LoadHistory returns every entry written so far (most recent last).
// Limit of 0 means no cap.
func LoadHistory(limit int) []HistoryEntry {
	p, err := historyPath()
	if err != nil {
		return nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []HistoryEntry
	dec := json.NewDecoder(f)
	for dec.More() {
		var e HistoryEntry
		if err := dec.Decode(&e); err != nil {
			break
		}
		out = append(out, e)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}
