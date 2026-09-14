# Why

Source issue: Issue #28.
This work is intentionally stacked on top of PR #27 (feat/21-translate-file), which has not yet been merged. The goal is to extend the existing translate-file feature without disturbing the original worktree at D:\tmp\my-cli-worktrees\translate-file.

We need to add provider selection for translation calls so the command can support deeplx, google, and microsoft while preserving the current DeepLX behavior. The implementation should remain compatible with the public read-frog endpoints and avoid introducing any new dependency.

# What

- Extend translate-file CLI to support `--provider deeplx|google|microsoft`.
- Preserve the existing DeepLX behavior as the default path.
- Support public read-frog-compatible GET endpoints:
  - Google: `https://translate.googleapis.com/translate_a/single`
  - Microsoft: `https://www.bing.com/ttranslatev3`
- Map language codes per provider:
  - Google: `zh-TW` for Traditional Chinese
  - Microsoft: `zh-Hant` for Traditional Chinese
- Parse provider-specific response payloads safely.
- Apply conservative chunk limits for each request to avoid oversized payloads.
- Validate the request/response contracts with `httptest` for all three providers.
- Do not add dependencies beyond the current standard library and existing project packages.

# How

1. Reuse the same translate-file command flow and add a provider selection switch.
2. Keep the current DeepLX request path unchanged when `--provider deeplx` is selected.
3. Add explicit provider-specific request builders for Google and Microsoft using public GET endpoints compatible with the read-frog implementation.
4. For Google, call the single translation endpoint with the appropriate `client`, `sl`, `tl`, `dt`, and `q` parameters, then deserialize the nested arrays in the JSON response.
5. For Microsoft, call the `ttranslatev3` endpoint with the provider-specific language mapping and parse the JSON result from the returned `translations` list.
6. Normalize the result into the same internal return structure used by the current translation flow so downstream logic is unchanged.
7. Enforce conservative chunking per provider request to avoid long URL or payload issues.
8. Add focused `httptest` tests covering request parameters, exact endpoint usage, response decoding, and error handling for deeplx, google, and microsoft.
9. Keep the implementation dependency-free and localized to the translation integration code path.

# 遭遇的困難

- The same feature branch is stacked on a larger unmerged PR (#27), so we must avoid modifying the original translate-file worktree while still coding the extension.
- Provider contracts differ across DeepLX, Google, and Microsoft; the request structure and response parsing are not identical.
- The Google and Microsoft public endpoints are less predictable than the current internal DeepLX flow, and the code must be careful about URL size and payload boundaries.
- The project requires validation through `httptest`, but we should not introduce external packages or dependencies.
- There is a need to maintain backward compatibility while expanding the provider API surface.

# 如何解決

- Create a new worktree/branch for this issue, keeping the earlier translate-file work isolated in its own directory.
- Centralize provider logic behind a small interface or switch so the CLI can route to deeplx, google, or microsoft without affecting existing code paths.
- Use the public read-frog-compatible URL patterns exactly, with provider-specific language mapping and conservative chunk limits.
- Add explicit JSON decoding helpers for each provider response and normalize the output before returning to the common caller.
- Keep tests narrow and deterministic by using `httptest.Server` to assert request components and mocked payloads.
- Validate locally with the existing Go test and lint workflow without introducing dependencies.

# 最後變動了什麼

- Created a stacked issue branch: `feat/28-translate-providers`.
- Added this specification document: `specs/20260914-translate-providers.md`.
- `cmd/my-cli/translate_file.go`: added `--provider deeplx|google|microsoft`,
  Google and Microsoft request/response adapters, conservative request chunking,
  and environment-only Microsoft credentials.
- `cmd/my-cli/translate_file_test.go`: added `httptest` contract tests for the
  Google and Microsoft providers.
- `README.md` and `CHANGELOG.md`: documented provider selection and Microsoft
  environment variables.
- Provider contracts are taken from the user-supplied read-frog source at commit
  `784a6f016fbd3fbe7fd3e671e2b94a8cdc9dddab`.

# 驗證結果

- Branch created from `feat/21-translate-file` to preserve the stacked dependency on PR #27.
- New worktree created at `D:\tmp\my-cli-worktrees\translate-providers`.
- Spec file created at `D:\tmp\my-cli-worktrees\translate-providers\specs\20260914-translate-providers.md`.
- No changes were made to `D:\tmp\my-cli-worktrees\translate-file`.
- The planned implementation remains scoped to the issue requirements: provider selection, public endpoint compatibility, mapping, parsing, chunking, and `httptest` validation, with no new dependencies.
- Focused provider tests and command-package lint passed before final validation.
