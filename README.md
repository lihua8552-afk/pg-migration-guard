# Postgres Migration Guard

[![CI](https://github.com/lihua8552-afk/pg-migration-guard/actions/workflows/ci.yml/badge.svg)](https://github.com/lihua8552-afk/pg-migration-guard/actions/workflows/ci.yml)
[![Release](https://github.com/lihua8552-afk/pg-migration-guard/actions/workflows/release.yml/badge.svg)](https://github.com/lihua8552-afk/pg-migration-guard/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/lihua8552-afk/pg-migration-guard.svg)](https://pkg.go.dev/github.com/lihua8552-afk/pg-migration-guard)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**English** | [简体中文](README.zh-CN.md)

**Ship PostgreSQL migrations with a safety net.** Postgres Migration Guard, powered by the `mguard` CLI, scans raw `.sql` migrations before deploy and flags changes that can lock large tables, destroy data, break rolling deploys, or create hard-to-roll-back production incidents.

The core rule engine is deterministic and does not require AI. Optional bring-your-own-key AI explanations can make findings easier to understand, but AI never decides severity and never suppresses rule results.

```text
[CRITICAL] MGD001 CREATE INDEX CONCURRENTLY cannot run inside an explicit transaction.
[HIGH]     MGD010 Dropping column legacy_status is destructive.
[HIGH]     MGD030 UPDATE without WHERE can rewrite every row in accounts.
```

## Try It In 30 Seconds

```sh
git clone https://github.com/lihua8552-afk/pg-migration-guard.git
cd pg-migration-guard
go run ./cmd/mguard check examples/migrations/001_dangerous.sql --format markdown
```

Or install the CLI:

```sh
go install github.com/lihua8552-afk/pg-migration-guard/cmd/mguard@latest
mguard check migrations --format text --fail-on high
```

## Why Teams Notice It

- **Production-focused migration rules**: catches risky indexes, destructive DDL, unsafe DML, table rewrites, invalid SQL, and rolling-deploy incompatibilities.
- **PostgreSQL-aware parsing**: uses a PostgreSQL parser backed by WASM with a tokenizer fallback for recoverable parser failures.
- **CI-native outputs**: supports text, Markdown, JSON, and SARIF for pull request checks, GitHub summaries, and code scanning dashboards.
- **Optional database metadata**: can read table sizes, row estimates, columns, indexes, and constraints through a read-only PostgreSQL connection for more accurate severity.
- **Privacy-conscious AI mode**: AI is disabled by default; BYOK providers are optional, and SQL literal redaction is available.
- **GitHub Action included**: use the project as a drop-in migration safety gate in pull requests.

Create a config file:

```sh
mguard init
```

Example `mguard.yaml`:

```yaml
database:
  dialect: postgres
  dsn_env: DATABASE_URL

migrations:
  paths:
    - migrations
    - db/migrate

risk:
  fail_on: high

ai:
  enabled: false
  provider: openai
  model: ""
  api_key_env: MGUARD_AI_KEY
  base_url: ""
  redact_sql: false
```

Run against one or more files or directories:

```sh
mguard check migrations --format text --fail-on high
```

## GitHub Action

```yaml
name: Migration Safety

on:
  pull_request:

jobs:
  mguard:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
      - uses: lihua8552-afk/pg-migration-guard@v0.1.0
        with:
          paths: |
            migrations
            db/migrate
          database-url: ${{ secrets.READONLY_DATABASE_URL }}
          fail-on: high
          format: markdown
          comment-pr: "true"
```

## Optional Database Metadata

mguard can run fully offline. For more accurate risk scoring, set a read-only PostgreSQL DSN:

```sh
export DATABASE_URL='postgres://readonly:password@localhost:5432/app?sslmode=require'
mguard check migrations --dsn-env DATABASE_URL
```

With metadata enabled, mguard reads PostgreSQL catalog views and sets `default_transaction_read_only = on`. It never executes migrations.

Minimal PostgreSQL role example:

```sql
create role mguard_readonly login password 'change-me';
grant connect on database app to mguard_readonly;
grant usage on schema public to mguard_readonly;
grant select on all tables in schema public to mguard_readonly;
```

## AI BYOK

AI explanations are optional. Supported providers are `openai`, `anthropic`, and `ollama`.

When AI is enabled, mguard sends each finding's rule, severity, file location, SQL statement, reason, and recommendation to the configured provider. Use `redact_sql: true` or `--ai-redact` to replace string and numeric SQL literals, including dollar-quoted bodies and `E'...'`/`B'...'`/`X'...'` strings. Table and column names remain visible because they are needed for useful migration advice.

```sh
MGUARD_AI_KEY=... mguard check migrations --ai on
MGUARD_AI_KEY=... mguard check migrations --ai on --ai-redact
```

## Rule Coverage

| Rule | Detects |
|---|---|
| `MGD000` | SQL parser failure |
| `MGD001` | `CREATE INDEX CONCURRENTLY` inside an explicit transaction |
| `MGD002` | Plain `CREATE INDEX` that can block writes |
| `MGD003` | Possible duplicate index when metadata shows an equivalent index |
| `MGD010` | Destructive `DROP COLUMN` |
| `MGD011` | Irreversible migration without an obvious rollback/down hint |
| `MGD012` | Backwards-incompatible column rename |
| `MGD013` | Backwards-incompatible table rename |
| `MGD014` | Column type change that may rewrite data or take strong locks |
| `MGD015` | `SET NOT NULL` validation risk |
| `MGD016` | Adding a `NOT NULL` column to existing rows |
| `MGD017` | Adding unique or primary key constraints |
| `MGD018` | Shortening character column length and risking data truncation |
| `MGD020` | Destructive `DROP TABLE` |
| `MGD030` | `UPDATE` or `DELETE` without `WHERE` |
| `MGD031` | `TRUNCATE` in a migration |

## Output Formats

```sh
mguard check migrations --format text
mguard check migrations --format markdown
mguard check migrations --format json
mguard check migrations --format sarif
```

Use SARIF with GitHub code scanning or other security dashboards.

## Project Status

mguard is suitable for early CI adoption on raw PostgreSQL `.sql` migrations. The first release focuses on deterministic safety checks for SQL files. Rails, Prisma, Alembic, and framework-specific migration adapters are good candidates for future work.

## Feedback

Tried it on real migrations? Please leave a quick rating or adoption note through the [feedback form](https://github.com/lihua8552-afk/pg-migration-guard/issues/new?template=feedback.yml). The most useful feedback includes:

- false positives or false negatives, with redacted SQL if possible;
- missing rules, framework adapters, or CI integrations;
- what would make the tool trustworthy enough for your release workflow.

Open-ended discussion is welcome in [GitHub Discussions](https://github.com/lihua8552-afk/pg-migration-guard/discussions).

## Development

```sh
go test ./...
go vet ./...
staticcheck ./...
govulncheck ./...
```

Integration tests require PostgreSQL 14+:

```sh
MGUARD_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/mguard?sslmode=disable' \
  go test -tags=integration ./...
```

## License

Apache-2.0. See [LICENSE](LICENSE).
