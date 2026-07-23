package config

import (
	"os"
	"path/filepath"
	"testing"
)

// isolateConfigDir points os.UserConfigDir() at a fresh temp directory so
// the test never reads the developer's real ~/.config/prcheck/config.json.
// It sets both HOME (macOS) and XDG_CONFIG_HOME (Linux) to cover either
// platform's UserConfigDir resolution.
func isolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	// Env vars must not leak in from the developer's shell.
	t.Setenv("PRCHECK_THEME", "")
}

// writeConfig writes raw bytes to the resolved config path, creating the
// parent directory. Empty content means "no file".
func writeConfig(t *testing.T, content string) {
	t.Helper()
	if content == "" {
		return
	}
	path := configPath()
	if path == "" {
		t.Fatal("configPath() returned empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestLoad_ThemeDefaultsAuto(t *testing.T) {
	isolateConfigDir(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.Theme != "auto" {
		t.Errorf("Theme = %q, want default \"auto\"", cfg.Theme)
	}
}

func TestLoad_ThemeFromFile(t *testing.T) {
	isolateConfigDir(t)
	writeConfig(t, `{"theme": "light"}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.Theme != "light" {
		t.Errorf("Theme = %q, want \"light\" from file", cfg.Theme)
	}
}

func TestLoad_ThemeEnvOverridesFile(t *testing.T) {
	isolateConfigDir(t)
	writeConfig(t, `{"theme": "light"}`)
	t.Setenv("PRCHECK_THEME", "dark")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.Theme != "dark" {
		t.Errorf("Theme = %q, want env value \"dark\" to win over file", cfg.Theme)
	}
}

func TestLoad_ThemeUnknownFallsBackAuto(t *testing.T) {
	isolateConfigDir(t)
	t.Setenv("PRCHECK_THEME", "psychedelic")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.Theme != "auto" {
		t.Errorf("Theme = %q, want \"auto\" for unknown value", cfg.Theme)
	}
}

func TestLoad_MalformedFileIsNonFatal(t *testing.T) {
	isolateConfigDir(t)
	writeConfig(t, `{ this is not valid json `)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should not fail on malformed file: %v", err)
	}
	if cfg.Theme != "auto" {
		t.Errorf("Theme = %q, want \"auto\" when file is malformed", cfg.Theme)
	}
}

func TestNormalizeTheme(t *testing.T) {
	cases := map[string]string{
		"dark":     "dark",
		"DARK":     "dark",
		"  light":  "light",
		"Light ":   "light",
		"auto":     "auto",
		"":         "auto",
		"nonsense": "auto",
	}
	for in, want := range cases {
		if got := normalizeTheme(in); got != want {
			t.Errorf("normalizeTheme(%q) = %q, want %q", in, got, want)
		}
	}
}
