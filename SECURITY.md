# Security Policy

## Reporting vulnerabilities

Please report security issues privately by opening a GitHub security advisory in the repository, or by contacting the maintainers listed in the project README.

Do not file public issues for vulnerabilities that could expose user databases, credentials, or private schema details.

## Data handling

mguard is designed to run locally or in CI. It does not execute migrations and does not modify database schema.

When AI explanations are enabled, mguard sends only the finding SQL, rule result, file location, and necessary metadata for that finding to the configured provider. AI is disabled by default. Use `ai.redact_sql: true` or `--ai-redact` to redact single-quoted, dollar-quoted, and escape-prefixed (`E'...'`, `B'...'`, `X'...'`) string literals plus numeric literals before prompts are sent. The flag only takes effect when AI is enabled.

Use a read-only PostgreSQL role for `DATABASE_URL`.
