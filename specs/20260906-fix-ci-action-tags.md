# 20260906-fix-ci-action-tags

## Why

fix-security-yaml 合併後（aeec64a），GitHub Actions 已能解析 `security.yml`，
但 gosec 與 osv-scanner 兩個 job 在 action 解析階段報錯：

```
Error: Unable to resolve action `google/osv-scanner-action@v2`, unable to find version `v2`
Error: Unable to resolve action `securego/gosec@v2`, unable to find version `v2`
```

根因：這兩個 repo **沒有發布 floating major tag**（`v2`）。以 GitHub API
（`/repos/<owner>/<repo>/tags`）查證：

- `securego/gosec`：無 `v2` tag，最新為 `v2.29.0`
- `google/osv-scanner-action`：無 `v2` tag，最新為 `v2.5.1`

原假設（skill 模板與 AGENTS.md §9「Actions 釘 major 版本」）預設所有 action
repo 都有 major tag——不成立。其餘 workflow 的 pin 已全部經 API 驗證存在：
checkout@v6、setup-go@v6、upload-artifact@v4、golangci-lint-action@v9、
codeql-action@v4、govulncheck-action@v1、gitleaks-action@v2。

## What

- 目標：讓 security.yml 四個 job 的 action 全部可解析、可執行。
- 範圍：僅 `.github/workflows/security.yml` 的 action 引用與 osv-scanner 步驟。
- 非目標：不動其他 workflow（pin 已驗證有效）；不動 AGENTS.md（另開
  `docs/harden-agents-verification` 工作項）。

## How

1. `securego/gosec@v2` → `securego/gosec@v2.29.0`（釘確切 semver；`args`
   input 經查 v2.29.0 的 action.yml 仍有效）。
2. osv-scanner：v2 的掃描子 action 是 Docker action，僅接受 `scan-args`，
   **v1 的 `sarif_result` input 已不存在**。依官方 reusable workflow 的
   v2 流程改寫為兩步：
   - `osv-scanner-action@v2.5.1` 輸出 JSON（`--format=json --output=...`）
   - `osv-reporter-action@v2.5.1` 將 JSON 轉為 SARIF
   - 既有 `upload-sarif@v4` 步驟不變
3. Dependabot 的 `github-actions` ecosystem 會持續更新確切 semver pin，
   釘死版本不會過時。

## 遭遇的困難

1. `v2` tag 不存在但 skill 模板指示「釘 major」——模板假設不適用於所有 repo。
2. osv-scanner v2 與 v1 的 action 介面不相容：`sarif_result` input 在 v2
   子 action 中已移除，只改 tag 號會導致「input 不存在」的下一個錯誤。
   僅靠網頁擷取 action.yml 內容不可靠（格式錯亂），改用
   `Invoke-WebRequest` 直接取 raw 檔確認。

## 如何解決

1. 以 GitHub API 逐一查證所有 action 的 tag 存在性；無 major tag 的 repo
   改釘確切 semver（v2.29.0 / v2.5.1），靠 Dependabot 更新。
2. 取 v2.5.1 的官方 reusable workflow（`osv-scanner-reusable.yml`）作為
   正確用法參考，改寫為 scan(JSON) → reporter(SARIF) → upload 三步。

## 最後變動了什麼

- `.github/workflows/security.yml`：
  - osv-scanner job：`osv-scanner-action@v2` → `@v2.5.1`，移除 v1 專有的
    `sarif_result` input，改為 scan(JSON, `continue-on-error: true`) +
    reporter(SARIF, `--fail-on-vuln=true`) 兩步（與官方 reusable workflow
    及原 v1 `sarif_result` 的「掃描完成才上報」行為一致）
  - 移除 `--lockfile=go.sum`：osv-scanner 的 Go 支援僅認 `go.mod`
    （go.sum 非其認定的 lockfile 格式，保留會導致掃描失敗）
  - gosec job：`securego/gosec@v2` → `@v2.29.0`
- `specs/20260906-fix-ci-action-tags.md`：本紀錄

（commit hash 待提交後補）

## 驗證結果

- `go build ./...` → OK；`go vet ./...` → OK
- `go tool golangci-lint run` → 0 issues（僅知悉的 gofumpt extra-rules deprecation 警告）
- `go test -shuffle=on ./...` → ok（cmd/my-cli 0.706s）
- `uv run --with pyyaml python` → YAML OK: 7 files（security.yml 改寫後可解析）
- Action 引用逐一經 GitHub API 查證存在：
  - `securego/gosec@v2.29.0`、`google/osv-scanner-action/...@v2.5.1`（本次修復）
  - checkout@v6、setup-go@v6、upload-artifact@v4、golangci-lint-action@v9、
    codeql-action@v4、govulncheck-action@v1、gitleaks-action@v2（既有 pin，確認有效）
- osv-scanner v2 步驟依官方 reusable workflow（v2.5.1 版）模式改寫：
  scan(JSON, continue-on-error) → reporter(SARIF, fail-on-vuln=true) → upload-sarif
