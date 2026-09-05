# 20260906 — add-taskfile：改用 go-task 作為 task runner + 建立工作紀錄規範

> Worktree：`../my-cli-worktrees/add-taskfile`（branch `feat/add-taskfile`）
> 日期：2026-09-06

## Why

1. 原規範 §2 採 Makefile，但使用者要求 CLI 工具與開發流程**跨 Windows/Linux/macOS**。`make` 在 Windows 非內建（需 scoop/choco/WSL），成為唯一門檻。
2. 使用者隨後決定：**任務調整改用 Task 方案（go-task）**——單一 Go 執行檔、內建跨平台 shell 直譯器，Go 社群主流。
3. 使用者追加要求：為了讓工作**可稽核、可復線**，工作流程需建立 `specs/` 目錄，一個檔案對應一次工作內容，記錄 why / what / how、遭遇的困難、如何解決、最後變動了什麼。

## What

- 以 `Taskfile.yml` 取代原規劃的 Makefile；targets：`build` / `run` / `test` / `vet` / `fmt` / `lint` / `tidy` / `clean` / `default`。
- `task` 本體以 Go 1.24+ 的 **go.mod `tool` directive** 管理（`go tool task`），不要求使用者另裝工具。
- 更新 `AGENTS.md`：§2 技術選型與目錄結構改為 Task；新增 §10.2 工作紀錄規範（原顆粒度規則順移為 §10.3）。
- 建立 `specs/` 目錄與本檔（首份工作紀錄）。

**非目標**：`.golangci.yml`、CI workflow、GoReleaser——各自為獨立工作項，後續以獨立 worktree 處理。

## How

1. 依 §10.1 建立 worktree：`git worktree add ../my-cli-worktrees/add-taskfile -b feat/add-taskfile`。
2. `AGENTS.md`：§2 表格加「Task runner | go-task」列、目錄結構 `Makefile` → `Taskfile.yml`。
3. 撰寫 `Taskfile.yml`（version '3'）：`vars` 以 `sh:` 動態取 `git describe` / `git rev-parse` / `date -u`；`build` 用 `-ldflags "-X main.version=... -X main.commit=... -X main.date=..."` 注入版本資訊；`{{exeExt}}` 處理 Windows 副檔名；`clean` 用 `platforms:` 分別給 POSIX（`rm -f`）與 Windows（`cmd /c del`）。
4. 安裝 tool：`go get -tool github.com/go-task/task/v3/cmd/task@latest` + `go mod tidy`（AGENTS.md §7：Go 1.24+ 開發工具用 tool directive）。
5. 逐一驗證 targets（見「驗證結果」）。
6. 使用者中途追加 specs 需求 → 更新 §10.2 → 建立本檔 → 拆成兩個 commit（功能本體 / 規範+紀錄）。

## 遭遇的困難

1. **`go get -tool go-task/task/cmd/task` 失敗**：`malformed module path: missing dot in first path element`——GitHub repo 路徑不等於 Go module path。
2. **tool directive 一度消失**：第一次 `go get -tool` 成功後，`go mod tidy` 之後 `go.mod` 卻看不到 `tool` 行；且終端 `git diff` 輸出異常（只見 CRLF warning），一度懷疑編輯遺失。
3. **終端 cwd 漂移污染主 repo**：早期 `go get -tool` / `go mod tidy` 在錯誤的 cwd（主 repo `D:\tmp\my-cli`）執行，把 task 的依賴寫進**主 repo** 的 `go.mod/go.sum`；違反 §10.1「不在主 repo 直接改碼」。
4. **`go tool task` 找不到 Taskfile**：`task: No Taskfile found at ""`——同樣是 cwd 漂移：終端還停在主 repo，而 `Taskfile.yml` 只存在於 worktree。
5. **`go tool task` 報 `no such tool "task"`**：切到 worktree 後仍找不到 tool——因為當時 worktree 的 `go.mod` 尚未含 `tool` directive（困難 2 的殘留狀態）。
6. **`.task/` 目錄出現在 git status**：task 執行後產生 state 目錄（checksum），不應進版控。

## 如何解決

1. 改用正確 module path：`go get -tool github.com/go-task/task/v3/cmd/task@latest`（module 是 `github.com/go-task/task/v3`，cmd 在其下）。
2. 根因：困難 3 的 cwd 漂移——第一次「成功」其實發生在主 repo；worktree 的 `go.mod` 從未拿到 tool directive。解法：`git stash push -- go.mod go.sum` 重置 worktree 檔案後，**在 worktree cwd** 重新執行 `go get -tool` + `go mod tidy`，`tool github.com/go-task/task/v3/cmd/task` 確認保留。終端輸出異常改用「寫檔再讀」或 `Select-String` 定向查驗，不依賴裸 `git diff` 輸出。
3. 主 repo 執行 `git -C D:\tmp\my-cli checkout -- go.mod go.sum` 還原；確認 `git status` 乾淨。教訓：**每個終端命令前先確認 cwd 在 worktree**（`Set-Location` 一次後持續使用）。
4. `Set-Location D:\tmp\my-cli-worktrees\add-taskfile` 後重跑，`task --list` 正常列出 9 個 targets。
5. 即困難 2 的同一根因；tool directive 補上後 `go tool task` 正常。
6. `.gitignore` 加入 `.task/`（Task runner state）。

## 最後變動了什麼

| 檔案 | 變更 | Commit |
|---|---|---|
| `Taskfile.yml` | 新增：9 個跨平台 targets，ldflags 版本注入，`{{exeExt}}`/`platforms:` 處理平台差異 | `2bce885` |
| `go.mod` / `go.sum` | 新增 `tool github.com/go-task/task/v3/cmd/task` 及其依賴；`go 1.25.6` → `1.25.10`（toolchain auto 升級） | `2bce885` |
| `.gitignore` | 新增 `.task/` | `2bce885` |
| `AGENTS.md` | §2 Makefile → Taskfile；新增 §10.2 工作紀錄規範（原顆粒度規則 → §10.3） | 本 commit |
| `specs/20260906-add-taskfile.md` | 新增（本檔，首份工作紀錄） | 本 commit |

## 驗證結果

- `go tool task --list`：9 個 targets 全部列出，YAML 語法正確。
- `go tool task build` + `.\my-cli.exe version`：`version: 4f47078-dirty`、`commit: 4f47078`、`date: 2026-09-05T17:25:15Z`——ldflags 注入成功。
- `go tool task test`：`ok github.com/cwchiu/my-cli/cmd/my-cli 0.600s`（本機無 `-race`，見 §5）。
- `go tool task vet`：通過。
- `go tool task tidy`：`go mod tidy` + `git diff --exit-code` 通過（tool directive 保留）。
- `go tool task clean`：`my-cli.exe` 已移除（`Test-Path` → False）。
- 最終 `go build ./... && go vet ./... && go test -shuffle=on ./...`：全綠。
- 主 repo `git status`：乾淨（污染已還原）。

## 待辦（後續工作項）

- `feat/add-lint-config`：`.golangci.yml`（skill 建議完整 v2 設定）。
- `feat/add-ci-workflow`：GitHub Actions（test → lint → security → release；`-race` 只在 CI）。
