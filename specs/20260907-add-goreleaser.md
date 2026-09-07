# 20260907 — feat/add-goreleaser

## Why

- 專案品質管線（test/lint/security/secrets）已齊備，但缺少發布機制：目前只能手動 `task build`
  後手工上傳，無法多平台 cross-compile、無 checksums、無 changelog 自動化。
- AGENTS.md §9 的 CI 階段順序 test → lint → security → **release** 中，release（GoReleaser）是最後一哩。
- 使用者核准 roadmap 最後一項，並同意 goreleaser 作為新依賴（§7）；本項起改採 **PR 流程**
  （push 分支 + GitHub PR），不再本地 ff-merge。

## What

- 新增 `.goreleaser.yaml`：多平台建置（linux/darwin/windows × amd64/arm64，CGO_ENABLED=0）、
  ldflags 注入 `main.version/commit/date`（與 Taskfile build 一致，改由 goreleaser 模板提供）、
  tar.gz/zip 封存、`checksums.txt`、changelog 過濾 docs/test/chore、GitHub Release 發布。
- 新增 `.github/workflows/release.yml`：push tag `v*` 觸發，`permissions: contents: write`，
  `fetch-depth: 0`（changelog 需完整歷史），用 `goreleaser/goreleaser-action@v7` 釘 version v2.18.1。
- `Taskfile.yml` 新增 `release:check`（驗證設定）與 `release:build`（snapshot 試 build）兩個目標，
  以 `go run github.com/goreleaser/goreleaser/v2@v2.18.1` 臨時執行。
- `README.md` 新增 Releasing 章節（tag 觸發流程、平台矩陣、為何不進 go.mod）。
- 非目標：不打 tag、不實際發布（首次發布為獨立的人工 checkpoint）；不動 go.mod。

## How

1. `git worktree add ../my-cli-worktrees/add-goreleaser -b feat/add-goreleaser`（自 89b5202）。
2. **依賴選型（關鍵決策）**：原計畫 `go get -tool github.com/goreleaser/goreleaser/v2`，實測
   v2.18.1 要求 `go >= 1.27.1`，會把全專案 toolchain 由 1.26.8 抬到 1.27.1，且拖入 AWS SDK、
   Docker CLI、GCP/OpenTelemetry 等數百個與本 CLI 無關的間接依賴。立即終止 `go get`
   （go.mod 未受污染，經 `git status` 確認）。與使用者確認後改採：
   - **CI**：`goreleaser-action` 自行下載 goreleaser binary（不進 go.mod）。
   - **本機**：Taskfile 用 `go run …@v2.18.1` 臨時執行（與 osv-scanner 同模式，已有先例）。
   - 好處：go.mod 乾淨、toolchain 維持 1.26、依賴膨脹只發生在開發者快取。
3. Action pin 查證（§9）：`gh api repos/goreleaser/goreleaser-action/tags` 確認 `v7` floating tag
   存在（v7.2.3 為最新），故 `@v7` 合法， Dependabot 可更新。
4. `.goreleaser.yaml` 重點：`version` 用 `{{.Version}}`（tag 名，含 v 前綴，與 version 子命令
   ldflags 語意一致）；封存命名 `my-cli_Linux_x86_64.tar.gz` / `my-cli_Windows_x86_64.zip`。
5. 驗證：YAML 第五關（pyyaml parse 全部 .github yml + .goreleaser.yaml + Taskfile.yml）→
   `go run goreleaser@v2.18.1 check` → 四關。

## 遭遇的困難

1. **goreleaser v2 強制 toolchain 1.27.1**：`go get -tool` 執行中出現
   `requires go >= 1.27.1; switching to go1.27.1`，且依賴下載量巨大（AWS/Docker/GCP）。
   若放任完成，go.mod 的 toolchain 行與 go.sum 將被大幅改寫，偏離「純 Go 小 CLI」的定位。
2. **`go get` 逾時移入背景**：300 秒未完成，無法直接觀察結束狀態，需以 go.mod 實際內容判斷寫入與否。

## 如何解決

1. 停下 `go get`（kill terminal），以 `Get-Content go.mod` + `git status --short go.mod go.sum`
   確認 go.mod 未被改寫（go get 在寫入前被終止）。改採「CI 用 action + 本機 go run」方案，
   經使用者確認選擇。此決策記錄於 README Releasing 章節，避免後來者重複踩坑。
2. 以 go.mod 內容（而非終端輸出）作為 ground truth（§10.1 第 9 條精神）；後果驗證：四關全綠且
   `git status` 僅顯示預期檔案。

## 最後變動了什麼

- `.goreleaser.yaml`（新增）：建置/封存/checksum/changelog/release 設定。
- `.github/workflows/release.yml`（新增）：tag `v*` 觸發的發布 workflow。
- `Taskfile.yml`（修改）：新增 `release:check`、`release:build`。
- `README.md`（修改）：新增 Releasing 章節。
- `specs/20260907-add-goreleaser.md`（新增）：本紀錄。
- commit：見分支 feat/add-goreleaser（hash 由 PR 描述記錄）。

## 驗證結果

- YAML 第五關：`YAML OK`（.github/**/*.yml + .goreleaser.yaml + Taskfile.yml 全部可解析）
- `go run github.com/goreleaser/goreleaser/v2@v2.18.1 check`：
  `1 configuration file(s) validated` ✅
- `go build ./...` — 通過；`go vet ./...` — 通過
- `go tool golangci-lint run` — `0 issues.`
- `go test -shuffle=on ./...` — `ok github.com/cwchiu/my-cli/cmd/my-cli 0.748s`
- `git status --short` — 僅 Taskfile.yml / release.yml / .goreleaser.yaml（+README/specs），go.mod/go.sum 未動 ✅
- **平台矩陣實測**（`release --snapshot --skip=publish --clean`，5m42s 成功）：build 目標
  `linux_amd64_v1`、`linux_arm64_v8.0`、`darwin_amd64_v1`、`darwin_arm64_v8.0`、
  `windows_amd64_v1`、`windows_arm64_v8.0` 全數通過，`dist/` 實際產出 6 個封存檔 + `checksums.txt`：

  | 產物 | 大小 |
  |---|---|
  | `my-cli_Linux_x86_64.tar.gz` | 2531 KB |
  | `my-cli_Linux_arm64.tar.gz` | 2293 KB |
  | `my-cli_Darwin_x86_64.tar.gz` | 2573 KB |
  | `my-cli_Darwin_arm64.tar.gz` | 2378 KB |
  | `my-cli_Windows_x86_64.zip` | 2633 KB |
  | `my-cli_Windows_arm64.zip` | 2363 KB |

  `checksums.txt` 含 6 條 SHA256，與產物一一對應；`dist/` 已由 `.gitignore:5 dist/` 排除。
- **使用者確認**：產出矩陣維持 amd64（x86-64）+ arm64，**不加 32 位元 `386`**。
  理由：macOS 10.15+ 無法執行 32 位元二進位、Go 1.21+ 亦不再官方支援 darwin/386。
- 遠端驗證（合併後）：PR CI 全綠；發布 workflow 待首次 tag 時驗證（人工 checkpoint）。
