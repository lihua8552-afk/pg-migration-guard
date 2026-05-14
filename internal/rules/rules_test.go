package rules

import (
	"testing"

	"github.com/lihua8552-afk/mguard/internal/model"
	"github.com/lihua8552-afk/mguard/internal/parser"
)

func TestEvaluateCreateIndexLargeTable(t *testing.T) {
	analysis := parser.Parse("001.sql", `CREATE INDEX idx_orders_user_id ON orders (user_id);`)
	meta := model.NewDBMetadata()
	meta.Tables["public.orders"] = model.TableMetadata{
		Schema:       "public",
		Name:         "orders",
		RowsEstimate: 2_000_000,
		SizeBytes:    2 << 30,
		Columns:      map[string]model.ColumnMetadata{},
	}
	findings := Evaluate([]model.FileAnalysis{analysis}, meta)
	assertFinding(t, findings, "MGD002", model.SeverityHigh)
}

func TestEvaluateCreateIndexConcurrentlyInTransaction(t *testing.T) {
	analysis := parser.Parse("001.sql", `BEGIN; CREATE INDEX CONCURRENTLY idx_orders_user_id ON orders (user_id); COMMIT;`)
	findings := Evaluate([]model.FileAnalysis{analysis}, nil)
	assertFinding(t, findings, "MGD001", model.SeverityCritical)
}

func TestEvaluateCreateIndexConcurrentlyAfterCommit(t *testing.T) {
	analysis := parser.Parse("001.sql", `BEGIN; SELECT 1; COMMIT; CREATE INDEX CONCURRENTLY idx_users_email ON users (email);`)
	findings := Evaluate([]model.FileAnalysis{analysis}, nil)
	assertNoFinding(t, findings, "MGD001")
}

func TestEvaluateParseError(t *testing.T) {
	analysis := parser.Parse("bad.sql", `CREAT INDEX idx_bad ON users (email);`)
	findings := Evaluate([]model.FileAnalysis{analysis}, nil)
	assertFinding(t, findings, "MGD000", model.SeverityCritical)
}

func TestEvaluateDestructiveAlterWithoutRollback(t *testing.T) {
	analysis := parser.Parse("002.sql", `ALTER TABLE users DROP COLUMN legacy_status;`)
	findings := Evaluate([]model.FileAnalysis{analysis}, nil)
	assertFinding(t, findings, "MGD010", model.SeverityHigh)
	assertFinding(t, findings, "MGD011", model.SeverityLow)
}

func TestEvaluateDMLWithoutWhere(t *testing.T) {
	analysis := parser.Parse("003.sql", `UPDATE users SET active = false;`)
	findings := Evaluate([]model.FileAnalysis{analysis}, nil)
	assertFinding(t, findings, "MGD030", model.SeverityHigh)
	assertFinding(t, findings, "MGD011", model.SeverityLow)
}

func TestEvaluateShorterVarchar(t *testing.T) {
	analysis := parser.Parse("004.sql", `ALTER TABLE users ALTER COLUMN name TYPE varchar(64);`)
	meta := model.NewDBMetadata()
	meta.Tables["public.users"] = model.TableMetadata{
		Schema: "public",
		Name:   "users",
		Columns: map[string]model.ColumnMetadata{
			"name": {Name: "name", DataType: "character varying(255)"},
		},
	}
	findings := Evaluate([]model.FileAnalysis{analysis}, meta)
	assertFinding(t, findings, "MGD018", model.SeverityHigh)
}

func TestEvaluatePostgresFastDefaultDoesNotEscalateLargeTable(t *testing.T) {
	analysis := parser.Parse("005.sql", `ALTER TABLE users ADD COLUMN plan text NOT NULL DEFAULT 'free';`)
	meta := model.NewDBMetadata()
	meta.PostgresVersionNum = 110000
	meta.Tables["public.users"] = model.TableMetadata{
		Schema:       "public",
		Name:         "users",
		RowsEstimate: 10_000_000,
		SizeBytes:    10 << 30,
		Columns:      map[string]model.ColumnMetadata{},
	}
	findings := Evaluate([]model.FileAnalysis{analysis}, meta)
	assertFinding(t, findings, "MGD016", model.SeverityMedium)
}

func TestEvaluateFunctionDefaultEscalatesLargeTable(t *testing.T) {
	analysis := parser.Parse("005.sql", `ALTER TABLE users ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();`)
	meta := model.NewDBMetadata()
	meta.PostgresVersionNum = 110000
	meta.Tables["public.users"] = model.TableMetadata{
		Schema:       "public",
		Name:         "users",
		RowsEstimate: 10_000_000,
		SizeBytes:    10 << 30,
		Columns:      map[string]model.ColumnMetadata{},
	}
	findings := Evaluate([]model.FileAnalysis{analysis}, meta)
	assertFinding(t, findings, "MGD016", model.SeverityHigh)
}

func TestEvaluateDuplicateIndex(t *testing.T) {
	analysis := parser.Parse("004.sql", `CREATE INDEX CONCURRENTLY idx_users_email_2 ON users (email);`)
	meta := model.NewDBMetadata()
	meta.Tables["public.users"] = model.TableMetadata{
		Schema: "public",
		Name:   "users",
		Indexes: []model.IndexMetadata{{
			Name:    "idx_users_email",
			Columns: []string{"email"},
		}},
	}
	findings := Evaluate([]model.FileAnalysis{analysis}, meta)
	assertFinding(t, findings, "MGD003", model.SeverityLow)
}

func TestEvaluateMultiTableDropAndTruncate(t *testing.T) {
	analysis := parser.Parse("multi.sql", `
DROP TABLE users, orders;
TRUNCATE TABLE sessions, audit_events;
`)
	findings := Evaluate([]model.FileAnalysis{analysis}, nil)
	if got := countFindings(findings, "MGD020"); got != 2 {
		t.Fatalf("MGD020 count = %d, findings = %#v", got, findings)
	}
	if got := countFindings(findings, "MGD031"); got != 2 {
		t.Fatalf("MGD031 count = %d, findings = %#v", got, findings)
	}
}

func assertFinding(t *testing.T, findings []model.Finding, ruleID string, severity model.Severity) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			if finding.Severity != severity {
				t.Fatalf("%s severity = %s, want %s", ruleID, finding.Severity, severity)
			}
			if finding.File == "" {
				t.Fatalf("%s missing file", ruleID)
			}
			return
		}
	}
	t.Fatalf("missing finding %s in %#v", ruleID, findings)
}

func assertNoFinding(t *testing.T, findings []model.Finding, ruleID string) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			t.Fatalf("unexpected finding %s in %#v", ruleID, findings)
		}
	}
}

func countFindings(findings []model.Finding, ruleID string) int {
	count := 0
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			count++
		}
	}
	return count
}
