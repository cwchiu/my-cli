# my-cli

[![Tests](https://github.com/cwchiu/my-cli/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/cwchiu/my-cli/actions/workflows/test.yml?query=branch%3Amain)
[![Lint](https://github.com/cwchiu/my-cli/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/cwchiu/my-cli/actions/workflows/lint.yml?query=branch%3Amain)
[![Security](https://github.com/cwchiu/my-cli/actions/workflows/security.yml/badge.svg?branch=main)](https://github.com/cwchiu/my-cli/actions/workflows/security.yml?query=branch%3Amain)
[![Secrets](https://github.com/cwchiu/my-cli/actions/workflows/secrets.yml/badge.svg?branch=main)](https://github.com/cwchiu/my-cli/actions/workflows/secrets.yml?query=branch%3Amain)
![Coverage](https://img.shields.io/badge/coverage-85.9%25-brightgreen)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A multi-subcommand CLI tool built with [Cobra](https://github.com/spf13/cobra) and
[Viper](https://github.com/spf13/viper), following production-grade Go practices:
structured errors, layered configuration, machine-readable output, and a CI
pipeline covering test, lint, SAST, SCA, and secret scanning.

## Demo

```console
$ my-cli version
version: dev
commit:  none
date:    unknown

$ my-cli version --output json
{
    "version": "dev",
    "commit": "none",
    "date": "unknown"
}
```

## Getting started

### Requirements

- Go **1.26** or later (see `go.mod` for the exact `go`/`toolchain` directives)
- [Task](https://taskfile.dev) (optional but recommended — cross-platform task runner)

### Install

```console
$ go install github.com/cwchiu/my-cli/cmd/my-cli@latest
```

Or build from source:

```console
$ git clone https://github.com/cwchiu/my-cli.git
$ cd my-cli
$ task build          # or: go build -o bin/my-cli ./cmd/my-cli
```

The build injects version information via `-ldflags`; `task build` fills in
version, commit, and build date automatically.

### Run

```console
$ my-cli --help
$ my-cli version
$ my-cli version -o json
```

## Commands

| Command              | Description                                                        |
| -------------------- | ------------------------------------------------------------------ |
| `version`            | Print version, commit, and build date.                             |
| `openai-chat-test`   | Smoke-test OpenAI-compatible chat APIs.                            |
| `falcon-cis-export`  | Export CrowdStrike Falcon container CIS violations (CSV/JSON).     |
| `nexus-repo-export`  | Export Sonatype Nexus Repository 3 repository settings (CSV/JSON). |
| `ip-lookup`          | Look up external IP information via ipinfo.io.                     |
| `http-static-server` | Serve a folder over HTTP until interrupted.                        |

Global flags:

| Flag           | Description                                                        |
| -------------- | ------------------------------------------------------------------ |
| `-c, --config` | Path to a config file (optional).                                   |
| `-h, --help`   | Show help for any command.                                          |

`version` accepts `--output, -o table|json` (default `table`) for machine-readable output.

### nexus-repo-export

Exports repository settings from a Sonatype Nexus Repository 3 server via
`GET /service/rest/v1/repositories`.

```console
$ my-cli nexus-repo-export --base-url https://nexus.example.com
$ my-cli nexus-repo-export --base-url https://nexus.example.com --output csv -O repos.csv
$ my-cli nexus-repo-export --base-url https://nexus.example.com --format docker --type proxy
```

| Flag             | Description                                                              |
| ---------------- | ------------------------------------------------------------------------ |
| `--base-url`     | Nexus server base URL (**required**; falls back to `NEXUS_BASE_URL`).    |
| `--username`     | Nexus user for Basic auth (falls back to `NEXUS_USERNAME`).              |
| `--format`       | Only export repositories with this format (e.g. `maven`, `npm`, `docker`). |
| `--type`         | Only export repositories with this type (`hosted\|proxy\|group`).        |
| `--timeout`      | Total run timeout (default `2m`).                                        |
| `--output, -o`   | Output format: `table\|json\|csv` (default `table`).                     |
| `--out, -O`      | Write the output to this file instead of stdout.                         |

Credentials: the password is read from the `NEXUS_PASSWORD` environment
variable **only** (never a flag, so it cannot leak via shell history or
process listings). Anonymous access is used when no username is configured.

Output formats:

- `table` — human-readable summary (`NAME`, `FORMAT`, `TYPE`, `URL`).
- `json` — the **untouched API response**, preserving every field (including
  format-specific attributes such as `docker.httpPort` or maven policies), so
  the export is detailed enough to rebuild the repositories.
- `csv` — a flat 24-column table of the common settings (storage, proxy,
  httpclient, negative/positive cache, cleanup), sorted by name for stable
  diffs. Fields a repository does not have are empty.

Run `my-cli nexus-repo-export --help` for the full reference.

### ip-lookup

Looks up external IP information via [ipinfo.io](https://ipinfo.io). Without
arguments the calling host's own external IP is reported; with one argument
that IPv4 or IPv6 address is looked up instead.

```console
$ my-cli ip-lookup
$ my-cli ip-lookup 8.8.8.8
$ my-cli ip-lookup 2001:4860:4860::8888 --output json
```

| Flag           | Description                                          |
| -------------- | ---------------------------------------------------- |
| `--base-url`   | ipinfo.io base URL (default `https://ipinfo.io`).    |
| `--timeout`    | Request timeout (default `30s`).                     |
| `--output, -o` | Output format: `table\|json` (default `table`).      |

No API token is required: the ipinfo.io `/json` endpoint allows free
anonymous access, which is rate limited per day (exceeding it returns 429).

Output formats:

- `table` — human-readable key-value summary (`ip`, `hostname`, `city`,
  `region`, `country`, `loc`, `org`, `postal`, `timezone`); fields the API
  omitted are skipped.
- `json` — the **untouched API response**, preserving every field.

Run `my-cli ip-lookup --help` for the full reference.

### http-static-server

Serves a folder over HTTP with the standard library file server until
interrupted (Ctrl-C or SIGTERM), then drains in-flight requests before
exiting. Requests are logged to stderr in a structured format; nothing is
written to stdout.

```console
$ my-cli http-static-server --folder ./report
$ my-cli http-static-server --folder ./report --listen 0.0.0.0 --port 9000
$ MYCLI_HTTP_STATIC_SERVER_FOLDER=./report my-cli http-static-server
```

| Flag       | Description                                                                        |
| ---------- | ---------------------------------------------------------------------------------- |
| `--listen` | IP address to bind (default `127.0.0.1`; falls back to `MYCLI_HTTP_STATIC_SERVER_LISTEN`). |
| `--port`   | TCP port to bind, `0` picks a free port (default `8080`; falls back to `MYCLI_HTTP_STATIC_SERVER_PORT`). |
| `--folder` | Folder to serve (**required**; falls back to `MYCLI_HTTP_STATIC_SERVER_FOLDER`).   |

Security: the server binds to `127.0.0.1` only by default, so the folder is
reachable from the local machine alone. Pass `--listen 0.0.0.0` to expose it
to the network. Directory requests serve `index.html` when present, otherwise
a listing; path traversal outside the folder is rejected by the standard file
server.

Run `my-cli http-static-server --help` for the full reference.

### Exit codes

| Code | Meaning                                     |
| ---- | ------------------------------------------- |
| `0`  | Success.                                    |
| `1`  | General error.                              |
| `2`  | Usage error (bad flag or argument).         |

`my-cli` writes data to **stdout** and logs/errors to **stderr**, so it is safe
to use in pipelines.

## Configuration

Configuration is resolved by Viper with the following precedence
(highest first):

```
set > flag > env > config file > defaults
```

- **Config file** (optional): `--config path/to/file`, otherwise searched as
  `my-cli.yaml` in the current directory, then `$HOME`. A missing file is not
  an error; a file that exists but fails to parse is.
- **Environment variables**: prefixed with `MYCLI_`, with `.` mapped to `_`.
  For example, `MYCLI_LOG_LEVEL=debug` sets the `log.level` key.

## Development

All common workflows are defined in [`Taskfile.yml`](Taskfile.yml):

```console
$ task            # list available tasks
$ task build      # build the binary with version ldflags
$ task test       # run tests (shuffled)
$ task vet        # go vet
$ task lint       # golangci-lint
$ task fmt        # gofumpt + goimports
$ task tidy       # go mod tidy
$ task security:all   # govulncheck + osv-scanner
```

The local quality gate is **build → vet → lint → test**; all four must pass
before a change is merged. The race detector (`-race`) runs in CI, which has a
C toolchain available.

### Coverage

Current statement coverage: **85.9%**, with a project floor of **85%**.

```console
$ go test -coverprofile=coverage.out ./...
$ go tool cover -func=coverage.out
```

Coverage focuses on the command surface: exit-code mapping in `run()`,
configuration loading in `initConfig`, and every `version` output format.
`main()` itself is a thin `os.Exit(run())` wrapper and is intentionally excluded.

## CI / Quality

GitHub Actions runs four pipelines on every push and pull request:

| Workflow                                        | Purpose                                                        |
| ----------------------------------------------- | -------------------------------------------------------------- |
| [Tests](.github/workflows/test.yml)             | `go test -race -shuffle=on` with coverage, Go version matrix.   |
| [Lint](.github/workflows/lint.yml)              | `go vet` + `golangci-lint`.                                     |
| [Security](.github/workflows/security.yml)      | `gosec`, CodeQL, `govulncheck`, `osv-scanner`.                  |
| [Secrets](.github/workflows/secrets.yml)        | `gitleaks` full-history secret scan.                            |

Dependabot keeps `gomod` and GitHub Actions dependencies up to date.

## Releasing

Releases are automated with [GoReleaser](https://goreleaser.com) via the
[Release](.github/workflows/release.yml) workflow, triggered by pushing a
`v*` tag:

```console
$ git tag v0.1.0
$ git push origin v0.1.0
```

The workflow builds `linux` / `darwin` / `windows` binaries for `amd64` and
`arm64` with version ldflags injected, generates `checksums.txt`, and publishes
a GitHub Release with a changelog from git history. GoReleaser runs in CI via
`goreleaser-action` and is intentionally **not** a `go.mod` tool dependency
(its dependency tree is huge and unrelated to this CLI). Validate the config
locally with `task release:check`, or dry-run the build with
`task release:build` (snapshot mode, no publish).

## Contributing

Changes are made in a [`git worktree`](https://git-scm.com/docs/git-worktree)
branched from `main`, kept small enough to review in one sitting, and verified
with the four local gates before merging. Each work item leaves an audit record
under [`specs/`](specs/). See [`AGENTS.md`](AGENTS.md) for the full conventions.

1. Fork the repository and create a worktree: `git worktree add ../my-cli-wt/<topic> -b feat/<topic>`
2. Make a focused change and add or update tests.
3. Run `task build vet lint test` and ensure all four pass.
4. Open a pull request describing the *why*.

## License

Licensed under the [Apache License 2.0](LICENSE).

```
Copyright 2026 cwchiu

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```
