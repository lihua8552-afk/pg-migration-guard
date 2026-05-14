# Postgres Migration Guard

[![CI](https://github.com/lihua8552-afk/pg-migration-guard/actions/workflows/ci.yml/badge.svg)](https://github.com/lihua8552-afk/pg-migration-guard/actions/workflows/ci.yml)
[![Release](https://github.com/lihua8552-afk/pg-migration-guard/actions/workflows/release.yml/badge.svg)](https://github.com/lihua8552-afk/pg-migration-guard/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/lihua8552-afk/pg-migration-guard.svg)](https://pkg.go.dev/github.com/lihua8552-afk/pg-migration-guard)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

[English](README.md) | **简体中文**

**给 PostgreSQL 迁移加一层发布前安全网。** Postgres Migration Guard 由 `mguard` CLI 驱动，会在部署前扫描原生 `.sql` 迁移文件，发现可能锁大表、破坏数据、影响滚动发布或制造难以回滚事故的高风险操作。

mguard 的核心规则引擎是确定性的，不依赖 AI。可选的 BYOK AI 解释只用于帮助理解风险，不决定严重级别，也不会隐藏规则引擎发现的问题。

```text
[CRITICAL] MGD001 CREATE INDEX CONCURRENTLY 不能放在显式事务里。
[HIGH]     MGD010 DROP COLUMN legacy_status 是破坏性变更。
[HIGH]     MGD030 不带 WHERE 的 UPDATE 可能重写整张 accounts 表。
```

## 30 秒试用

```sh
git clone https://github.com/lihua8552-afk/pg-migration-guard.git
cd pg-migration-guard
go run ./cmd/mguard check examples/migrations/001_dangerous.sql --format markdown
```

安装 CLI：

```sh
go install github.com/lihua8552-afk/pg-migration-guard/cmd/mguard@latest
mguard check migrations --format text --fail-on high
```

## 为什么值得关注

- **面向生产事故的规则**：覆盖危险索引、破坏性 DDL、不安全 DML、表重写、SQL 解析失败、滚动发布不兼容等常见迁移风险。
- **理解 PostgreSQL 语义**：使用 WASM 支持的 PostgreSQL 解析器，并在可恢复场景下回退到轻量 tokenizer。
- **适合 CI/CD 接入**：支持 text、Markdown、JSON、SARIF 输出，可用于 PR 检查、GitHub Step Summary 和代码扫描平台。
- **可选数据库元数据增强**：使用只读 PostgreSQL 连接读取表大小、行数估计、列、索引和约束，从而更准确地判断风险等级。
- **隐私友好的 AI 模式**：默认关闭 AI；启用时使用用户自己的 key，并支持 SQL 字面量脱敏。
- **自带 GitHub Action**：可以直接作为 PR 阶段的数据库迁移安全门禁。

生成配置文件：

```sh
mguard init
```

示例 `mguard.yaml`：

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

检查迁移目录或文件：

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

## 可选数据库元数据

mguard 可以完全离线运行。如果希望更准确地判断风险，可以设置只读 PostgreSQL DSN：

```sh
export DATABASE_URL='postgres://readonly:password@localhost:5432/app?sslmode=require'
mguard check migrations --dsn-env DATABASE_URL
```

启用元数据后，mguard 会读取 PostgreSQL catalog views，并设置 `default_transaction_read_only = on`。它不会执行迁移 SQL。

最小权限角色示例：

```sql
create role mguard_readonly login password 'change-me';
grant connect on database app to mguard_readonly;
grant usage on schema public to mguard_readonly;
grant select on all tables in schema public to mguard_readonly;
```

## AI BYOK

AI 解释是可选能力。当前支持 `openai`、`anthropic` 和 `ollama`。

启用 AI 时，mguard 会把每个 finding 的规则、严重级别、文件位置、SQL 语句、原因和建议发送给配置的提供方。可以使用 `redact_sql: true` 或 `--ai-redact` 脱敏字符串、数字、dollar-quoted body 以及 `E'...'`/`B'...'`/`X'...'` 字面量。表名和列名会保留，因为它们对迁移建议有必要。

```sh
MGUARD_AI_KEY=... mguard check migrations --ai on
MGUARD_AI_KEY=... mguard check migrations --ai on --ai-redact
```

## 规则覆盖

| 规则 | 检测内容 |
|---|---|
| `MGD000` | SQL 解析失败 |
| `MGD001` | 显式事务中的 `CREATE INDEX CONCURRENTLY` |
| `MGD002` | 可能阻塞写入的普通 `CREATE INDEX` |
| `MGD003` | 元数据显示可能重复的索引 |
| `MGD010` | 破坏性的 `DROP COLUMN` |
| `MGD011` | 缺少明显回滚/down 提示的不可逆迁移 |
| `MGD012` | 不兼容滚动发布的列重命名 |
| `MGD013` | 不兼容滚动发布的表重命名 |
| `MGD014` | 可能重写数据或加重锁的列类型变更 |
| `MGD015` | `SET NOT NULL` 验证风险 |
| `MGD016` | 向已有数据添加 `NOT NULL` 列 |
| `MGD017` | 添加唯一约束或主键约束 |
| `MGD018` | 缩短字符列长度导致截断风险 |
| `MGD020` | 破坏性的 `DROP TABLE` |
| `MGD030` | 不带 `WHERE` 的 `UPDATE` 或 `DELETE` |
| `MGD031` | 迁移中的 `TRUNCATE` |

## 输出格式

```sh
mguard check migrations --format text
mguard check migrations --format markdown
mguard check migrations --format json
mguard check migrations --format sarif
```

SARIF 输出可以接入 GitHub code scanning 或其他安全看板。

## 项目状态

mguard 适合在早期 CI 阶段检查原生 PostgreSQL `.sql` 迁移。首个版本聚焦确定性的 SQL 安全规则。Rails、Prisma、Alembic 以及其他框架 DSL 适配器可以作为后续方向。

## 开发

```sh
go test ./...
go vet ./...
staticcheck ./...
govulncheck ./...
```

集成测试需要 PostgreSQL 14+：

```sh
MGUARD_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/mguard?sslmode=disable' \
  go test -tags=integration ./...
```

## 许可证

Apache-2.0。详见 [LICENSE](LICENSE)。
