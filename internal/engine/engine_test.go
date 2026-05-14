package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lihua8552-afk/pg-migration-guard/internal/config"
)

func TestRunStrictPathsRejectsMissingPath(t *testing.T) {
	dir := t.TempDir()
	migration := filepath.Join(dir, "001.sql")
	missing := filepath.Join(dir, "missing")
	if err := os.WriteFile(migration, []byte("CREATE INDEX CONCURRENTLY idx_users_email ON users (email);"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{
		Paths:       []string{missing, migration},
		StrictPaths: true,
		Config:      config.Default(),
	})
	if err == nil {
		t.Fatalf("expected missing path error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConfigDefaultPathsSkipMissingAlternates(t *testing.T) {
	dir := t.TempDir()
	migrations := filepath.Join(dir, "migrations")
	if err := os.Mkdir(migrations, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(migrations, "001.sql"), []byte("CREATE INDEX CONCURRENTLY idx_users_email ON users (email);"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Migrations.Paths = []string{migrations, filepath.Join(dir, "db", "migrate")}
	result, err := Run(context.Background(), Options{Config: cfg})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("unexpected findings: %#v", result.Findings)
	}
}
