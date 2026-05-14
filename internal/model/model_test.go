package model

import "testing"

func TestLookupTableAvoidsAmbiguousSchemaMatches(t *testing.T) {
	meta := NewDBMetadata()
	meta.Tables["archive.orders"] = TableMetadata{Schema: "archive", Name: "orders"}
	meta.Tables["tenant.orders"] = TableMetadata{Schema: "tenant", Name: "orders"}

	if _, ok := meta.LookupTable("orders"); ok {
		t.Fatalf("expected ambiguous bare table name not to match")
	}
	if table, ok := meta.LookupTable("archive.orders"); !ok || table.Schema != "archive" {
		t.Fatalf("expected exact schema match, got %#v ok=%v", table, ok)
	}
}

func TestLookupTablePrefersPublicForBareName(t *testing.T) {
	meta := NewDBMetadata()
	meta.Tables["archive.orders"] = TableMetadata{Schema: "archive", Name: "orders"}
	meta.Tables["public.orders"] = TableMetadata{Schema: "public", Name: "orders"}

	table, ok := meta.LookupTable("orders")
	if !ok || table.Schema != "public" {
		t.Fatalf("expected public schema match, got %#v ok=%v", table, ok)
	}
}

func TestLookupTableRecordsAmbiguousNamesAsWarnings(t *testing.T) {
	meta := NewDBMetadata()
	meta.Tables["archive.orders"] = TableMetadata{Schema: "archive", Name: "orders"}
	meta.Tables["tenant.orders"] = TableMetadata{Schema: "tenant", Name: "orders"}

	if _, ok := meta.LookupTable("orders"); ok {
		t.Fatalf("ambiguous lookup should not return a table")
	}
	if _, ok := meta.LookupTable("orders"); ok {
		t.Fatalf("ambiguous lookup should not return a table on repeat call")
	}

	warnings := meta.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("expected one deduplicated warning, got %#v", warnings)
	}
	if want := "table \"orders\""; warnings[0][:len(want)] != want {
		t.Fatalf("unexpected warning prefix: %q", warnings[0])
	}
}

func TestWarningsEmptyWhenNoAmbiguity(t *testing.T) {
	meta := NewDBMetadata()
	meta.Tables["public.orders"] = TableMetadata{Schema: "public", Name: "orders"}
	if _, ok := meta.LookupTable("orders"); !ok {
		t.Fatalf("expected public match")
	}
	if got := meta.Warnings(); got != nil {
		t.Fatalf("expected no warnings, got %#v", got)
	}
}

func TestStatementQualifiedTablesUsesTableRefs(t *testing.T) {
	stmt := Statement{
		SchemaName: "public",
		TableName:  "users",
		TableRefs: []TableRef{
			{SchemaName: "public", TableName: "users"},
			{SchemaName: "archive", TableName: "orders"},
		},
	}
	got := stmt.QualifiedTables()
	if len(got) != 2 || got[0] != "public.users" || got[1] != "archive.orders" {
		t.Fatalf("qualified tables = %#v", got)
	}
}
