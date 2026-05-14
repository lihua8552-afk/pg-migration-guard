# Introducing Postgres Migration Guard v0.1.0

## Ship PostgreSQL migrations with a safety net

Database migrations are one of the few code changes that can lock production, rewrite a large table, break rolling deploys, or destroy data in a single statement. Many teams still review these risks manually in pull requests.

Postgres Migration Guard is an open source CLI and GitHub Action that scans raw PostgreSQL `.sql` migrations before deployment. It flags risky DDL and DML with deterministic rules, then reports the result in formats that work naturally in CI: text, Markdown, JSON, and SARIF.

The goal is simple: make dangerous migration patterns visible before they reach production.

## What it catches

Postgres Migration Guard, powered by the `mguard` CLI, detects migration patterns such as:

- `CREATE INDEX CONCURRENTLY` inside an explicit transaction;
- plain `CREATE INDEX` statements that can block writes;
- destructive `DROP COLUMN` and `DROP TABLE` operations;
- table or column renames that can break rolling deploys;
- column type changes and `SET NOT NULL` operations that may rewrite data or take strong locks;
- `UPDATE`, `DELETE`, and `TRUNCATE` operations that can affect too much data;
- SQL parser failures that should not be ignored in CI.

When a read-only PostgreSQL connection is available, mguard can also inspect metadata such as table size, row estimates, columns, indexes, and constraints to make findings more useful.

## Why it is worth trying

- **Fast to adopt**: run it locally or drop it into GitHub Actions.
- **No database required by default**: static SQL checks work fully offline.
- **CI-native output**: Markdown for pull requests, SARIF for code scanning, JSON for automation.
- **PostgreSQL-aware**: built for PostgreSQL migration behavior, not generic SQL linting.
- **Privacy-conscious AI mode**: AI is optional, disabled by default, and only explains findings. Rule severity still comes from the deterministic engine.
- **Open source**: Apache-2.0 licensed and ready for issue-driven feedback.

## Try it in 30 seconds

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

## Use it in GitHub Actions

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
          fail-on: high
          format: markdown
          comment-pr: "true"
```

## Who should try it

Try Postgres Migration Guard if your team:

- reviews PostgreSQL migration files in pull requests;
- wants a lightweight safety gate before production deployment;
- has been burned by table locks, destructive DDL, or broad data rewrites;
- needs migration findings in GitHub Actions or SARIF code scanning;
- wants deterministic checks first, with optional AI explanations only when useful.

This first release focuses on raw PostgreSQL `.sql` migrations. Framework adapters for Rails, Prisma, Alembic, and other migration systems are good candidates for future work.

## Links

- Repository: https://github.com/lihua8552-afk/pg-migration-guard
- Release: https://github.com/lihua8552-afk/pg-migration-guard/releases/tag/v0.1.0
- Feedback form: https://github.com/lihua8552-afk/pg-migration-guard/issues/new?template=feedback.yml
- Discussions: https://github.com/lihua8552-afk/pg-migration-guard/discussions

If you try it on real migrations, feedback on false positives, missing rules, noisy severity, and setup friction is especially valuable.

## 中文

# Postgres Migration Guard v0.1.0 发布介绍

## 给 PostgreSQL 迁移加一层发布前安全网

数据库迁移是少数可以用一条语句锁住生产环境、重写大表、破坏滚动发布，甚至直接删除数据的代码变更。很多团队仍然依赖人工在 PR 里审查这些风险。

Postgres Migration Guard 是一个开源 CLI 和 GitHub Action，用于在部署前扫描原生 PostgreSQL `.sql` 迁移文件。它用确定性的规则识别危险 DDL/DML，并输出适合 CI 使用的 text、Markdown、JSON 和 SARIF 结果。

它的目标很直接：在危险迁移进入生产前，把风险明确展示出来。

## 它能发现什么

由 `mguard` CLI 驱动的 Postgres Migration Guard 可以检测：

- 显式事务中的 `CREATE INDEX CONCURRENTLY`；
- 可能阻塞写入的普通 `CREATE INDEX`；
- 破坏性的 `DROP COLUMN` 和 `DROP TABLE`；
- 可能破坏滚动发布的表重命名、列重命名；
- 可能重写数据或加重锁的列类型变更、`SET NOT NULL`；
- 可能影响过多数据的 `UPDATE`、`DELETE`、`TRUNCATE`；
- CI 中不应该被忽略的 SQL 解析失败。

如果提供只读 PostgreSQL 连接，mguard 还可以读取表大小、行数估计、列、索引和约束等元数据，让风险判断更有上下文。

## 为什么值得试

- **接入快**：可以本地运行，也可以直接放进 GitHub Actions。
- **默认不需要数据库**：静态 SQL 检查可以完全离线运行。
- **适合 CI**：Markdown 用于 PR，SARIF 用于 code scanning，JSON 用于自动化。
- **理解 PostgreSQL 迁移风险**：不是泛泛的 SQL lint，而是面向 PostgreSQL 发布事故场景。
- **隐私友好的 AI 模式**：AI 默认关闭，只用于解释结果；严重级别仍由确定性规则决定。
- **开源可反馈**：Apache-2.0 许可证，欢迎用真实迁移推动规则改进。

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

## 适合谁

如果你的团队有下面这些场景，可以试试 Postgres Migration Guard：

- 在 PR 中审查 PostgreSQL 迁移文件；
- 想在生产发布前加一个轻量的迁移安全门禁；
- 遇到过表锁、破坏性 DDL 或大范围数据重写问题；
- 希望把迁移风险接入 GitHub Actions 或 SARIF code scanning；
- 想先使用确定性规则，只在需要时用 AI 帮助解释。

第一个版本聚焦原生 PostgreSQL `.sql` 迁移。Rails、Prisma、Alembic 以及其他迁移框架适配器会是后续很自然的方向。

## 链接

- 仓库：https://github.com/lihua8552-afk/pg-migration-guard
- Release：https://github.com/lihua8552-afk/pg-migration-guard/releases/tag/v0.1.0
- 反馈表单：https://github.com/lihua8552-afk/pg-migration-guard/issues/new?template=feedback.yml
- 讨论区：https://github.com/lihua8552-afk/pg-migration-guard/discussions

如果你愿意用真实迁移试一下，最有价值的反馈是：误报、漏报、规则缺失、严重级别是否吵、以及哪里让你无法放心接入 CI。
