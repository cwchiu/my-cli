# Why

Source issue: Issue #28.
This work is intentionally stacked on top of PR #27 (feat/21-translate-file), which has not yet been merged. The goal is to extend the existing translate-file feature without disturbing the original worktree at D:\tmp\my-cli-worktrees\translate-file.

We need to add provider selection for translation calls so the command can support deeplx, google, and microsoft while preserving the current DeepLX behavior. The implementation should remain compatible with the public read-frog endpoints and avoid introducing any new dependency.

# What

- Extend translate-file CLI to support `--provider deeplx|google|microsoft`.
- Preserve the existing DeepLX behavior as the default path.
- Match the read-frog reference implementation (commit `784a6f0`) exactly, with
  no API keys or configuration required from the user:
  - Google: POST `https://translate-pa.googleapis.com/v1/translateHtml` with
    `Content-Type: application/json+protobuf`, the public browser API key
    read-frog embeds in its open-source code (`X-Goog-API-Key`), body
    `[[[escapedText], "auto", "zh-TW"], "wt_lib"]`, response `result[0][0]`.
  - Microsoft: POST `https://edge.microsoft.com/translate/translatetext?from=&to=zh-Hant&isEnterpriseClient=false`
    with `Content-Type: application/json` and no authentication; body is a
    plain JSON string array `"[escapedText]"`; response `[{translations:[{text}]}]`.
- HTML-escape request text (`& < >` and quotes) and HTML-unescape the response
  exactly once, matching read-frog's `escapeText`/decode contract.
- Apply conservative chunk limits for each request to avoid oversized payloads.
- Validate the request/response contracts with `httptest` for all three providers.
- Do not add dependencies beyond the current standard library and existing project packages.

# How

1. Reuse the same translate-file command flow and add a provider selection switch.
2. Keep the current DeepLX request path unchanged when `--provider deeplx` is selected.
3. Add explicit provider-specific request builders for Google and Microsoft that
   mirror the read-frog TypeScript implementations line for line.
4. For Google, POST the protobuf-JSON body with the embedded public browser key
   and parse `result[0][0]` from the JSON array response.
5. For Microsoft, POST a plain JSON string array to the Edge endpoint with
   `from`/`to`/`isEnterpriseClient` query parameters and parse the
   `translations[0].text` of the first response item.
6. Normalize the result into the same internal return structure used by the
   current translation flow so downstream logic is unchanged.
7. Enforce conservative chunking per provider request to avoid long URL or
   payload issues.
8. Add focused `httptest` tests covering request parameters, exact endpoint
   usage, HTML escaping, response decoding, and error handling for deeplx,
   google, and microsoft.
9. Keep the implementation dependency-free and localized to the translation
   integration code path.

# 遭遇的困難

- The same feature branch is stacked on a larger unmerged PR (#27), so we must avoid modifying the original translate-file worktree while still coding the extension.
- Provider contracts differ across DeepLX, Google, and Microsoft; the request structure and response parsing are not identical.
- The first implementation guessed the provider endpoints (`translate_a/single`
  GET and Azure `ttranslatev3`) instead of reading the specified read-frog
  commit; the user rejected it because Google demanded an API key environment
  variable and Microsoft demanded a subscription key/region, while read-frog
  needs neither.
- Embedding the public browser API key trips gosec G101 and gitleaks Google
  API-key detection even though the key is published in read-frog's OSS code.
- gitleaks v8 rejects an empty `[allowlist]` alongside `[[allowlists]]` and
  rejects `[[allowlists]]` entries without at least one rule.
- The user first asked for sentence-level bilingual pairs, then changed the
  requirement to paragraph-level pairs; the output was reworked from one
  whole-file `Source:/Chinese:` block to paragraph-aligned pairs.

# 如何解決

- Create a new worktree/branch for this issue, keeping the earlier translate-file work isolated in its own directory.
- Centralize provider logic behind a small interface or switch so the CLI can route to deeplx, google, or microsoft without affecting existing code paths.
- Fetched the raw read-frog `google.ts` and `microsoft.ts` at the specified
  commit and mirrored their contracts exactly: embedded the public browser key
  as a constant with `#nosec G101`, removed all credential environment
  variables, and added HTML escape/unescape around both providers.
- Suppressed the gitleaks false positive with a `[[allowlists]]` regex entry
  for the exact public key, keeping the scan itself enabled.
- Keep tests narrow and deterministic by using `httptest.Server` to assert request components and mocked payloads.
- Validate locally with the existing Go test and lint workflow without introducing dependencies.

# 最後變動了什麼

- Created a stacked issue branch: `feat/28-translate-providers`.
- Added this specification document: `specs/20260914-translate-providers.md`.
- `cmd/my-cli/translate_file.go`: added `--provider deeplx|google|microsoft`
  with read-frog-compatible adapters: Google POSTs the protobuf-JSON body with
  the embedded public browser key to `translate-pa.googleapis.com/v1/translateHtml`
  and Microsoft POSTs a plain JSON string array to the unauthenticated
  `edge.microsoft.com/translate/translatetext`; both HTML-escape requests and
  HTML-unescape responses; no credential environment variables remain.
- `cmd/my-cli/translate_file_test.go`: added `httptest` contract tests for the
  Google and Microsoft providers, including HTML escaping and entity decoding.
- `.gitleaks.toml`: added a `[[allowlists]]` entry for the public browser key.
- `README.md` and `CHANGELOG.md`: documented provider selection with no keys
  required.
- Paragraph-aligned output (follow-up user request): `translate_file.go` now
  splits the source into paragraphs (blank-line separated), translates each
  paragraph separately (provider chunk limits still apply per paragraph), and
  renders table output as alternating source/translation pairs; `--output
  json` emits an array of `{source, translation}` pairs. Empty files fail with
  "source file contains no translatable text".
- `translate_file_test.go`: updated table/JSON assertions to the pair format
  and added paragraph-splitting, inner-line preservation, and empty-source
  tests.
- Provider contracts are taken from the user-supplied read-frog source at commit
  `784a6f016fbd3fbe7fd3e671e2b94a8cdc9dddab`.

# 驗證結果

- Branch created from `feat/21-translate-file` to preserve the stacked dependency on PR #27.
- New worktree created at `D:\tmp\my-cli-worktrees\translate-providers`.
- Spec file created at `D:\tmp\my-cli-worktrees\translate-providers\specs\20260914-translate-providers.md`.
- No changes were made to `D:\tmp\my-cli-worktrees\translate-file`.
- The planned implementation remains scoped to the issue requirements: provider selection, public endpoint compatibility, mapping, parsing, chunking, and `httptest` validation, with no new dependencies.
- Focused provider tests and command-package lint passed before final validation.
- Root-cause fix after user rejection: replaced the guessed endpoints with the
  exact read-frog contracts and removed every credential requirement.
- Paragraph-aligned output rework: `go build ./...`, `go vet ./...`,
  `go tool golangci-lint run` (0 issues after switching to `strings.SplitSeq`
  per the modernize linter), and `go test -shuffle=on ./...` (all packages ok).
- Local gates: `go build ./...`, `go vet ./...`, `go tool golangci-lint run`
  (0 issues), `go test -shuffle=on ./...` (all packages ok),
  `go tool task security:all` (govulncheck 0 vulns, osv-scanner clean, gosec
  0 issues), and a local gitleaks `dir` scan (no leaks found).
