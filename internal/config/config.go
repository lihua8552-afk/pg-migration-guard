package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database   DatabaseConfig   `yaml:"database"`
	Migrations MigrationsConfig `yaml:"migrations"`
	Risk       RiskConfig       `yaml:"risk"`
	AI         AIConfig         `yaml:"ai"`
}

type DatabaseConfig struct {
	Dialect string `yaml:"dialect"`
	DSNEnv  string `yaml:"dsn_env"`
}

type MigrationsConfig struct {
	Paths []string `yaml:"paths"`
}

type RiskConfig struct {
	FailOn string `yaml:"fail_on"`
}

type AIConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"`
	BaseURL   string `yaml:"base_url"`
	RedactSQL bool   `yaml:"redact_sql"`
}

func Default() Config {
	return Config{
		Database: DatabaseConfig{
			Dialect: "postgres",
			DSNEnv:  "DATABASE_URL",
		},
		Migrations: MigrationsConfig{
			Paths: []string{"migrations", "db/migrate"},
		},
		Risk: RiskConfig{
			FailOn: "high",
		},
		AI: AIConfig{
			Enabled:   false,
			Provider:  "openai",
			Model:     "",
			APIKeyEnv: "MGUARD_AI_KEY",
		},
	}
}

func Load(path string) (Config, bool, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, false, err
	}
	if cfg.Database.Dialect == "" {
		cfg.Database.Dialect = "postgres"
	}
	if cfg.Database.DSNEnv == "" {
		cfg.Database.DSNEnv = "DATABASE_URL"
	}
	if len(cfg.Migrations.Paths) == 0 {
		cfg.Migrations.Paths = Default().Migrations.Paths
	}
	if cfg.Risk.FailOn == "" {
		cfg.Risk.FailOn = "high"
	}
	if cfg.AI.Provider == "" {
		cfg.AI.Provider = "openai"
	}
	if cfg.AI.APIKeyEnv == "" {
		cfg.AI.APIKeyEnv = "MGUARD_AI_KEY"
	}
	return cfg, true, nil
}

func DefaultYAML() []byte {
	return []byte(`database:
  dialect: postgres
  dsn_env: DATABASE_URL

migrations:
  paths:
    - migrations
    - db/migrate

risk:
  fail_on: high

ai:
  enabled: false
  provider: openai
  model: ""
  api_key_env: MGUARD_AI_KEY
  base_url: ""
  redact_sql: false
`)
}
