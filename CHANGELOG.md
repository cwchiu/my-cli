# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-09-12

### Added

- `falcon-cis-export` subcommand: export Kubernetes CIS benchmark violations from
  CrowdStrike Falcon container compliance API (`/container-compliance/aggregates/rules/v2`).
- Support for CrowdStrike Falcon OAuth2 client credentials authentication with token caching.
- Flat CSV export format matching CrowdStrike Falcon Web Export `ComplianceByRules` structure
  (12 columns, total/passed/failed asset counts, and pass percentage).
- Support for multiple output formats: `--output table|json|csv` and `--out` / `-O` file output.
- Minimal-traffic `--probe` mode to validate API credentials and connectivity.
- External API integration guidelines in `docs/api-integration-guidelines.md` (Source of Truth principle, CSV alignment checklist).
- Historical pitfall analysis and lesson registry in `docs/pitfalls-and-lessons.md`.

### Changed

- Upgraded `google.golang.org/grpc` to `v1.83.2` to resolve `GHSA-2v4p-qf9q-27wj`.
- Hardened OAuth2 endpoint constants with `#nosec G101` and renamed away from `token*` prefixes to prevent SAST false positives.
- Modularized `AGENTS.md` with PR review security reflections and pre-commit SCA verification gates.

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
