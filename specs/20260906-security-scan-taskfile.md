# 20260906 — security-scan-taskfile

## Why

CI 的 Security workflow 出現兩個問題：

1. **osv-scanner job 失敗**：報告 1 個 High 漏洞（`Total 1 package affected by 1 known vulnerability (0 Critical, 1 High, ...)`，
   「1 vulnerability can be fixed.」）。使用者要求先在本地掃描解決後才 merge/push。
2. **大量 tar 錯誤噪音**：`govulncheck` job 日誌出現大量
   `Error: /usr/bin/tar: ../../../go/pkg/mod/golang.org/x/sys@v0.47.0/...: Cannot open: File exists`。

另外使用者要求把掃描整併到 Taskfile，讓本地與 CI 行為一致。

## What

- 本地重現並識別漏洞，升級依賴修復（必要時修正 toolchain）。
- 調查 CI tar 錯誤根因並消除。
- Taskfile 新增 `security:govulncheck` / `security:osv` / `security:all`。
- 非目標：不改 gosec / CodeQL job；不調整掃描頻率與觸發條件。

## How

1. **本地掃描**（worktree 內，直接 `go run ...@version`，不動 go.mod 依賴）：
   - `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
   - `go run github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.5.1 scan --lockfile=go.mod`
2. **識別結果**：
   - osv-scanner（對齊 CI）：`google.golang.org/grpc@1.83.0` → GHSA-vp52-pcj8-j9qc
     （gRPC-Go HTTP/2 DATA 碎片化 OOM，severity 8.7 / High），修復版本 1.83.1。
   - govulncheck（符號級）：本地 Go 1.26.0 標準库 2 個漏洞：
     - GO-2026-5037 `crypto/x509`（fixed in go1.26.4）
     - GO-2026-4601 `net/url`（fixed in go1.26.1）
3. **修復**：
   - `go get google.golang.org/grpc@v1.83.1` + `go mod tidy`（diff 僅 2 行：grpc 版本與 hash）。
   - `go mod edit -toolchain=go1.26.8`：go.mod 增加 `toolchain go1.26.8`，本地與 CI 均以 1.26.8 編譯，
     標準库漏洞隨 toolchain 修復。`go` 欄位維持 `1.26.0`（最低語言版本不變，僅提升實際工具鏈）。
4. **tar 噪音根因**（取自 run 34028287547 完整日誌）：
   - 錯誤全部出自 **govulncheck job** 的 actions/cache 步驟：`golang/govulncheck-action@v1`
     內建的 Go module cache 還原在快取已存在的檔案上再次解壓 tar，`##[error]` 標記使其顯示為錯誤；
     job 結論仍為 success。
   - 修法：丟掉該 action，改為直接 `run: go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
     （與本地 task 行為一致，且不再碰 module cache 還原）。
   - osv-scanner job 的失敗與 tar 無關，就是漏洞本身（reporter `--fail-on-vuln=true`）；
     本地升級後 CI 應轉綠。
5. **Taskfile**：新增三個 security task（見 How-3 的指令），與 CI 的掃描器版本/參數對齊。

## 遭遇的困難

1. **osv-scanner v2 本地執行**：直接 `go run github.com/google/osv-scanner/cmd/osv-scanner@v2.5.1`
   找不到模組路徑；v2 的 module path 帶 `/v2`，正確為
   `github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.5.1`。
2. **`go mod edit -toolchain=go1.26.8` 在 PowerShell 炸裂**：未加引號時被拆成
   `go: open .26.8: The system cannot find the file specified.`。
3. **task --list 中文亂碼**：pwsh 終端碼問題，task 本身執行正常（實跑驗證通過）。

## 如何解決

1. 改用帶 `/v2` 的 module path；與 CI 釘同一版本 v2.5.1，保證本地/CI 一致。
2. 參數加引號：`go mod edit "-toolchain=go1.26.8"`。
3. 以 `go tool task security:osv` 實跑輸出為準驗證，不依賴 list 顯示。

## 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `go.mod` | `google.golang.org/grpc` v1.83.0 → v1.83.1（修 GHSA-vp52-pcj8-j9qc / High）；新增 `toolchain go1.26.8`（修 GO-2026-5037、GO-2026-4601） |
| `go.sum` | grpc v1.83.1 校驗行 |
| `.github/workflows/security.yml` | govulncheck job 改用 `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`，移除 `golang/govulncheck-action@v1`（消除 tar 噪音），附原因註解 |
| `Taskfile.yml` | 新增 `security:govulncheck`、`security:osv`、`security:all` |
| `specs/20260906-security-scan-taskfile.md` | 本紀錄 |

## 驗證結果

- 本地 govulncheck（toolchain 1.26.8 後）：`No vulnerabilities found.`（exit 0）
- 本地 osv-scanner v2.5.1：`Total 0 packages affected by 0 known vulnerabilities`
- `go build ./...`：BUILD OK
- `go vet ./...`：VET OK
- `go tool golangci-lint run`：`0 issues.`
- `go test -shuffle=on ./...`：ok（cmd/my-cli 0.721s）
- YAML 第五關：`YAML OK`（含 Taskfile.yml）
- `go tool task security:osv`：實跑成功，0 漏洞
- `go mod tidy` 後 diff 僅含本工作項預期變更（grpc、toolchain）
