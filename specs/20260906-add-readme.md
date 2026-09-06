# 20260906 — docs/add-readme

## Why

路線圖工作項 4（文件）。專案目前沒有 README，使用者無法快速了解 my-cli 的用途、
安裝、用法與品質狀態。使用者明確要求：新增 README.md 並包含 coverage 資訊。

## What

- 新增 `README.md`：標題、badges（含 coverage）、摘要、安裝、快速上手、
  命令與設定說明、開發工作流、CI/品質、貢獻指引。
- Coverage 資訊：以 shields.io 靜態 badge 呈現目前 85.9%，並在「品質」章節
  說明計算方式與門檻（≥85%）。
- 非目標：不動任何 Go 代碼；不新增 LICENSE/CHANGELOG（屬後續工作項，
  本 README 不對尚未存在的 LICENSE 做宣告）。

## How

- 依 AGENTS.md §8 的 README 順序：標題 → badges → 摘要 → demo → getting started
  → features → contributing → license。
- 所有事實取自實際檔案：workflow 名稱（Tests/Lint/Security/Secrets）、
  Taskfile 目標、go.mod 的 Go 版本、命令與旗標來自 `cmd/my-cli/`。
- Coverage badge 用 shields.io `static` endpoint（無外部 coverage 服務），
  數值與 `go tool cover` 實測一致。

## 遭遇的困難

1. **順序依賴**：使用者要求 README 含 coverage 資訊，但 85.9% 來自尚未合併的
   `test/expand-coverage`。README 若從舊 main（70.3%）出發會寫出過時數字。
2. **Coverage badge 無資料來源**：本專案沒有 codecov 等外部 coverage 服務，
   shields.io 的動態 coverage endpoint 無處可接。
3. **LICENSE 尚未存在**：AGENTS.md §8 的 README 模板以 license 收尾，但 LICENSE
   是後續工作項，直接宣告 Apache-2.0 會與 repo 實際狀態不符。

## 如何解決

1. 先取得使用者同意合併 `test/expand-coverage`（ff-merge → main 四關重驗 → push →
   清理 worktree），再以 `55bffb2` 為基線建立 `docs/add-readme` worktree。
2. 改用 shields.io **static** badge（`img.shields.io/badge/coverage-85.9%25`），
   並在 README「Coverage」章節附本機重算指令（`go test -coverprofile` +
   `go tool cover -func`），讓數字可驗證、可更新。
3. License 章節如實寫 "TBD，LICENSE 將於後續變更加入"，不做虛假宣告。

## 最後變動了什麼

- `README.md`（新增）：標題、5 個 badges（Tests/Lint/Security/Secrets/coverage
  85.9%/Go 1.26）、摘要、demo、安裝、命令與 exit codes、設定優先序、
  Taskfile 開發工作流、coverage 計算方式與 85% 門檻、CI 四管線表、貢獻指引、
  license TBD 宣告。
- `specs/20260906-add-readme.md`：本紀錄。

## 驗證結果

- `go build ./...` ✅
- `go vet ./...` ✅
- `go tool golangci-lint run` → **0 issues** ✅
- `go test -shuffle=on ./...` → ok ✅
- README 事實查核：workflow 名稱與 `.github/workflows/*.yml` 的 `name:` 一致；
  Taskfile 目標與 `Taskfile.yml` 一致；Go 版本與 `go.mod`（1.26.0/toolchain 1.26.8）
  一致；命令/旗標/exit codes 與 `cmd/my-cli/` 實作一致；coverage 85.9% 與
  `go tool cover -func` 實測一致。

