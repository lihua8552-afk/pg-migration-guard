package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lihua8552-afk/pg-migration-guard/internal/config"
	"github.com/lihua8552-afk/pg-migration-guard/internal/model"
)

type Client interface {
	Explain(ctx context.Context, finding model.Finding) (string, error)
}

func NewClient(cfg config.AIConfig) (Client, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch provider {
	case "openai":
		key := os.Getenv(cfg.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("missing API key env %s", cfg.APIKeyEnv)
		}
		if cfg.Model == "" {
			cfg.Model = "gpt-4.1-mini"
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.openai.com/v1"
		}
		return &openAIClient{baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: key, model: cfg.Model, http: defaultHTTPClient()}, nil
	case "anthropic":
		key := os.Getenv(cfg.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("missing API key env %s", cfg.APIKeyEnv)
		}
		if cfg.Model == "" {
			cfg.Model = "claude-haiku-4-5"
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.anthropic.com"
		}
		return &anthropicClient{baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: key, model: cfg.Model, http: defaultHTTPClient()}, nil
	case "ollama":
		if cfg.Model == "" {
			cfg.Model = "llama3.1"
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:11434"
		}
		return &ollamaClient{baseURL: strings.TrimRight(cfg.BaseURL, "/"), model: cfg.Model, http: defaultHTTPClient()}, nil
	default:
		return nil, fmt.Errorf("unknown ai provider %q", cfg.Provider)
	}
}

func Enrich(ctx context.Context, cfg config.AIConfig, findings []model.Finding) ([]model.Finding, []string) {
	client, err := NewClient(cfg)
	if err != nil {
		return findings, []string{fmt.Sprintf("AI disabled: %v", err)}
	}
	return EnrichWithClient(ctx, client, findings, cfg.RedactSQL)
}

func EnrichWithClient(ctx context.Context, client Client, findings []model.Finding, redact bool) ([]model.Finding, []string) {
	out := append([]model.Finding(nil), findings...)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	sem := make(chan struct{}, 4)
	var warnings []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range out {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("AI explanation skipped for %s at %s:%d: %v", out[i].RuleID, out[i].File, out[i].Line, ctx.Err()))
				mu.Unlock()
				return
			}

			finding := out[i]
			if redact {
				finding.Statement = redactSQLLiterals(finding.Statement)
			}
			text, err := client.Explain(ctx, finding)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("AI explanation failed for %s at %s:%d: %v", out[i].RuleID, out[i].File, out[i].Line, err))
				return
			}
			out[i].AIExplanation = strings.TrimSpace(text)
		}()
	}
	wg.Wait()
	return out, warnings
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func prompt(f model.Finding) string {
	return fmt.Sprintf(`You explain PostgreSQL migration safety findings. Do not change severity. Keep the answer concise and actionable.

Rule: %s
Severity: %s
File: %s:%d
SQL: %s
Reason: %s
Recommendation: %s

Return a short explanation and concrete safer migration advice.`, f.RuleID, f.Severity, f.File, f.Line, f.Statement, f.Reason, f.Recommendation)
}

var (
	quotedLiteralPattern  = regexp.MustCompile(`(?i)[ebx]?'([^']|'')*'`)
	numericLiteralPattern = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
)

func redactSQLLiterals(sql string) string {
	sql = redactDollarQuoted(sql)
	sql = quotedLiteralPattern.ReplaceAllString(sql, "'?'")
	sql = numericLiteralPattern.ReplaceAllString(sql, "?")
	return sql
}

func redactDollarQuoted(sql string) string {
	var out strings.Builder
	i := 0
	for i < len(sql) {
		if sql[i] == '$' {
			if tag, tagLen, ok := readDollarTag(sql[i:]); ok {
				rest := sql[i+tagLen:]
				if closeIdx := strings.Index(rest, tag); closeIdx >= 0 {
					out.WriteString("$$?$$")
					i = i + tagLen + closeIdx + len(tag)
					continue
				}
			}
		}
		out.WriteByte(sql[i])
		i++
	}
	return out.String()
}

func readDollarTag(s string) (tag string, length int, ok bool) {
	if len(s) == 0 || s[0] != '$' {
		return "", 0, false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '$' {
			return s[:i+1], i + 1, true
		}
		if !(c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return "", 0, false
		}
	}
	return "", 0, false
}

type openAIClient struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func (c *openAIClient) Explain(ctx context.Context, finding model.Finding) (string, error) {
	reqBody := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a senior PostgreSQL migration reviewer."},
			{"role": "user", "content": prompt(finding)},
		},
		"temperature": 0.1,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := doJSON(c.http, req, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty OpenAI response")
	}
	return resp.Choices[0].Message.Content, nil
}

type anthropicClient struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func (c *anthropicClient) Explain(ctx context.Context, finding model.Finding) (string, error) {
	reqBody := map[string]any{
		"model":       c.model,
		"max_tokens":  500,
		"temperature": 0.1,
		"messages": []map[string]string{
			{"role": "user", "content": prompt(finding)},
		},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := doJSON(c.http, req, &resp); err != nil {
		return "", err
	}
	for _, part := range resp.Content {
		if part.Text != "" {
			return part.Text, nil
		}
	}
	return "", fmt.Errorf("empty Anthropic response")
}

type ollamaClient struct {
	baseURL string
	model   string
	http    *http.Client
}

func (c *ollamaClient) Explain(ctx context.Context, finding model.Finding) (string, error) {
	reqBody := map[string]any{
		"model":  c.model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a senior PostgreSQL migration reviewer."},
			{"role": "user", "content": prompt(finding)},
		},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var resp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := doJSON(c.http, req, &resp); err != nil {
		return "", err
	}
	if resp.Message.Content == "" {
		return "", fmt.Errorf("empty Ollama response")
	}
	return resp.Message.Content, nil
}

func doJSON(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("provider returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}
