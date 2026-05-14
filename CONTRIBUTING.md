# Contributing

Thanks for improving mguard.

## Development

Requirements:

- Go 1.25.10 or newer
- PostgreSQL 14+ for integration tests

Common commands:

```sh
go test ./...
go test -tags=integration ./...
go run ./cmd/mguard check examples/migrations/002_safer.sql --format markdown
```

## Rule changes

Rules should be deterministic. AI output may explain a finding, but it must not decide severity or hide a rule engine result.

When adding a rule:

- Add parser coverage for the SQL shape.
- Add rule tests for safe, warning, and high-risk cases when applicable.
- Include a short, actionable recommendation.
- Keep metadata access read-only.

## Pull requests

Please include tests for behavior changes and update README rule lists when adding public behavior.
