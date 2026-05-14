package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/lihua8552-afk/pg-migration-guard/internal/model"
)

func TestWriteMarkdown(t *testing.T) {
	result := model.Result{
		Tool:        "mguard",
		GeneratedAt: time.Unix(0, 0),
		Findings: []model.Finding{{
			RuleID:         "MGD002",
			Severity:       model.SeverityHigh,
			File:           "migrations/001.sql",
			Line:           3,
			Reason:         "Plain CREATE INDEX can block writes.",
			Recommendation: "Use CREATE INDEX CONCURRENTLY.",
		}},
	}
	var buf bytes.Buffer
	if err := Write(&buf, "markdown", result); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"# mguard report", "MGD002", "migrations/001.sql:3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown output missing %q:\n%s", want, out)
		}
	}
}

func TestWriteSARIF(t *testing.T) {
	result := model.Result{
		Tool:        "mguard",
		GeneratedAt: time.Unix(0, 0),
		Findings: []model.Finding{{
			RuleID:         "MGD020",
			Severity:       model.SeverityCritical,
			File:           "drop.sql",
			Line:           1,
			Reason:         "drop table",
			Recommendation: "do not",
		}},
	}
	var buf bytes.Buffer
	if err := Write(&buf, "sarif", result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"version": "2.1.0"`) || !strings.Contains(buf.String(), `"ruleId": "MGD020"`) {
		t.Fatalf("unexpected SARIF:\n%s", buf.String())
	}
}
