package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/lihua8552-afk/pg-migration-guard/internal/config"
	"github.com/lihua8552-afk/pg-migration-guard/internal/model"
)

type mockClient struct{}

func (mockClient) Explain(context.Context, model.Finding) (string, error) {
	return "Use a safer staged migration.", nil
}

func TestEnrichWithClientDoesNotChangeSeverity(t *testing.T) {
	findings := []model.Finding{{
		RuleID:   "MGD002",
		Severity: model.SeverityHigh,
		File:     "001.sql",
		Line:     1,
	}}
	enriched, warnings := EnrichWithClient(context.Background(), mockClient{}, findings, false)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if enriched[0].Severity != model.SeverityHigh {
		t.Fatalf("severity changed: %s", enriched[0].Severity)
	}
	if enriched[0].AIExplanation == "" {
		t.Fatalf("missing AI explanation")
	}
}

type captureClient struct {
	mu        sync.Mutex
	statement string
}

func (c *captureClient) Explain(_ context.Context, finding model.Finding) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statement = finding.Statement
	return "ok", nil
}

func TestEnrichWithClientCanRedactSQLLiterals(t *testing.T) {
	client := &captureClient{}
	findings := []model.Finding{{
		RuleID:    "MGD030",
		Severity:  model.SeverityHigh,
		File:      "001.sql",
		Line:      1,
		Statement: "UPDATE users SET plan = 'enterprise', seats = 42",
	}}
	_, warnings := EnrichWithClient(context.Background(), client, findings, true)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	client.mu.Lock()
	statement := client.statement
	client.mu.Unlock()
	if strings.Contains(statement, "enterprise") || strings.Contains(statement, "42") {
		t.Fatalf("statement was not redacted: %s", statement)
	}
	if findings[0].Statement != "UPDATE users SET plan = 'enterprise', seats = 42" {
		t.Fatalf("redaction mutated original finding")
	}
}

func TestRedactSQLLiteralsHandlesDollarQuotedAndEscapeStrings(t *testing.T) {
	cases := []struct {
		name string
		in   string
		bad  []string
	}{
		{
			name: "dollar quoted body",
			in:   "DO $body$ raise notice 'secret token'; $body$",
			bad:  []string{"secret token", "$body$"},
		},
		{
			name: "untagged dollar quote",
			in:   "SELECT $$another secret$$",
			bad:  []string{"another secret"},
		},
		{
			name: "escape string literal",
			in:   "UPDATE x SET note = E'line1\\nline2 secret'",
			bad:  []string{"secret"},
		},
		{
			name: "bit and hex string literals",
			in:   "INSERT INTO x VALUES (B'1010', X'1F')",
			bad:  []string{"1010", "1F"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSQLLiterals(tc.in)
			for _, fragment := range tc.bad {
				if strings.Contains(got, fragment) {
					t.Fatalf("expected %q to be redacted from %q, got %q", fragment, tc.in, got)
				}
			}
		})
	}
}

func TestNewClientSupportsOpenAICompatibleGateway(t *testing.T) {
	t.Setenv("MGUARD_TEST_AI_KEY", "test-key")
	var gotPath, gotAuth, gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = body.Model
		if len(body.Messages) == 0 {
			t.Fatalf("missing messages in request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Use a staged migration."}}]}`))
	}))
	defer server.Close()

	client, err := NewClient(config.AIConfig{
		Provider:  "openai-compatible",
		Model:     "deepseek-chat",
		APIKeyEnv: "MGUARD_TEST_AI_KEY",
		BaseURL:   server.URL + "/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := client.Explain(context.Background(), model.Finding{
		RuleID:         "MGD030",
		Severity:       model.SeverityHigh,
		Statement:      "UPDATE users SET disabled = true",
		Reason:         "UPDATE without WHERE.",
		Recommendation: "Batch or add a WHERE clause.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "Use a staged migration." {
		t.Fatalf("text = %q", text)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotModel != "deepseek-chat" {
		t.Fatalf("model = %q", gotModel)
	}
}

func TestOpenAICompatibleRequiresModelAndBaseURL(t *testing.T) {
	t.Setenv("MGUARD_TEST_AI_KEY", "test-key")
	_, err := NewClient(config.AIConfig{
		Provider:  "openai-compatible",
		APIKeyEnv: "MGUARD_TEST_AI_KEY",
		BaseURL:   "https://relay.example.com/v1",
	})
	if err == nil || !strings.Contains(err.Error(), "ai.model") {
		t.Fatalf("expected missing model error, got %v", err)
	}
	_, err = NewClient(config.AIConfig{
		Provider:  "openai-compatible",
		Model:     "deepseek-chat",
		APIKeyEnv: "MGUARD_TEST_AI_KEY",
	})
	if err == nil || !strings.Contains(err.Error(), "ai.base_url") {
		t.Fatalf("expected missing base URL error, got %v", err)
	}
}

func TestOpenAIChatCompletionsEndpointAcceptsCommonGatewayURLs(t *testing.T) {
	cases := map[string]string{
		"https://relay.example.com":                     "https://relay.example.com/v1/chat/completions",
		"https://relay.example.com/v1":                  "https://relay.example.com/v1/chat/completions",
		"https://relay.example.com/proxy/openai/v1":     "https://relay.example.com/proxy/openai/v1/chat/completions",
		"https://relay.example.com/v1/chat/completions": "https://relay.example.com/v1/chat/completions",
	}
	for in, want := range cases {
		got, err := openAIChatCompletionsEndpoint(in)
		if err != nil {
			t.Fatalf("endpoint(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("endpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenAIProviderAllowsCustomBaseURLAndModel(t *testing.T) {
	t.Setenv("MGUARD_TEST_AI_KEY", "test-key")
	client, err := NewClient(config.AIConfig{
		Provider:  "openai",
		Model:     "gateway-model",
		APIKeyEnv: "MGUARD_TEST_AI_KEY",
		BaseURL:   "https://gateway.example.com/v1/chat/completions",
	})
	if err != nil {
		t.Fatal(err)
	}
	openai, ok := client.(*openAIClient)
	if !ok {
		t.Fatalf("client type = %T", client)
	}
	if openai.chatCompletionsURL != "https://gateway.example.com/v1/chat/completions" {
		t.Fatalf("endpoint = %q", openai.chatCompletionsURL)
	}
	if openai.model != "gateway-model" {
		t.Fatalf("model = %q", openai.model)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
