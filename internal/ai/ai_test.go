package ai

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/lihua8552-afk/mguard/internal/model"
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
