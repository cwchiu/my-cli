# 20260906-update-workflow — 工作流程更新：迴避已踩過的坑 + 迭代不破壞功能

> Worktree：`../my-cli-worktrees/update-workflow`（branch `feat/update-workflow`）
> 日期：2026-09-06

## Why

使用者要求更新工作流程，兩個目標：

1. **回顧 `specs/20260906*.md`，把已處理過的問題變成流程內建防護**——同樣的坑不該靠記憶迴避，要寫進規範讓每次迭代自動執行。
2. **確保每次迭代不破壞原有功能**——目前流程只在「工作項完成時」驗證一次；需要把「驗證」變成每個變更步驟的強制閘門，並在合併後於 main 上再驗證一次（fix-eol 工作項已示範：合併後 main 仍可能出問題）。

## What

回顧三份 specs 萃取出的教訓，對應新增到 AGENTS.md：

| 來源 spec | 踩過的坑 | 流程防護（新增條文） |
|---|---|---|
| add-taskfile | 終端 cwd 漂移，`go get`/`go mod tidy` 污染主 repo | §10.1 新增第 7 條：每個終端命令前確認 cwd 在 worktree；主 repo 出現非預期變更立即 `git checkout --` 還原 |
| add-taskfile | `go get -tool` 用錯 module path | §10.3 新增：`go get -tool` 必須用完整 module path |
| add-lint-config | 修 lint 時引入 `undefined: cmd` 編譯錯誤 | §10.3 第 3 條強化：每次編輯後 build+vet+lint+test 四關全過才進下一步 |
| fix-eol | 合併後 main 上 lint 才爆（attributes 未重新套用） | §10.1 新增第 7 條：**合併後在 main 上重跑完整驗證**（lint/test/build），不通過視同合併未完成 |
| fix-eol | `git diff` 在 autocrlf 下看不到 EOL 變化、輸出被吞 | §10.1 新增：診斷以 `git status --short` + `git ls-files --eol` 為準，不依賴裸 `git diff` |
| fix-eol | ff merge 引入 `.gitattributes` 不會對既有檔案重新套用 | 記錄於 specs（環境陷阱），不需流程條文 |

**非目標**：不改 §1–§9 的技術規範；不新增工具或依賴。

## How

1. 從 `main`（`e9c5c3f`）建立 worktree `feat/update-workflow`。
2. 修改 `AGENTS.md` §10.1（worktree 流程補強）與 §10.3（顆粒度規則補強）。
3. 撰寫本 specs 紀錄。
4. 驗證（lint/test/build）→ 使用者確認 → 合併。

## 遭遇的困難

1. **worktree 檢查發現 2 個檔案 `w/crlf`**：`.gitattributes` 與 `.gitignore` 本身——它們不在 `eol=lf` 規則內（`.gitattributes` 無法規範自己以外的這兩個檔名），且內容為純 ASCII、無 lint 影響。確認為無害，不需處理。

## 如何解決

1. 以 `git ls-files --eol` 確認只有這兩個檔案、`attr/` 欄為空（無規則套用）、lint 0 issues——判定無害，維持現狀。

## 最後變動了什麼

| 檔案 | 變更 | 說明 |
|---|---|---|
| `AGENTS.md` | 修改 | §10.1 新增第 7–9 條（合併後驗證、cwd 紀律、git 診斷準則）；§10.3 第 3 條強化為四關閘門 + 新增第 9 條（依賴操作紀律）；§11 對照表新增 3 列 |
| `specs/20260906-update-workflow.md` | 新增 | 本檔 |

## 驗證結果

- `go build ./...`：exit 0。
- `go vet ./...`：exit 0。
- `go tool golangci-lint run`：**0 issues**（僅既有 gofumpt `extra-rules` deprecation warning）。
- `go test -shuffle=on ./...`：`ok github.com/cwchiu/my-cli/cmd/my-cli 0.606s`。
- `git ls-files --eol`：僅 `.gitattributes`/`.gitignore` 為 `w/crlf`（無規則套用、無害）。
