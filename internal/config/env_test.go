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
