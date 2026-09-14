# Issue #21: translate-file

## Why
Source GitHub Issue #21: add a CLI command to translate text files from a source language into Traditional Chinese while fitting the project's existing Cobra patterns and keeping dependencies minimal.

## What
- Implement a new Cobra subcommand: `translate-file`
- Accept exactly one UTF-8 plain-text input file
- Support a configurable DeepLX-compatible API endpoint
- Translate source text with DeepLX target language `ZH` and present it as Traditional Chinese output
- Write a bilingual source/Chinese pair to stdout, with `--output json` for scripts
- Keep the implementation self-contained and avoid third-party dependencies

## How
1. Add a new Cobra subcommand under the main CLI entry structure.
2. Read one bounded source file (10 MiB maximum) and reject invalid UTF-8.
3. Send DeepLX-compatible JSON (`text`, `source_lang: auto`, `target_lang: ZH`) to `--endpoint`, defaulting to `http://localhost:1188/translate`.
4. Validate HTTP status and JSON response before rendering source and translation in table or JSON output.
5. Cover request payload, bilingual output, JSON output, missing files, HTTP errors, timeouts, and invalid UTF-8 through local `httptest` servers.

## 遭遇的困難
- 需要與現有 Cobra CLI 架構保持一致，不破壞既有命令模式。
- 輸入內容可能是純文字，需確保不額外引入依賴或複雜解析流程。
- DeepLX 端點需要可配置，以便在不同環境中切換。
- 需要在輸出時明確保留原文與繁體中文譯文的雙語需求，且不能讓任意檔案耗盡記憶體。

## 如何解決
- 依照專案目前的 Cobra 子命令設計新增 `translate-file`，以 `cobra.ExactArgs(1)` 驗證輸入。
- 使用 Go 標準庫讀取、UTF-8 驗證與 HTTP client，避免第三方函式庫及憑證設定。
- 以 `--endpoint` 讓 DeepLX 相容端點在不同環境切換；預設只連到本機服務。
- 將輸出規格定義為 `Source` 和 `Chinese` 區段，另提供 JSON 供管線處理。
- 限制輸入與回應皆為 10 MiB，並驗證失敗狀態、空結果與逾時錯誤。

## 最後變動了什麼
- `cmd/my-cli/translate_file.go`：新增 DeepLX 相容翻譯命令、UTF-8/大小驗證、HTTP 請求與雙語 JSON/table 輸出。
- `cmd/my-cli/translate_file_test.go`：新增 command、JSON、使用方式、HTTP、timeout、讀檔與 UTF-8 測試。
- `cmd/my-cli/root.go`：註冊 `translate-file` 子命令。
- `README.md`、`CHANGELOG.md`：補充使用方式與未發布變更。
- 本文件：記錄 Issue #21 的決策與驗證。
- Commit: `e795f2fc16fe10af294e4732c8c160f12cb27e8b` (`feat: add text file translation command`)。

## 驗證結果
- `go test -shuffle=on -run 'TestTranslateFile' ./cmd/my-cli`：通過。
- `go build ./...`：通過。
- `go vet ./...`：通過。
- `go tool golangci-lint run`：通過，0 issues。
- `go test -shuffle=on ./...`：通過。
