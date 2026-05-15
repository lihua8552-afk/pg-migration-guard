# GitHub Marketplace Publishing Notes

Postgres Migration Guard is ready to publish as a GitHub Action listing.

## Recommended Listing

- **Action name**: Postgres Migration Guard
- **Repository**: `lihua8552-afk/pg-migration-guard`
- **Primary category**: Code quality
- **Secondary category**: Security
- **Short description**: Catch risky PostgreSQL migrations in CI/CD before they lock tables, destroy data, or break deploys.

## Release Title

```text
Postgres Migration Guard v0.1.1
```

## Release Description

````markdown
Postgres Migration Guard is a PostgreSQL migration safety checker for CI/CD and pull requests.

It scans raw `.sql` migration files before deployment and flags risky DDL/DML such as blocking indexes, destructive drops, unsafe updates, table rewrites, rolling-deploy incompatibilities, and parser failures.

Highlights:

- deterministic PostgreSQL migration safety rules;
- GitHub Action and CLI support;
- Markdown, text, JSON, and SARIF output;
- optional read-only PostgreSQL metadata for better severity;
- optional BYOK AI explanations, including OpenAI-compatible proxy gateways.

Try it in GitHub Actions:

```yaml
- uses: lihua8552-afk/pg-migration-guard@v0.1.1
  with:
    paths: migrations
    fail-on: high
    format: markdown
    comment-pr: "true"
```

For OpenAI-compatible AI gateways:

```yaml
- uses: lihua8552-afk/pg-migration-guard@v0.1.1
  with:
    paths: migrations
    ai-provider: openai-compatible
    ai-base-url: ${{ secrets.AI_BASE_URL }}
    ai-model: deepseek-chat
    ai-api-key: ${{ secrets.AI_API_KEY }}
    ai-redact: "true"
```
````

## Final Manual Step

GitHub Marketplace publication is completed from the GitHub release UI:

1. Open the root `action.yml` page in GitHub.
2. Click **Draft a release**.
3. Use tag `v0.1.1`.
4. Select **Publish this Action to the GitHub Marketplace**.
5. Choose the categories above.
6. Publish the release.

If the Marketplace checkbox is disabled, the repository owner must accept the GitHub Marketplace Developer Agreement and have two-factor authentication enabled.
