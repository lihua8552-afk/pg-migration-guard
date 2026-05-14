package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lihua8552-afk/mguard/internal/ai"
	"github.com/lihua8552-afk/mguard/internal/config"
	"github.com/lihua8552-afk/mguard/internal/introspect"
	"github.com/lihua8552-afk/mguard/internal/model"
	"github.com/lihua8552-afk/mguard/internal/parser"
	"github.com/lihua8552-afk/mguard/internal/rules"
)

type Options struct {
	Paths       []string
	StrictPaths bool
	DSNEnv      string
	AIOverride  string
	AIRedactSQL *bool
	Format      string
	Config      config.Config
}

func Run(ctx context.Context, opts Options) (model.Result, error) {
	cfg := opts.Config
	if strings.ToLower(strings.TrimSpace(cfg.Database.Dialect)) != "postgres" {
		return model.Result{}, fmt.Errorf("unsupported database dialect %q; mguard v1 only supports postgres", cfg.Database.Dialect)
	}
	paths := opts.Paths
	if len(paths) == 0 {
		paths = cfg.Migrations.Paths
	}
	files, err := collectSQLFiles(paths, opts.StrictPaths)
	if err != nil {
		return model.Result{}, err
	}
	if len(files) == 0 {
		return model.Result{}, fmt.Errorf("no .sql migration files found in %s", strings.Join(paths, ", "))
	}

	result := model.Result{Tool: "mguard", GeneratedAt: time.Now().UTC()}
	var analyses []model.FileAnalysis
	for _, file := range files {
		analysis, err := parser.ParseFile(file)
		if err != nil {
			return model.Result{}, err
		}
		result.Warnings = append(result.Warnings, analysis.Warnings...)
		analyses = append(analyses, analysis)
	}

	var metadata *model.DBMetadata
	dsnEnv := cfg.Database.DSNEnv
	if opts.DSNEnv != "" {
		dsnEnv = opts.DSNEnv
	}
	if dsn := os.Getenv(dsnEnv); dsn != "" {
		loaded, err := introspect.LoadPostgres(ctx, dsn)
		if err != nil {
			return model.Result{}, fmt.Errorf("metadata connection failed from %s: %w", dsnEnv, err)
		} else {
			metadata = loaded
		}
	}

	findings := rules.Evaluate(analyses, metadata)
	if metadata != nil {
		result.Warnings = append(result.Warnings, metadata.Warnings()...)
	}
	if aiEnabled(cfg.AI.Enabled, opts.AIOverride) && len(findings) > 0 {
		if opts.AIRedactSQL != nil {
			cfg.AI.RedactSQL = *opts.AIRedactSQL
		}
		var warnings []string
		findings, warnings = ai.Enrich(ctx, cfg.AI, findings)
		result.Warnings = append(result.Warnings, warnings...)
	}
	if findings == nil {
		findings = []model.Finding{}
	}
	result.Findings = findings
	return result, nil
}

func aiEnabled(configEnabled bool, override string) bool {
	switch strings.ToLower(strings.TrimSpace(override)) {
	case "on", "true", "1", "yes":
		return true
	case "off", "false", "0", "no":
		return false
	default:
		return configEnabled
	}
}

func collectSQLFiles(paths []string, strict bool) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				if strict {
					return nil, fmt.Errorf("migration path %q does not exist", path)
				}
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			if strings.EqualFold(filepath.Ext(path), ".sql") {
				clean := filepath.Clean(path)
				if !seen[clean] {
					files = append(files, clean)
					seen[clean] = true
				}
			}
			continue
		}
		err = filepath.WalkDir(path, func(file string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(file), ".sql") {
				clean := filepath.Clean(file)
				if !seen[clean] {
					files = append(files, clean)
					seen[clean] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
