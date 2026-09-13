# specs/20260912-add-http-static-server.md

> 工作項：新增 `http-static-server` 子命令——以 `http.FileServer` 提供指定資料夾的靜態檔案服務。
> 狀態：進行中。

## Why

使用者需要一個輕量的靜態檔案伺服器，用於：

1. **本機開發 / 驗證**：快速起一個 HTTP server 提供資料夾內容（如導出的 CSV / JSON 報表、靜態網頁），供瀏覽器或下游工具存取。
2. **臨時分享**：區網內臨時提供檔案下載點。

現有 `my-cli` 指令皆為一次性執行；本工作項引入**第一個長駐型指令**，同時建立兩個專案級基礎建設：

- **signal 優雅關閉**（`signal.NotifyContext`）——AGENTS.md §3 硬性要求，codebase 尚無。
- **slog 結構化日誌**——AGENTS.md §3 規範 stderr 日誌，codebase 尚無。

## What

### 目標
- 新增子命令 `http-static-server`：以 `http.FileServer` 服務指定資料夾。
- Flags：`--listen`（預設 `127.0.0.1`）、`--port`（預設 `8080`）、`--folder`（必填）。
- Env fallback：`MYCLI_HTTP_STATIC_SERVER_LISTEN/PORT/FOLDER`（flag 優先）。
- 請求日誌：slog 每請求一行到 stderr（method、path、status、duration、remote）。
- 優雅關閉：Ctrl-C / SIGTERM → `http.Server.Shutdown`（5 秒等待）。
- 安全預設：只綁 `127.0.0.1`；對外需明確 `--listen 0.0.0.0`。

### 非目標
- 不做 Basic Auth / TLS / URL 前綴（日後需要再加）。
- 不做上傳、WebDAV、快取控制等進階功能。
- 不引入新第三方依賴（純標準庫）。
- 不做 daemon 化（前景執行，Ctrl-C 結束）。

## How

### 技術選型
- `http.FileServer` + `http.Dir`：標準行為——有 `index.html` 顯示它，否則列出目錄。
- `http.Server{ReadHeaderTimeout: 10s}`：**必須**明確設定，否則 gosec G114（`http.ListenAndServe` without timeouts）擋下。
- `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`：`ListenAndServe` 跑 goroutine，主流程 `<-ctx.Done()` → `Shutdown(5s)`。
- `resolveHttpStaticServerConfig`（範本：`falcon_cis.go` 的 `resolveFalconCisConfig`）：flag → env → 驗證（folder 存在且為目錄、port 1–65535、listen 可被 `net.ParseIP` 解析）；錯誤在 RunE 包 `errUsage` → exit code 2；**驗證先於任何 I/O**。
- slog handler：`slog.New(slog.NewTextHandler(os.Stderr, nil))`；logging middleware 記錄 method/path/status/duration/remote。

### 檔案結構
```
cmd/my-cli/
├── http_static_server.go        # newHttpStaticServerCmd + resolve + middleware + serve
└── http_static_server_test.go   # 錯誤分類 + 實際 listen 測試
specs/20260912-add-http-static-server.md   # 本檔
```

### 關鍵設計決策
1. **安全預設**：`--listen` 預設 `127.0.0.1`——區網內不會意外暴露檔案；對外需明確指定。
2. **port 0 支援**：`--port 0` 讓 OS 配置隨機 port，測試用（啟動後 slog 印出實際位址）。
3. **stdout 不使用**：本指令無資料輸出；啟動/關閉/請求訊息全走 slog（stderr）。
4. **測試併發安全**：middleware 記錄請求到共享狀態必須加鎖（nexus data race 教訓，PR #8 實例）。

## 遭遇的困難

1. **revive var-naming**：函式名 `newHttpStaticServerCmd` / `resolveHttpStaticServerConfig` 被要求 `HTTP` 全大寫（`newHTTPStaticServerCmd`）；第一次改名漏掉一處呼叫點，lint 出現 `undefined: resolveHttpStaticServerConfig` 編譯錯誤。
2. **noctx**：`net.Listen` 與 `http.Get` 皆被擋，要求 context 版本（`net.ListenConfig.Listen(ctx, ...)`、`http.NewRequestWithContext` + `Client.Do`）。
3. **gosec G703**（taint-analysis path traversal）：`os.Stat(folder)` 被標記——folder 來自使用者輸入。
4. **測試 hang（最嚴重）**：`TestHTTPStaticServerConfigFromEnv` 直接以 `executeCommand`（`context.Background()`）啟動 server，沒有任何取消路徑——goroutine 永遠停在 `srv.Serve`，`go test` 600 秒逾時（goroutine dump 證實 server goroutine 仍在 Accept）。
5. **平行子測試時序**：修好 #4 後 `TestHTTPStaticServerServesFiles` 反而失敗——`cancel()` 寫在父測試函式尾端，但 Go 的平行子測試在**父函式 return 後**才恢復執行，server 在子測試發請求前就被關掉了（日誌顯示 "shutting down" 先於子測試請求）。
6. **lint 尾聲**：`--folder` 字面量出現 6 次觸發 goconst；`fmt.Sprint(port)` 觸發 perfsprint；`listener.Addr().(*net.TCPAddr)` 型別斷言觸發 errcheck（check-type-assertions）。
7. **PowerShell 管線遮蔽**：`go test ... | Select-Object -Last 30` 會緩衝全部輸出，逾時期間完全看不到進度，誤判為無輸出。
8. **CI gosec 與本機 golangci-lint 的 gosec 版本不一致（PR #12 首輪 checks 失敗）**：本機 `golangci-lint run` 0 issues，但 CI 的 `securego/gosec@v2.29.0` check run 失敗——`//nolint:gosec` 是 golangci-lint 的抑制語法，**gosec 本體不認得**，只認 `#nosec` 註解；G703（taint-analysis path traversal）在 CI 上照報。
9. **rebase 衝突**：ip-lookup（PR #14）先合併進 main 後，本分支 rebase 時 `root.go`（命令註冊順序）與 `README.md`（Commands 表 + 小節順序）衝突。

## 如何解決

1. 全部呼叫點一次改齊（函式定義 + root.go 註冊 + doc comments）；型別名 `httpStaticServerConfig` 小寫開頭不受 revive 影響，維持原樣。
2. 測試輔助函式統一走 context 版本：`httpGet(t, url)`（`NewRequestWithContext(t.Context(), ...)` + `DefaultClient.Do`）、`reserveFreePort` 改用 `net.ListenConfig.Listen(t.Context(), ...)`。
3. `//nolint:gosec // G703: user-selected folder is the feature, not an injection sink` + 上方說明註解（nolintlint 要求指名 linter 並附理由）。
4. 新增 `executeHTTPStaticServer(ctx, args...)`：`newRootCmd()` + `SetContext(ctx)` + `Execute()`（root_test.go 的 `SetContext(t.Context())` 慣例延伸）；`ConfigFromEnv` 改為 `context.WithCancel(t.Context())` + goroutine + `waitForHTTPStaticServer` 輪詢 + `cancel()` + `<-cmdErr`。測試一律加 `-timeout 60s` 讓未來的 hang 自動中斷並吐 goroutine dump。
5. 關閉邏輯移入 `t.Cleanup`：Cleanup 在**所有平行子測試完成後**執行，時序正確；並加註解說明原因。
6. `--folder` 抽成 `argFlagFolder` 常數；`fmt.Sprint` → `strconv.Itoa`；型別斷言改 `addr, ok := ...; require.True(t, ok)`。
7. 診斷時不用管線，直接跑裸指令或重導檔案；逾時改用 `-timeout` 參數而非 shell timeout。
8. 抑制語法改為 gosec 原生格式：`// #nosec G703 -- user-selected folder is the feature, not an injection sink`（gosec 與 golangci-lint 都認得；`#nosec` 帶 rule ID 與理由也符合 gosec `-nosec-require-rules`/`-nosec-require-justification` 的嚴格模式）。本機以 `go run github.com/securego/gosec/v2/cmd/gosec@v2.29.0 ./...` 重現 CI 結果驗證（Issues: 0）。教訓：**本機 lint 綠 ≠ CI SAST 綠**，兩者掃描器不同，提交前應以 CI 同版本 gosec 驗證。
9. rebase 衝突手動解決：`root.go` 兩個命令都保留（`newIPLookupCmd()` 在前、`newHTTPStaticServerCmd()` 在後）；`README.md` 兩個小節都保留（ip-lookup 在前、http-static-server 在後），解完後四關重跑全綠。

## 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `cmd/my-cli/http_static_server.go` | 新增 |
| `cmd/my-cli/http_static_server_test.go` | 新增 |
| `cmd/my-cli/root.go` | 註冊 `newHttpStaticServerCmd()` |
| `README.md` | Commands 表 + 專屬小節 |
| `CHANGELOG.md` | `[Unreleased]` Added 條目 |
| `specs/20260912-add-http-static-server.md` | 本檔 |

## 驗證結果

- `go build ./...`：通過（無輸出）。
- `go vet ./...`：通過（無輸出）。
- `go tool golangci-lint run`：**0 issues**（歷經 11 → 0：noctx×6、goconst、errcheck、perfsprint×2、gofumpt）。
- `go test -shuffle=on -timeout 120s ./...`：全綠——`cmd/my-cli`、`internal/falconcis`、`internal/nexus` 全部 `ok`。
- `go tool task security:all`：通過——govulncheck 0 漏洞、osv-scanner 0 漏洞（298 packages）、gosec 完成（SARIF 產出後清除）。
- 手動驗證：實際起 server（`--folder <temp> --port 18123`）→ `GET /hello.txt` 回 200 內容正確、`GET /missing.txt` 回 404；slog 存取日誌（method/path/status/duration/remote）正確輸出到 stderr；usage 錯誤（缺 folder、port abc）exit code 2。Ctrl-C 優雅關閉路徑由單元測試覆蓋（`"shutting down" grace=5s` + `srv.Shutdown` 無誤差返回）；Windows 終端環境無法從外部對子程序送 console signal，故未做互動式 Ctrl-C 實測。

## PR 後續修正（2026-09-13）

- **CI gosec check run 失敗**（PR #12 首輪 checks）：`//nolint:gosec` 僅對 golangci-lint 有效，CI 的 `securego/gosec@v2.29.0` 不認得，G703 照報（failure annotation）。改為 `// #nosec G703 -- 理由` 後，本機以 CI 同版本 gosec 驗證 `Issues: 0`。
- **rebase onto main**（ip-lookup 先合併）：解決 `root.go` 與 `README.md` 衝突（兩邊內容都保留），修正 rebase 帶入的縮排問題（gofumpt/wsl_v5），四關重跑全綠（build/vet 無輸出、lint 0 issues、test 三套件 ok、security:all govulncheck 0 + osv-scanner 0/298 + gosec Issues: 0）。
- **CodeQL `go/log-injection` alert（line 248，medium）**：存取日誌記錄 `r.URL.Path`（使用者可控）。評估：路徑僅寫入本機 stderr 的 slog 結構化欄位（非 SQL/shell/HTML sink），且 `http.FileServer` 已拒絕路徑正規化失敗的請求；不構成可利用注入，屬可接受風險，不修改程式碼。CodeQL check run 結論為 success，不阻擋合併。
