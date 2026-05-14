package introspect

import (
	"reflect"
	"testing"
)

func TestParseIndexColumns(t *testing.T) {
	got := parseIndexColumns(`CREATE INDEX idx_users_lower_email ON public.users USING btree (lower(email), created_at)`)
	want := []string{"lower(email)", "created_at"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %#v, want %#v", got, want)
	}
}

func TestCleanIndexColumnRemovesIdentifierQuotes(t *testing.T) {
	if got := cleanIndexColumn(`"Email"`); got != "email" {
		t.Fatalf("cleanIndexColumn = %q", got)
	}
}
