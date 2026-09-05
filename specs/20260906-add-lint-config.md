# specs/20260906-add-lint-config — 加入 golangci-lint 設定

## Why

AGENTS.md §6 規定「以 `.golangci.yml`（v2 格式）為唯一準則；本專案採用完整建議設定」，但專案目前尚無此檔案。Taskfile 的 `lint` target（`golangci-lint run`）已存在，卻沒有設定檔可依循，等於 lint 關卡尚未真正落地。本工作項補上設定檔，讓 lint 成為可執行、可重現的品質關卡。

## What

- 新增 `.golangci.yml`（golangci-lint v2 格式），採用 `samber/cc-skills-golang@golang-lint` 技能提供的完整建議設定（48 個 linters + 2 個 formatters）。
- 依使用者指示擴大範圍：首次 lint 發現既有程式碼 29 個問題（`fmt` 自動修 5 個後剩 24 個），**在同一工作項內修完**，並將「lint 完全通過」寫入 AGENTS.md 作為每個工作項的永久完成條件。
- 非目標：CI workflow（下一個工作項 `feat/add-ci-workflow`）、GoReleaser、依賴升級。

## How

1. 從最新 `main`（`8e4eb4c`）建立 worktree `feat/add-lint-config`。
2. 讀取技能 assets 的建議設定：`c:\Users\cwchi\.copilot\installed-plugins\_direct\https---github-com-samber-cc-skills-golang\skills\golang-lint\assets\.golangci.yml`。
3. 原樣採用（含逐項註解），不做專案客製化調整——先以業界建議基準起跑，日後依實際 lint 結果再調整。
4. 以 `go get -tool` 安裝 golangci-lint v2.13.2 為 go.mod tool directive（經使用者同意，§7），使 lint 不依賴本機 PATH 安裝。
5. 首次 `golangci-lint run` 得 29 issues → `golangci-lint fmt` 自動修 5 個 gofumpt → 剩 24 個逐一修復（見「遭遇的困難」）。
6. 依 §10.2 留下本紀錄，commit 後請使用者確認再合併。

## 遭遇的困難

1. **`go tool task lint` 找不到 golangci-lint**：Taskfile 的 `lint` target 呼叫裸 `golangci-lint`，但 go.mod tool directive 安裝的二進位不在 task 子程序的 PATH 中（錯誤：`executable file not found in $PATH`）。
2. **首次 lint 爆出 29 個問題**：既有程式碼在 48 個 linters 下有 errcheck 4、goconst 1、modernize 1、nolintlint 1、revive 4、testifylint 1、thelper 3、wsl_v5 9、gofumpt 5 個問題。
3. **modernize 建議 `errors.AsType`**：linter 建議將 `errors.As` 改為 `errors.AsType[viper.ConfigFileNotFoundError]`，但該 API 需 Go 1.26，本專案 go.mod 為 `go 1.25.10`，直接改會編譯失敗。
4. **修復過程引入編譯錯誤**：revive 建議將 `PersistentPreRunE` 的參數改為 `_`，但函式體內仍引用原參數名 `cmd`，導致 `undefined: cmd`。

## 如何解決

1. Taskfile `lint`/`fmt` target 改為 `go tool golangci-lint run` / `go tool golangci-lint fmt`，讓 task 子程序經 `go tool` 解析依賴，不再依賴 PATH；`fmt` 同時移除 `go run ...@latest`（每次抓最新版不可重現）。
2. 24 個問題逐一修復：errcheck 將 `fmt.Fprint*` 錯誤包裝回傳（`print version: %w` 等）；goconst 抽 `argVersion` 常數；revive 未用參數改 `_`；thelper 測試 helper 加 `t.Helper()`；testifylint 前置條件改 `require.ErrorIs`；wsl_v5 補空行。全部為行為保持或可接受之行為強化（stdout 寫入失敗現在會回報錯誤）。
3. `errors.As` 保留原寫法，加 `//nolint:modernize // errors.AsType requires Go 1.26; project targets Go 1.25.`（符合 §6 nolintlint 規範：指名 linter + 理由）。待專案升級 Go 1.26 後移除。
4. 以 `go build`/lint typecheck 立即抓到 `undefined: cmd`，將參數命名為 `command` 修正；教訓：revive 未用參數建議需確認函式體是否真的未用。

## 最後變動了什麼

| 檔案 | 變更 | 說明 |
|---|---|---|
| `.golangci.yml` | 新增 | golangci-lint v2 完整建議設定（48 linters、gofumpt+goimports） |
| `go.mod` / `go.sum` | 修改 | 新增 tool directive：golangci-lint v2.13.2（經使用者同意） |
| `Taskfile.yml` | 修改 | `lint`/`fmt` 改用 `go tool golangci-lint`，可重現且不依賴 PATH |
| `cmd/my-cli/args.go` | 修改 | wsl_v5：`return nil` 前補空行 |
| `cmd/my-cli/main.go` | 修改 | wsl_v5：if 區塊前後補空行 |
| `cmd/my-cli/root.go` | 修改 | wsl_v5 補空行；revive 未用參數改 `_`/`command`；移除無效 `//nolint:unused`；`errors.As` 加 nolint 說明 |
| `cmd/my-cli/version.go` | 修改 | errcheck：`fmt.Fprint*` 錯誤包裝回傳；revive 未用參數；wsl_v5 補空行 |
| `cmd/my-cli/version_test.go` | 修改 | goconst 抽 `argVersion`；thelper 加 `t.Helper()`；testifylint 改 `require.ErrorIs`；wsl_v5 補空行 |
| `AGENTS.md` | 修改 | §10.2 新增第 5 條：lint 完全通過（0 issues）為每個工作項的強制完成/合併條件 |
| `specs/20260906-add-lint-config.md` | 新增 | 本工作紀錄 |

## 驗證結果

- `go tool golangci-lint run`：**0 issues**（首次 29 → fmt 後 24 → 修復後 0）。
- `go tool task lint`：成功（Taskfile 修復後 target 可用，0 issues）。
- `go test -shuffle=on ./...`：`ok github.com/cwchiu/my-cli/cmd/my-cli 0.421s`（全綠）。
- 註：`-race` 需 cgo/gcc，本機（Windows 無 gcc）不跑，僅在 CI 執行（§5）。
