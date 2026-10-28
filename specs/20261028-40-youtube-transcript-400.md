# Issue #40: fix YouTube transcript HTTP 400

## Why

Source GitHub Issue #40: a real request for a public video returned HTTP 400 even though the mock-based command tests passed.

## What

- Replace the rejected `get_transcript` request with transcript acquisition through YouTube caption tracks.
- Fall back from the Android player client to iOS and then the watch page.
- Keep HTTP error details bounded and reject caption URLs whose origin differs from the player endpoint.
- Add regression tests and verify transcript retrieval from public videos.

## How

1. Reproduce the failure and inspect the API error response.
2. Probe Android/iOS player responses and the caption-track download formats against the reported video.
3. Use the Android player response to select a language track and download its `json3` captions; fall back to iOS and the watch page if needed.
4. Add tests for player requests, iOS/watch-page fallback, JSON3 parsing, and origin validation.
5. Run build, vet, lint, tests, security scans, and live CLI checks.

## 遭遇的困難

- 舊 `get_transcript` endpoint 對影片 `eHS0WIWNtu0` 回傳 `400 FAILED_PRECONDITION`（`Precondition check failed.`），即使影片頁及字幕軌可存取。
- 專案釘選的 golangci-lint v2.13.2 無法解碼 Go 1.27.3 的 export data。
- `task security:all` 中的 OSV 掃描發現既有 `github.com/go-jose/go-jose/v4@v4.1.4` High 漏洞，修復版為 v4.1.6；因此安全任務在 gosec 前停止。
- 單獨執行 gosec v2.29.0 時，Go 1.27.3 型別資訊錯誤導致部分套件跳過 SSA 分析。

## 如何解決

- 以 Android player API v20.10.38 / iOS player API v20.10.4 讀取可用字幕軌，並以字幕軌的 `json3` URL 下載字幕。兩種 player client 均對回報影片成功，返回非空 JSON3；Android 實測 1,332 行。
- 保留 bounded HTTP error preview；字幕 URL 必須與 player endpoint 使用相同 scheme/host，避免將外部回應中的 URL 當作任意請求目的地。
- 使用相容 Go 1.27.3 的 golangci-lint v2.15.0，lint 0 issues。依賴掃描結果是本次工作原先已存在的問題，未混入 YouTube 修復。

## 最後變動了什麼

- `cmd/my-cli/youtube_transcript.go`：以 Android/iOS player API 及字幕軌 JSON3 下載取代舊 `get_transcript` API；增加 watch-page fallback、字幕解析與 origin 驗證。
- `cmd/my-cli/youtube_transcript_test.go`：更新 CLI fixture，涵蓋 JSON3、iOS/watch-page fallback、HTTP error diagnostics 及 origin 驗證。
- Commit：`ee2f2af2ce954c80d8fb586fa15a51d1d54dc0ed`；分支尚未推送，未建立 PR、未合併。

## 驗證結果

- `go build ./...`: 通過。
- `go vet ./...`: 通過。
- `go test -shuffle=on ./...`: 通過。
- golangci-lint v2.15.0 `run`: `0 issues`。
- `govulncheck ./...`: `No vulnerabilities found.`
- OSV：未通過；回報既有 go-jose v4.1.4 的 1 項 High 漏洞（fixed v4.1.6）。
- gosec v2.29.0：指令 exit 0，但部分套件因 Go 1.27.3 型別資訊錯誤跳過 SSA 分析，不能視為完整掃描通過。
- 實際 CLI 對 `eHS0WIWNtu0` 成功取得 1,332 行英文字幕；對 `dQw4w9WgXcQ` 成功取得 61 行英文字幕。
- 以預設文字輸出再次執行 `eHS0WIWNtu0`：1,332 行，首行 `[00:00:00.000] Welcome to this beginner-friendly Claude`，末行 `[00:56:49.320] for watching.`。
- 使用者本機驗證及 commit / PR / merge 尚待處理；不會在使用者驗證前合併。
