package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, found, err := Load(filepath.Join(t.TempDir(), "mguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("missing config should not be reported as found")
	}
	if cfg.Database.Dialect != "postgres" || cfg.Risk.FailOn != "high" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestLoadFillsPartialConfigDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mguard.yaml")
	if err := os.WriteFile(path, []byte("ai:\n  enabled: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, found, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("expected config to be found")
	}
	if cfg.Database.Dialect != "postgres" {
		t.Fatalf("dialect = %q", cfg.Database.Dialect)
	}
	if cfg.AI.Provider != "openai" || cfg.AI.APIKeyEnv != "MGUARD_AI_KEY" {
		t.Fatalf("AI defaults not filled: %#v", cfg.AI)
	}
	if len(cfg.Migrations.Paths) == 0 {
		t.Fatalf("migration paths were not defaulted")
	}
}
