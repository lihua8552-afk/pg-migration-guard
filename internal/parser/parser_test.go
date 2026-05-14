package parser

import (
	"testing"

	"github.com/lihua8552-afk/mguard/internal/model"
)

func TestParseCreateIndexConcurrently(t *testing.T) {
	analysis := Parse("001.sql", `
BEGIN;
CREATE UNIQUE INDEX CONCURRENTLY idx_users_email ON public.users (email);
COMMIT;
`)
	if !analysis.HasExplicitTransaction {
		t.Fatalf("expected explicit transaction")
	}
	if len(analysis.Statements) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(analysis.Statements))
	}
	stmt := analysis.Statements[1]
	if stmt.Kind != model.KindCreateIndex {
		t.Fatalf("kind = %s", stmt.Kind)
	}
	if stmt.StartLine != 3 {
		t.Fatalf("start line = %d", stmt.StartLine)
	}
	if !stmt.Concurrently || !stmt.Unique {
		t.Fatalf("expected concurrent unique index")
	}
	if stmt.SchemaName != "public" || stmt.TableName != "users" || stmt.IndexName != "idx_users_email" {
		t.Fatalf("unexpected names: %#v", stmt)
	}
	if len(stmt.Columns) != 1 || stmt.Columns[0] != "email" {
		t.Fatalf("unexpected columns: %#v", stmt.Columns)
	}
}

func TestParseTransactionScope(t *testing.T) {
	analysis := Parse("tx.sql", `
BEGIN;
CREATE INDEX CONCURRENTLY idx_inside ON users (email);
COMMIT;
CREATE INDEX CONCURRENTLY idx_outside ON users (created_at);
`)
	if len(analysis.Statements) != 4 {
		t.Fatalf("expected 4 statements, got %d", len(analysis.Statements))
	}
	if !analysis.Statements[1].InTransaction {
		t.Fatalf("expected index inside transaction to be marked in transaction")
	}
	if analysis.Statements[3].InTransaction {
		t.Fatalf("expected index after COMMIT not to be marked in transaction")
	}
}

func TestParseInvalidSQLAddsParseErrorStatement(t *testing.T) {
	analysis := Parse("bad.sql", `CREAT INDEX idx_bad ON users (email);`)
	if len(analysis.Statements) == 0 {
		t.Fatalf("expected parse error statement")
	}
	if analysis.Statements[0].Kind != model.KindParseError {
		t.Fatalf("first statement kind = %s", analysis.Statements[0].Kind)
	}
	if analysis.Statements[0].ParseError == "" {
		t.Fatalf("expected parse error text")
	}
}

func TestRollbackHintRequiresDownMigrationComment(t *testing.T) {
	if hasRollbackHint("BEGIN; ROLLBACK; COMMIT;") {
		t.Fatalf("transaction rollback statement should not count as a down migration hint")
	}
	if !hasRollbackHint("-- +down\nDROP TABLE users;") {
		t.Fatalf("expected -- +down to count as rollback hint")
	}
	if !hasRollbackHint("-- migrate:down\nDROP TABLE users;") {
		t.Fatalf("expected -- migrate:down to count as rollback hint")
	}
}

func TestParseAlterTableActions(t *testing.T) {
	analysis := Parse("002.sql", `
ALTER TABLE accounts
  ADD COLUMN plan text NOT NULL DEFAULT 'free',
  ALTER COLUMN status TYPE varchar(12),
  DROP COLUMN legacy_status,
  ADD CONSTRAINT accounts_email_unique UNIQUE (email);
`)
	if len(analysis.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(analysis.Statements))
	}
	stmt := analysis.Statements[0]
	if stmt.Kind != model.KindAlterTable {
		t.Fatalf("kind = %s", stmt.Kind)
	}
	if stmt.TableName != "accounts" {
		t.Fatalf("table = %s", stmt.TableName)
	}
	if len(stmt.AlterActions) != 4 {
		t.Fatalf("expected 4 actions, got %#v", stmt.AlterActions)
	}
	if stmt.AlterActions[0].Type != model.AlterAddColumn || !stmt.AlterActions[0].HasNotNull || !stmt.AlterActions[0].HasDefault {
		t.Fatalf("unexpected add column action: %#v", stmt.AlterActions[0])
	}
	if !stmt.AlterActions[0].DefaultConstant {
		t.Fatalf("expected constant default: %#v", stmt.AlterActions[0])
	}
	if stmt.AlterActions[1].Type != model.AlterColumnType {
		t.Fatalf("unexpected alter type action: %#v", stmt.AlterActions[1])
	}
	if stmt.AlterActions[1].DataType != "pg_catalog.varchar(12)" {
		t.Fatalf("unexpected alter type data type: %q", stmt.AlterActions[1].DataType)
	}
	if stmt.AlterActions[2].Type != model.AlterDropColumn {
		t.Fatalf("unexpected drop column action: %#v", stmt.AlterActions[2])
	}
	if stmt.AlterActions[3].ConstraintType != "unique" {
		t.Fatalf("unexpected constraint action: %#v", stmt.AlterActions[3])
	}
}

func TestParseAddColumnFunctionDefaultIsNotConstant(t *testing.T) {
	analysis := Parse("volatile.sql", `ALTER TABLE users ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();`)
	if len(analysis.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(analysis.Statements))
	}
	actions := analysis.Statements[0].AlterActions
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %#v", actions)
	}
	if !actions[0].HasDefault {
		t.Fatalf("expected default: %#v", actions[0])
	}
	if actions[0].DefaultConstant {
		t.Fatalf("function default should not be treated as constant: %#v", actions[0])
	}
}

func TestParseMultiTableDropAndTruncate(t *testing.T) {
	analysis := Parse("multi.sql", `
DROP TABLE public.users, archive.orders;
TRUNCATE TABLE users, audit.events RESTART IDENTITY;
`)
	if len(analysis.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(analysis.Statements))
	}
	drop := analysis.Statements[0]
	if got := drop.QualifiedTables(); len(got) != 2 || got[0] != "public.users" || got[1] != "archive.orders" {
		t.Fatalf("unexpected drop tables: %#v", got)
	}
	truncate := analysis.Statements[1]
	if got := truncate.QualifiedTables(); len(got) != 2 || got[0] != "users" || got[1] != "audit.events" {
		t.Fatalf("unexpected truncate tables: %#v", got)
	}
}

func TestParseDMLAndTruncate(t *testing.T) {
	analysis := Parse("003.sql", `
UPDATE users SET active = false;
DELETE FROM sessions WHERE expires_at < now();
TRUNCATE TABLE audit_events;
`)
	if got := analysis.Statements[0]; got.Kind != model.KindUpdate || got.HasWhere {
		t.Fatalf("unexpected update parse: %#v", got)
	}
	if got := analysis.Statements[1]; got.Kind != model.KindDelete || !got.HasWhere {
		t.Fatalf("unexpected delete parse: %#v", got)
	}
	if got := analysis.Statements[2]; got.Kind != model.KindTruncate || got.TableName != "audit_events" {
		t.Fatalf("unexpected truncate parse: %#v", got)
	}
}
