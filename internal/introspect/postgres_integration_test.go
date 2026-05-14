//go:build integration

package introspect

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestLoadPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("MGUARD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MGUARD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `
drop table if exists public.mguard_items;
create table public.mguard_items (
  id bigserial primary key,
  email text not null
);
create index idx_mguard_items_email on public.mguard_items (email);
`)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)

	meta, err := LoadPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	table, ok := meta.LookupTable("mguard_items")
	if !ok {
		t.Fatalf("missing table in metadata")
	}
	if _, ok := table.Columns["email"]; !ok {
		t.Fatalf("missing email column: %#v", table.Columns)
	}
	if len(table.Indexes) == 0 {
		t.Fatalf("missing indexes")
	}
}
