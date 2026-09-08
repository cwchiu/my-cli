# 20260908-add-openai-chat-test

## Why

使用者需要一個子命令，用於測試 OpenAI 相容的 chat API（如 OpenAI、Azure OpenAI、ollama、vLLM、LM Studio 等相容端點）。目前 `my-cli` 只有 `version` 子命令，缺少實用的業務功能。

需求（使用者原話）：

> 我想要建立一個子命令. 用於測試 openai 相容 chat api, 參數應該會有
> 1. base_api
> 2. model_name
> 3. key (optional)

## What

新增 `openai-chat-test` 子命令，對指定的 OpenAI 相容端點送出一則固定短訊息，驗證端點可用性並顯示回應。

**範圍：**

- 子命令名稱：`openai-chat-test`（使用者指定）
- 參數（皆為 flags，使用者選擇 Flags 形式）：
  - `--base-api`（必填）：API base URL，如 `https://api.openai.com/v1`
  - `--model`（必填）：模型名稱，如 `gpt-4o-mini`
  - `--key`（選填）：API key；未提供時 fallback 讀 `OPENAI_API_KEY` 環境變數；兩者皆無則不送 `Authorization` header（相容 ollama/vLLM 等免金鑰伺服器）
  - `--timeout`（選填，預設 30s）：HTTP 逾時
  - `--output`（選填，預設 table）：`table|json`，與 `version` 子命令一致
- 測試訊息：固定短訊息（`Say 'OK'.`），回應文字印出
- JSON 輸出：完整回應結構（model、回應文字、延遲、token 用量）

**非目標：**

- 不做多輪互動式對話
- 不引入 OpenAI SDK（標準庫 `net/http` + `encoding/json` 足夠，避免新增第三方依賴）
- 不支援 streaming

## How

1. **Worktree**：`git worktree add ../my-cli-worktrees/add-openai-chat-test -b feat/openai-chat-test`（自 `35bce68`）。
2. **檔案**：`cmd/my-cli/openai_chat.go`（命令定義）+ `cmd/my-cli/openai_chat_test.go`（測試）。一個子命令一個檔案（AGENTS.md §2）。
   - **命名修正**：原規劃的 `openai_chat_test.go` 會被 Go 視為測試檔（`_test.go` 後綴），`go build` 會排除它；故實作放 `openai_chat.go`、測試放 `openai_chat_test.go`，同時符合 §5「測試檔與來源檔同名」。
3. **命令結構**（cobra 技能規範）：
   - `RunE`（非 `Run`）；`Args: wrapUsage(cobra.NoArgs)`（沿用 `args.go` 的 exit-code-2 機制）
   - 必填 flags 用 `MarkFlagRequired`；`--output` 沿用 `version.go` 實際模式（`StringVarP` + `RegisterFlagCompletionFunc` + 防禦性 default case）。`version.go` 註解提到的 `MarkFlagCustom` 在 codebase 中不存在（過時註解），未採用
   - 輸出一律 `cmd.OutOrStdout()`；錯誤訊息小寫開頭、`%w` 包裝
   - 設定解析抽成 `resolveChatTestConfig`：驗證 timeout 為正、output format 合法（**在網路 I/O 前 fail-fast**）、trim base URL 尾端 `/`、解析 key
4. **HTTP 客戶端**：標準庫。`POST {base_api}/chat/completions`，body 為 `{"model": ..., "messages": [{"role":"user","content":"Say 'OK'."}]}`；`Authorization: Bearer <key>` 僅在取得 key 時附加。
5. **Key 解析順序**：`--key` flag > `OPENAI_API_KEY` env > 空（不送 header）。金鑰不寫入任何輸出（stdout/stderr/log）。
6. **逾時**：`http.Client{Timeout: ...}`，`--timeout` 為 `time.Duration` flag。
7. **測試**：`httptest.Server` 模擬相容端點（成功、401、500、404、逾時、缺參數、json 輸出、key 不外洩）；table-driven + `t.Parallel()`；每測試重建 command tree（沿用 `executeCommand` helper）。
8. **驗證**：四關（build / vet / lint / test）全綠後 commit、push、開 PR。

## 遭遇的困難

1. **檔名陷阱**：原規劃把實作放 `openai_chat_test.go`——以 `_test.go` 結尾的檔案會被 `go build` 排除，命令根本不會被編進 binary。
2. **`MarkFlagCustom` 不存在**：`version.go` 第 69 行註解寫「Guarded by MarkFlagCustom below」，但全 codebase 搜尋不到該呼叫；實際防護只有 switch 的防禦性 default case。
3. **lint 首輪 10 個問題**：errcheck（`resp.Body.Close` 未檢查）、gosec G101（`OPENAI_API_KEY` 常數誤判為硬編碼憑證）、perfsprint（無格式化的 `fmt.Errorf` 應為 `errors.New`）、gofumpt 格式（2 檔）、goconst（`"json"`、`"{URL}"`、`"gpt-4o-mini"` 重複字面值，含既有 `version_test.go`/`main_test.go`）。
4. **`t.Setenv` 與 `t.Parallel()` 互斥**：env fallback 測試放在 parallel 表格中直接 panic（testing 套件強制：改 process 狀態的測試不可平行）。
5. **output format 驗證時機錯誤**：`--output xml` 的錯誤原本在 HTTP 請求**之後**才回報，測試拿到的是 dial 錯誤而非格式錯誤——驗證應在網路 I/O 前 fail-fast。
6. **timeout 測試拖慢 5 秒**：handler 用 `time.Sleep(5s)` 模擬慢伺服器，`ts.Close()` 會等 handler 結束，整個測試被拖住；且違反 §5「禁止 time.Sleep」。
7. **revive unused-parameter**：timeout 測試的 handler 改寫後 `w` 參數未使用。
8. **gosec G101 在 CI 開 alert，本機 lint 卻 0 issues**：PR 掃描（`refs/pull/4/merge`）由獨立的 `securego/gosec` action（`security.yml` gosec job）執行，它**不讀 `.golangci.yml`、不認 `//nolint` 指令**——`//nolint:gosec` 只被 golangci-lint 的 gosec wrapper 尊重，兩者抑制語意不同（code-scanning alert #6）。
9. **timeout 測試間歇性卡死 10 分鐘（flaky）**：`#nosec` 修改後重跑四關，`TestOpenaiChatTestTimeout` 卡死至 10 分鐘 panic——handler 卡在 `<-r.Context().Done()`，`httptest.Server blocked in Close after 5 seconds`。同程式碼先前 3 次全綠，屬 timing race 掩蓋的設計缺陷。

## 如何解決

1. **檔名**：實作改放 `cmd/my-cli/openai_chat.go`，測試放 `openai_chat_test.go`（同時滿足 §5 同名對應）。
2. **MarkFlagCustom**：跟隨實際存在的模式（`StringVarP` + `RegisterFlagCompletionFunc` + default case），不發明不存在的 API；`version.go` 的過時註解留待日後獨立工作項處理（不在本項混入）。
3. **lint**：
   - errcheck → `defer` 內顯式檢查 `Close` 錯誤（body 已已讀完，錯誤僅標註忽略）
   - gosec G101 → `//nolint:gosec // G101 false positive: this is an environment variable name, not a credential.`（符合 §6 指名 linter + 理由）
   - perfsprint → `errors.New`
   - gofumpt → `go tool golangci-lint fmt cmd/my-cli/...`
   - goconst → 新增 `outputFormatTable`/`outputFormatJSON` 常數並讓 `version_test.go`/`main_test.go` 共用；測試新增 `urlPlaceholder`、`testModel`、`flagBaseAPI`、`flagModel` 常數
4. **t.Setenv**：env fallback 抽成獨立的 `TestOpenaiChatTestKeyFromEnv`（不平行），並附註解說明為何不可 `t.Parallel()`。
5. **fail-fast**：output format 驗證移入 `resolveChatTestConfig`（RunE 一開始就呼叫），錯誤在送出任何請求前回報。
6. **timeout 測試**：handler 改為 `<-r.Context().Done()`（等 client 斷線），client timeout 觸發後 handler 立即返回；測試時間 5s → 0.7s，且不再用 `time.Sleep`。
7. **revive**：未使用參數改為 `_`。
8. **gosec 雙重抑制**：改用 gosec **原生** `#nosec G101` 註解（`// #nosec G101 -- this is an environment variable name, not a credential.`）——gosec 本體原生支援，golangci-lint 的 gosec wrapper 也尊重它，兩套工具一次滿足；nolintlint 只驗證 `//nolint` 指令，`#nosec` 不受 `require-explanation`/`require-specific` 約束。Taskfile 缺 `security:gosec` 本機關卡屬基礎設施缺口，另開工作項補（見 §10.3.1 一次只做一件事）。
9. **flaky 根因（讀 Go 1.26.8 `net/http` 原始碼確認）**：server 端偵測 client 斷線靠 `connReader.backgroundRead`，而它只在 request body 被讀到 EOF（`registerOnHitEOF` → `startBackgroundRead`）後才啟動；timeout 測試的 handler 只等 `r.Context().Done()`、從不讀 body，client 逾時斷線後 server **永遠偵測不到**，context 不會被取消。`httptest.Server.Close()` 只強制關 `StateIdle`/`StateNew` 連線、不碰 `StateActive`（handler 執行中），故 `ts.Close()` 永久阻塞。先前 3 次通過是 race：完整套件下 client 斷線早於 server 讀完 request，`readRequest` 直接失敗、handler 根本沒啟動。**修法**：handler 先 `io.Copy(io.Discard, r.Body)` 把 body 讀到 EOF（啟動 background read），再等 `ctx.Done()`——client 斷線時 `handleReadErrorLocked` → `cancelCtx`，handler 返回、`ts.Close()` 正常結束。修復後單獨跑 3 次全 PASS（0.07s/次）。

## 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `cmd/my-cli/openai_chat.go` | 新增：`openai-chat-test` 子命令（flags 定義、key 解析、HTTP 請求、回應解析、table/json 輸出、錯誤處理含 key 遮蔽）；`envAPIKey` 以原生 `#nosec G101` 抑制誤判 |
| `cmd/my-cli/openai_chat_test.go` | 新增：httptest 模擬端點的完整測試（成功/401/500/404/逾時/缺參數/json 輸出/key 不外洩/env fallback）；timeout 測試 handler 先 drain request body 再等 `ctx.Done()`（修 flaky 卡死） |
| `cmd/my-cli/root.go` | 註冊 `newOpenaiChatTestCmd()`（1 行） |
| `cmd/my-cli/version_test.go` | `"json"` 字面值改用 `outputFormatJSON` 常數（goconst） |
| `cmd/my-cli/main_test.go` | 同上（goconst） |
| `specs/20260908-add-openai-chat-test.md` | 本紀錄 |

Commit：`17410e2`（feat: add openai-chat-test subcommand）；fix commit（`#nosec G101` + timeout 測試 drain body 修 flaky）見 push 後 hash。

## 驗證結果

四關（worktree cwd 執行）：

- `go build ./...`：通過（無輸出）
- `go vet ./...`：通過（無輸出）
- `go tool golangci-lint run`：**0 issues**
- `go test -shuffle=on -count=1 ./...`：`ok github.com/cwchiu/my-cli/cmd/my-cli`（2 個不同 shuffle seed 全綠）

測試明細：13 個子測試全 PASS，含 key 不外洩（`[redacted]`）與逾時（50ms client timeout 正確觸發）案例。

修復後補驗：

- `go test -run 'TestOpenaiChatTestTimeout$' -count=3 -timeout 90s`：3 次全 PASS（0.07s/次；修復前單獨跑必現卡死 91s timeout panic）
- lint 對 `#nosec G101` 註解：0 issues（golangci-lint 的 gosec 尊重原生 `#nosec`；nolintlint 不約束 `#nosec`）
