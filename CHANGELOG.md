# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-09-09

### Added

- Cobra root command with `version` subcommand (`table` / `json` output).
- Layered configuration via Viper: flag > env (`MYCLI_` prefix) > config file > defaults.
- Exit-code contract: `0` success, `1` general error, `2` usage error.
- `openai-chat-test` subcommand: smoke-test OpenAI-compatible chat APIs
  (`--base-api` / `--model` required, `--key`, `--timeout`, `--output table|json`).
- Taskfile targets: `build`, `run`, `test`, `vet`, `fmt`, `lint`, `tidy`, `security:*`
  (govulncheck, osv-scanner, gosec), `clean`.
- CI pipelines (GitHub Actions): Tests, Lint, Security (gosec, CodeQL, govulncheck,
  osv-scanner), Secrets (gitleaks).
- Dependabot updates for `gomod` and GitHub Actions.
- Test suite covering exit-code mapping, configuration loading, version output, and
  the openai-chat-test subcommand (85.9% statement coverage, floor 85%).
- README with badges and coverage information.
- Apache License 2.0.

### Changed

- Adopted Go 1.26 idioms: `errors.AsType[T]`, modernized golangci-lint gofumpt settings.
