package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lihua8552-afk/pg-migration-guard/internal/config"
	"github.com/lihua8552-afk/pg-migration-guard/internal/engine"
	"github.com/lihua8552-afk/pg-migration-guard/internal/model"
	"github.com/lihua8552-afk/pg-migration-guard/internal/report"
)

var version = "0.1.0"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "check":
		return runCheck(ctx, args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "mguard %s\n", version)
		return 0
	case "help", "--help", "-h":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runInit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "mguard.yaml", "path to write default configuration")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if _, err := os.Stat(*configPath); err == nil {
		fmt.Fprintf(stderr, "%s already exists\n", *configPath)
		return 1
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(stderr, "stat %s: %v\n", *configPath, err)
		return 1
	}
	if err := os.WriteFile(*configPath, config.DefaultYAML(), 0644); err != nil {
		fmt.Fprintf(stderr, "write %s: %v\n", *configPath, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", *configPath)
	return 0
}

func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "mguard.yaml", "path to mguard.yaml")
	dsnEnv := fs.String("dsn-env", "", "environment variable containing a read-only PostgreSQL DSN")
	format := fs.String("format", "text", "output format: text, json, markdown, sarif")
	failOn := fs.String("fail-on", "", "exit non-zero at or above severity: low, medium, high, critical")
	aiMode := fs.String("ai", "", "AI explanations: on or off")
	aiProvider := fs.String("ai-provider", "", "AI provider: openai, openai-compatible, anthropic, ollama")
	aiModel := fs.String("ai-model", "", "AI model name")
	aiBaseURL := fs.String("ai-base-url", "", "AI provider base URL; for OpenAI-compatible gateways use /v1 or /v1/chat/completions")
	aiAPIKeyEnv := fs.String("ai-api-key-env", "", "environment variable containing the AI API key")
	aiRedact := fs.Bool("ai-redact", false, "redact SQL literals before sending findings to AI providers")
	aiRedactOverride := boolFlagOverride(args, "ai-redact", aiRedact)
	if err := fs.Parse(normalizeCheckArgs(args, map[string]bool{
		"config":         true,
		"dsn-env":        true,
		"format":         true,
		"fail-on":        true,
		"ai":             true,
		"ai-provider":    true,
		"ai-model":       true,
		"ai-base-url":    true,
		"ai-api-key-env": true,
	})); err != nil {
		return 2
	}
	cfg, _, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}
	if *failOn != "" {
		cfg.Risk.FailOn = *failOn
	}
	aiConfigOverridden := false
	if *aiProvider != "" {
		cfg.AI.Provider = *aiProvider
		aiConfigOverridden = true
	}
	if *aiModel != "" {
		cfg.AI.Model = *aiModel
		aiConfigOverridden = true
	}
	if *aiBaseURL != "" {
		cfg.AI.BaseURL = *aiBaseURL
		aiConfigOverridden = true
	}
	if *aiAPIKeyEnv != "" {
		cfg.AI.APIKeyEnv = *aiAPIKeyEnv
		aiConfigOverridden = true
	}
	aiOverride := *aiMode
	if aiOverride == "" && aiConfigOverridden {
		aiOverride = "on"
	}
	threshold, err := model.ParseSeverity(cfg.Risk.FailOn)
	if err != nil {
		fmt.Fprintf(stderr, "invalid fail-on severity: %v\n", err)
		return 2
	}

	result, err := engine.Run(ctx, engine.Options{
		Paths:       fs.Args(),
		StrictPaths: len(fs.Args()) > 0,
		DSNEnv:      *dsnEnv,
		AIOverride:  aiOverride,
		AIRedactSQL: aiRedactOverride,
		Format:      *format,
		Config:      cfg,
	})
	if err != nil {
		fmt.Fprintf(stderr, "check failed: %v\n", err)
		return 1
	}
	var buf bytes.Buffer
	if err := report.Write(&buf, *format, result); err != nil {
		fmt.Fprintf(stderr, "write report: %v\n", err)
		return 2
	}
	if _, err := stdout.Write(buf.Bytes()); err != nil {
		fmt.Fprintf(stderr, "write stdout: %v\n", err)
		return 1
	}
	if result.MaxSeverity().Rank() >= threshold.Rank() {
		return 1
	}
	return 0
}

func boolFlagOverride(args []string, name string, value *bool) *bool {
	for _, arg := range args {
		trimmed := strings.TrimLeft(arg, "-")
		if trimmed == name || strings.HasPrefix(trimmed, name+"=") {
			return value
		}
	}
	return nil
}

func normalizeCheckArgs(args []string, valueFlags map[string]bool) []string {
	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			if before, _, ok := strings.Cut(name, "="); ok {
				name = before
			}
			if valueFlags[name] && !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flags, positional...)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, strings.TrimSpace(`mguard is a PostgreSQL migration safety checker.

Usage:
  mguard init [-config mguard.yaml]
  mguard check [flags] <paths...>
  mguard version

Common check flags:
  -config string    path to mguard.yaml
  -dsn-env string   env var containing a read-only PostgreSQL DSN
  -format string    text, json, markdown, sarif
  -fail-on string   low, medium, high, critical
  -ai string        on or off
  -ai-provider      openai, openai-compatible, anthropic, ollama
  -ai-model         model name for AI explanations
  -ai-base-url      AI API base URL, including /v1 for OpenAI-compatible gateways
  -ai-api-key-env   env var containing the AI API key
  -ai-redact        redact SQL literals before AI explanations
`))
}
