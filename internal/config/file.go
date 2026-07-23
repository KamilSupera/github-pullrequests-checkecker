package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// fileConfig mirrors the on-disk JSON config file. Every field is a
// pointer so an omitted key stays distinguishable from an explicit zero
// value and never clobbers a setting supplied via the environment. Only
// Theme is wired up today; other fields can be added here as the
// PRCHECK_* env vars migrate into the file.
type fileConfig struct {
	Theme *string `json:"theme"`
}

// configPath returns the path to prcheck's JSON config file, or "" when
// the user config dir cannot be determined. It uses os.UserConfigDir()
// — the config-dir analog of the cache dir used elsewhere — which is
// ~/.config/prcheck on Linux and ~/Library/Application Support/prcheck on
// macOS.
func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "prcheck", "config.json")
}

// loadFile reads and decodes the JSON config file. A missing file is not
// an error — it returns a zero fileConfig. Malformed JSON returns an
// error so the caller can warn the user; callers must treat it as
// non-fatal and fall back to defaults + environment.
func loadFile() (fileConfig, error) {
	var fc fileConfig
	path := configPath()
	if path == "" {
		return fc, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fc, nil
		}
		return fc, err
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		return fc, err
	}
	return fc, nil
}
