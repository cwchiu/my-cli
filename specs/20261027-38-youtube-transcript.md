# Issue #38: youtube-transcript

## Why

Source GitHub Issue #38: add a command to retrieve YouTube transcripts, following the fetching approach in the user-provided `youtube-fetcher.ts` reference.

## What

- Add `my-cli youtube-transcript URL`.
- Support `--lang` (default `en`), `--timeout`, and `--output text|json`.
- Fall back to English, then the first available caption language.
- Print transcript segments with start timestamps; include timing and language details in JSON.
- Keep the implementation dependency-free and test network behavior against local HTTP servers.

## How

1. Validate supported YouTube URL hosts and extract an 11-character video ID.
2. Read caption tracks and visitor data from the video page.
3. Use YouTube's transcript endpoint with the Android client payload pattern from the reference.
4. Bound HTTP timeouts and response sizes; decode both WEB and Android transcript segment formats.
5. Add command tests, document the command, and record verification results below.

## 遭遇的困難

1. 在 worktree 執行 CLI package 測試時，Go Proxy 與替代鏡像對未快取的既有依賴回傳 HTTP 403（包含 `golang.org/x/sys`、`golang.org/x/text` 與 `go.yaml.in/yaml/v3`）。
2. 初版命令測試包含錯誤的臨時占位碼；在執行測試時發現並移除，改為有效的 protobuf 參數驗證。
3. Go 1.27.3 下，專案釘選的 golangci-lint 2.13.2 無法解析 Go 1.27 標準庫 export data；較新的 golangci-lint 2.15.0 可分析此 Go 版本。

## 如何解決

1. 確認 `go.mod` 未因本功能變更，且測試程式不需要新增依賴。檢查後確認 `HTTP_PROXY`/`HTTPS_PROXY` 指向 `http://10.51.4.170:443`：經代理請求失敗，單次測試程序以 `NO_PROXY=*` 繞過代理後可下載依賴並執行完整測試；未修改永久代理設定或 Git proxy。
2. 改用標準 `testing` assertions，補上 protobuf 內容及 HTTP 狀態錯誤測試。
3. 保留使用者指定的 Go 1.27.3，使用不修改專案依賴或工具釘選的 golangci-lint 2.15.0 執行 lint，並修正本次程式的 lint 診斷。

## 最後變動了什麼

- `cmd/my-cli/youtube_transcript.go`：新增 URL 驗證、caption 語言 fallback、YouTube transcript API 請求、bounded HTTP 回應、WEB/Android transcript 解析及 text/JSON 輸出。
- `cmd/my-cli/youtube_transcript_test.go`：新增 URL、語言選擇、API 參數、HTTP、輸出及錯誤測試。
- `cmd/my-cli/root.go`：註冊 `youtube-transcript` 子命令。
- `README.md`、`CHANGELOG.md`：新增命令文件及 Unreleased 條目。
- 本文件：記錄 Issue #38 的實作與驗證。
- 功能 commit：`77b7303bce31428d8e27f85e0ac0d375e02127b3`。
- PR：[#39](https://github.com/cwchiu/my-cli/pull/39)。

## 驗證結果

- `gofmt -d cmd/my-cli/youtube_transcript.go cmd/my-cli/youtube_transcript_test.go`：通過。
- `git diff --check`：通過。
- `go build ./...`：Go 1.27.3 下通過。
- `go vet ./...`：Go 1.27.3 下通過。
- `go test -shuffle=on ./...`：Go 1.27.3 下通過；測試程序使用 `NO_PROXY=*` 避免失效代理。
- `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run`：Go 1.27.3 下通過，0 issues；此版本僅供本次驗證，未更動專案工具設定。
- `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`：未發現漏洞。
- `go run github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.5.1 scan --lockfile=go.mod`：掃描 300 個套件，0 個受已知漏洞影響。
- PR #39 checks：Lint、CodeQL、gosec、govulncheck、OSV Scanner、gitleaks、Go 1.26 與 stable 測試全數通過。
