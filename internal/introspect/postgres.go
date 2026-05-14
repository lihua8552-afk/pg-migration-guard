package introspect

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/lihua8552-afk/pg-migration-guard/internal/model"
)

func LoadPostgres(ctx context.Context, dsn string) (*model.DBMetadata, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, nil
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, "set default_transaction_read_only = on"); err != nil {
		return nil, fmt.Errorf("enable read-only mode: %w", err)
	}

	meta := model.NewDBMetadata()
	if err := loadVersion(ctx, conn, meta); err != nil {
		return nil, err
	}
	if err := loadTables(ctx, conn, meta); err != nil {
		return nil, err
	}
	if err := loadColumns(ctx, conn, meta); err != nil {
		return nil, err
	}
	if err := loadIndexes(ctx, conn, meta); err != nil {
		return nil, err
	}
	if err := loadConstraints(ctx, conn, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func loadVersion(ctx context.Context, conn *pgx.Conn, meta *model.DBMetadata) error {
	var version int64
	if err := conn.QueryRow(ctx, "select current_setting('server_version_num')::bigint").Scan(&version); err != nil {
		return err
	}
	meta.PostgresVersionNum = version
	return nil
}

func loadTables(ctx context.Context, conn *pgx.Conn, meta *model.DBMetadata) error {
	rows, err := conn.Query(ctx, `
select n.nspname, c.relname, c.reltuples::bigint, pg_total_relation_size(c.oid)::bigint
from pg_class c
join pg_namespace n on n.oid = c.relnamespace
where c.relkind in ('r', 'p')
  and n.nspname not in ('pg_catalog', 'information_schema')
  and n.nspname not like 'pg_toast%'
`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var table model.TableMetadata
		if err := rows.Scan(&table.Schema, &table.Name, &table.RowsEstimate, &table.SizeBytes); err != nil {
			return err
		}
		table.Columns = map[string]model.ColumnMetadata{}
		key := strings.ToLower(table.QualifiedName())
		meta.Tables[key] = table
	}
	return rows.Err()
}

func loadColumns(ctx context.Context, conn *pgx.Conn, meta *model.DBMetadata) error {
	rows, err := conn.Query(ctx, `
select n.nspname, c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull
from pg_attribute a
join pg_class c on c.oid = a.attrelid
join pg_namespace n on n.oid = c.relnamespace
where a.attnum > 0
  and not a.attisdropped
  and c.relkind in ('r', 'p')
  and n.nspname not in ('pg_catalog', 'information_schema')
`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var schema, tableName string
		var column model.ColumnMetadata
		if err := rows.Scan(&schema, &tableName, &column.Name, &column.DataType, &column.NotNull); err != nil {
			return err
		}
		key := strings.ToLower(schema + "." + tableName)
		table := meta.Tables[key]
		if table.Columns == nil {
			table.Columns = map[string]model.ColumnMetadata{}
		}
		table.Columns[strings.ToLower(column.Name)] = column
		meta.Tables[key] = table
	}
	return rows.Err()
}

func loadIndexes(ctx context.Context, conn *pgx.Conn, meta *model.DBMetadata) error {
	rows, err := conn.Query(ctx, `
select schemaname, tablename, indexname, indexdef
from pg_indexes
where schemaname not in ('pg_catalog', 'information_schema')
`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var schema, tableName string
		var idx model.IndexMetadata
		if err := rows.Scan(&schema, &tableName, &idx.Name, &idx.Def); err != nil {
			return err
		}
		idx.Columns = parseIndexColumns(idx.Def)
		key := strings.ToLower(schema + "." + tableName)
		table := meta.Tables[key]
		table.Indexes = append(table.Indexes, idx)
		meta.Tables[key] = table
	}
	return rows.Err()
}

func loadConstraints(ctx context.Context, conn *pgx.Conn, meta *model.DBMetadata) error {
	rows, err := conn.Query(ctx, `
select n.nspname, c.relname, con.conname, con.contype::text
from pg_constraint con
join pg_class c on c.oid = con.conrelid
join pg_namespace n on n.oid = c.relnamespace
where n.nspname not in ('pg_catalog', 'information_schema')
`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var schema, tableName string
		var constraint model.ConstraintMetadata
		if err := rows.Scan(&schema, &tableName, &constraint.Name, &constraint.Type); err != nil {
			return err
		}
		key := strings.ToLower(schema + "." + tableName)
		table := meta.Tables[key]
		table.Constraints = append(table.Constraints, constraint)
		meta.Tables[key] = table
	}
	return rows.Err()
}

var indexColumnsPattern = regexp.MustCompile(`\((.*)\)`)

func parseIndexColumns(indexDef string) []string {
	match := indexColumnsPattern.FindStringSubmatch(indexDef)
	if len(match) != 2 {
		return nil
	}
	body := match[1]
	var columns []string
	depth := 0
	start := 0
	for i, ch := range body {
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				columns = append(columns, cleanIndexColumn(body[start:i]))
				start = i + 1
			}
		}
	}
	columns = append(columns, cleanIndexColumn(body[start:]))
	return columns
}

func cleanIndexColumn(column string) string {
	column = strings.TrimSpace(column)
	column = strings.Trim(column, `"`)
	return strings.ToLower(column)
}
