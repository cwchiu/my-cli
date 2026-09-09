# 20260909 — add-gosec-task（Taskfile 補 security:gosec + 發布 v0.1.0）

## Why

PR #4（openai-chat-test）在 CI 的獨立 gosec 掃描爆出 G101 alert #6，但本機四關全綠——
因為 Taskfile 的 `security:all` 只有 govulncheck + osv-scanner，**沒有 gosec task**，
本機從未跑過獨立 gosec。golangci-lint 的 gosec wrapper 認 `//nolint:gosec`，
而 CI 的 `securego/gosec` action 只認原生 `#nosec`，兩者語意差異導致本機/CI 結果不一致。

使用者要求：補上 `security:gosec` task，並接著發布 v0.1.0。

## What

- Taskfile 新增 `security:gosec` task，納入 `security:all`。
- `.gitignore` 補 `gosec-results.sarif`（掃描產物不進 repo）。
- CHANGELOG 將 `Unreleased` 定版為 `0.1.0`（含 PR #4 的 openai-chat-test 子命令）。
- 打 tag `v0.1.0` 觸發 Release workflow（GoReleaser）發布 GitHub Release。

非目標：req01.txt（Falcon K8s CIS → CSV）屬 0.1.0 之後的功能，另行討論。

## How

- gosec 版本與 CI 對齊：`securego/gosec@v2.29.0` →
  `go run github.com/securego/gosec/v2/cmd/gosec@v2.29.0`（完整 module path，§10.3.9）。
- 參數與 CI 相同：`-no-fail -fmt sarif -out gosec-results.sarif ./...`，
  讓本機輸出可直接比對 CI 的 SARIF 上傳結果。
- 發布流程：worktree 合併回 main → push → main 四關 → tag `v0.1.0` → push tag →
  驗證 Release workflow 與 GitHub Release 產物。

## 遭遇的困難

1. `task` CLI 不在 PATH——Taskfile 定義了 task 卻無法直接以 `task security:gosec` 執行。
2. `gosec-results.sarif` 產物落在 repo 根目錄，`.gitignore` 原本未涵蓋
   （`*.out` 只匹配 `.out` 結尾，SARIF 會被 `git status` 列為未追蹤）。

## 如何解決

1. go.mod 的 `tool` directive 已註冊 `github.com/go-task/task/v3/cmd/task`，
   改用 `go tool task security:gosec` 執行（與 `go tool golangci-lint` 同模式）。
2. `.gitignore` 新增 `gosec-results.sarif`；以 `git check-ignore` 驗證生效
   （exit 0，`git status` 不再顯示該檔）。

## 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `Taskfile.yml` | 新增 `security:gosec` task（v2.29.0，SARIF 輸出）；`security:all` 納入 |
| `.gitignore` | 新增 `gosec-results.sarif` |
| `CHANGELOG.md` | `Unreleased` → `[0.1.0] - 2026-09-09`；補 openai-chat-test 條目 |
| `specs/20260909-add-gosec-task.md` | 本紀錄 |

Commit：`0c08f86`（feat: add security:gosec task and prepare v0.1.0 release）。
Tag：`v0.1.0`（annotated）。Release run：`34256975453`（success）。

## 驗證結果

- YAML 第五關：`uv run --with pyyaml` parser → `YAML OK`。
- `go tool task security:gosec`：exit 0，掃描 5 個檔案（含 `openai_chat.go` 的
  `#nosec G101`），SARIF 產出且被 gitignore。
- 四關（worktree）：`go build` 0 / `go vet` 0 / `golangci-lint run` 0 issues /
  `go test -shuffle=on -count=1` ok 1.163s。
