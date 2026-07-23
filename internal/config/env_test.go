package config

import "testing"

func TestLoad_Default(t *testing.T) {
	t.Setenv("PRCHECK_DEBUG", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.Debug {
		t.Error("Debug should default to false")
	}
}

func TestLoad_DebugFlag(t *testing.T) {
	t.Setenv("PRCHECK_DEBUG", "1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if !cfg.Debug {
		t.Error("Debug should be true when PRCHECK_DEBUG=1")
	}
}

func TestLoad_WatchDefault(t *testing.T) {
	t.Setenv("PRCHECK_WATCH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.WatchMinutes != 5 {
		t.Errorf("WatchMinutes = %d, want default 5", cfg.WatchMinutes)
	}
}

func TestLoad_WatchCustom(t *testing.T) {
	t.Setenv("PRCHECK_WATCH", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.WatchMinutes != 10 {
		t.Errorf("WatchMinutes = %d, want 10", cfg.WatchMinutes)
	}
}

func TestLoad_WatchClampsBelowOne(t *testing.T) {
	t.Setenv("PRCHECK_WATCH", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if cfg.WatchMinutes != 5 {
		t.Errorf("WatchMinutes = %d, want clamp to 5", cfg.WatchMinutes)
	}
}
