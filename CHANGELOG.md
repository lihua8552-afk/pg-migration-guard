# Changelog

All notable changes to mguard will be documented in this file.

The project follows semantic versioning after the first stable release.

## Unreleased

- Added first-class OpenAI-compatible gateway support through `provider: openai-compatible`, configurable `base_url`, `model`, and CLI/GitHub Action overrides.

## 0.1.0 - Initial public release

- Added deterministic PostgreSQL migration safety rules.
- Added CLI commands for config initialization and migration checks.
- Added Markdown, text, JSON, and SARIF output formats.
- Added optional PostgreSQL metadata introspection through a read-only DSN.
- Added optional BYOK AI explanations with SQL literal redaction.
- Added composite GitHub Action for pull request migration checks.
- Added CI and release workflows.
