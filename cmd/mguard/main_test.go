package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckMarkdownFailsOnHigh(t *testing.T) {
	dir := t.TempDir()
	migration := filepath.Join(dir, "001.sql")
	if err := os.WriteFile(migration, []byte("DROP TABLE users;"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"check", migration, "-format", "markdown", "-fail-on", "high"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "MGD020") {
		t.Fatalf("missing finding in stdout:\n%s", stdout.String())
	}
}

func TestCheckTextSucceedsBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	migration := filepath.Join(dir, "001.sql")
	if err := os.WriteFile(migration, []byte("CREATE INDEX CONCURRENTLY idx_users_email ON users (email);"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"check", "-format", "text", "-fail-on", "high", migration}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
}

func TestCheckInvalidSQLFails(t *testing.T) {
	dir := t.TempDir()
	migration := filepath.Join(dir, "bad.sql")
	if err := os.WriteFile(migration, []byte("CREAT INDEX idx_bad ON users (email);"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"check", "-format", "text", "-fail-on", "high", migration}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "MGD000") {
		t.Fatalf("missing parse error finding in stdout:\n%s", stdout.String())
	}
}

func TestCheckBoolFlagDoesNotConsumePath(t *testing.T) {
	dir := t.TempDir()
	migration := filepath.Join(dir, "001.sql")
	if err := os.WriteFile(migration, []byte("CREATE INDEX CONCURRENTLY idx_users_email ON users (email);"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"check", "-ai-redact", "-format", "json", migration}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
}

func TestCheckRejectsUnsupportedDialect(t *testing.T) {
	dir := t.TempDir()
	migration := filepath.Join(dir, "001.sql")
	configPath := filepath.Join(dir, "mguard.yaml")
	if err := os.WriteFile(migration, []byte("CREATE INDEX CONCURRENTLY idx_users_email ON users (email);"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("database:\n  dialect: mysql\nmigrations:\n  paths:\n    - "+migration+"\nrisk:\n  fail_on: high\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"check", "-config", configPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "unsupported database dialect") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCheckFailsWhenExplicitDSNConnectionFails(t *testing.T) {
	dir := t.TempDir()
	migration := filepath.Join(dir, "001.sql")
	if err := os.WriteFile(migration, []byte("CREATE INDEX CONCURRENTLY idx_users_email ON users (email);"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MGUARD_BAD_DSN", "postgres://bad:bad@127.0.0.1:9/nope?sslmode=disable")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"check", "-dsn-env", "MGUARD_BAD_DSN", migration}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "metadata connection failed") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCheckFailsWhenExplicitPathIsMissing(t *testing.T) {
	dir := t.TempDir()
	migration := filepath.Join(dir, "001.sql")
	missing := filepath.Join(dir, "missing")
	if err := os.WriteFile(migration, []byte("CREATE INDEX CONCURRENTLY idx_users_email ON users (email);"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"check", missing, migration}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, stderr = %s, stdout = %s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "does not exist") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestInitWritesConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mguard.yaml")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"init", "-config", configPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "dialect: postgres") {
		t.Fatalf("unexpected config:\n%s", data)
	}
}
