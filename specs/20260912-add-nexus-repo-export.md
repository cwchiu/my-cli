# specs/20260912-add-nexus-repo-export.md

> 工作項：新增 `nexus-repo-export` 子命令——導出 Sonatype Nexus Repository 3 的 repository 設定為 CSV / JSON。
> 狀態：已完成（待 PR / 合併）。

## Why

使用者需要將 Nexus Repository 3 上的 repository 設定導出，用途：

1. **備份 / 稽核**：定期導出 repo 設定快照，供 diff 比對與留存。
2. **重建（rebuild）**：JSON 導出必須**足夠詳細以重建 repo**——保留 API 回應全部欄位（lossless），並記錄 GET→POST 對照關係。
3. **報表**：CSV 扁平格式供試算表 / 管理報告使用。

現有 `my-cli` 已有 `falcon-cis-export`（CrowdStrike Falcon CIS 導出）成熟模式，本工作項複製其架構，零新依賴。

## What

### 目標
- 新增子命令 `nexus-repo-export`：呼叫 `GET /service/rest/v1/repositories`（NXRM3 REST v1），導出全部 repository 設定。
- 輸出格式：`--output table|json|csv`（預設 table）；`-O/--out` 導出到檔案。
- 認證：Basic Auth（`--username` flag + `NEXUS_PASSWORD` env；匿名可讀時可省略）。
- 過濾：`--format`（如 maven/npm/docker）、`--type`（hosted/proxy/group）客端過濾。
- JSON 為 **lossless** 原始回應（`json.RawMessage`），足夠重建 repo。

### 非目標
- 不導出 repository 內容/元件（僅設定）。
- 不支援建立/修改 repository（唯讀導出）。
- 不支援 Nexus Repository 2。
- 不引入新第三方依賴。
- 不做 user token / Bearer token 認證。

## How

### 技術選型
- 完全複製 `falcon-cis-export` 模式：`cmd/my-cli/falcon_cis.go`（命令）+ `internal/falconcis/`（client 套件）。
- 標準庫：`net/http`、`encoding/json`、`encoding/csv`、`text/tabwriter`、`sort`。
- 無分頁（`GET /v1/repositories` 一次回傳全部）；無 token cache（Basic Auth 每請求帶入）；比 falconcis 簡化。

### 檔案結構
```
internal/nexus/
├── client.go          # Client struct + NewClient + getRepositories（Basic Auth、錯誤處理）
├── repositories.go    # Repository struct、ListRepositories（排序）、ListRepositoriesRaw（lossless）、flattenAttributes
├── client_test.go
└── repositories_test.go
cmd/my-cli/
├── nexus_export.go    # newNexusExportCmd + resolveNexusConfig + render 三格式
└── nexus_export_test.go
specs/20260912-add-nexus-repo-export.md  # 本檔
```

### 關鍵設計決策
1. **JSON lossless（可重建）**：`ListRepositoriesRaw(ctx) (json.RawMessage, error)` 直接回傳原始 body，**不經固定 struct**（避免靜默丟未知欄位）。struct 僅服務 CSV/table。
2. **CSV 自訂扁平 schema（24 欄）**：無真實 Web 匯出檔可對齊，依 `docs/api-integration-guidelines.md` 攤平 API 回應；缺欄輸出空字串；依 Name 排序（穩定可 diff）；必用 `encoding/csv`（RFC 4180）。
3. **密碼安全**：`NEXUS_PASSWORD` env（`#nosec G101` 註解）；絕不做 flag；不進 log/error。
4. **GET→POST 對照表**（重建用，記錄於本檔）：
   - GET 回應的 `url`、`version` 為計算欄位，重建（`POST /v1/repositories/{format}/{type}`）時不需送回。
   - `format`、`type` 在 POST 是 URL 路徑參數，不在 body。
   - 部分 format 屬性名有差異（待真實環境驗證補充）。
   - routing rules 在部分 NXRM3 版本不在 GET 回應中——Phase 5 驗證把關，必要時補抓 `GET /v1/routing-rules` 併入 JSON。

### CSV 欄位（24 欄）
`Name,Format,Type,URL,Version,ContainsComponent,Exposed,Online,WritePolicy,StorageBlobStoreName,StorageStrictContentTypeValidation,StorageQuotaType,StorageQuotaLimit,CleanupPolicyNames,HTTPClientBlocked,HTTPClientAutoBlock,HTTPClientRetries,ProxyRemoteURL,ProxyContentMaxAge,ProxyMetadataMaxAge,NegativeCacheEnabled,NegativeCacheTimeToLive,PositiveCacheEnabled,PositiveCacheTimeToLive`

攤平來源：`attributes.{storage,httpclient,proxy,negativeCache,positiveCache,cleanup}.*`；陣列以 `;` 串接。

## 遭遇的困難

1. **測試 helper 未定義**：`nexus_export_test.go` 初版呼叫了未定義的 `newNexusTestHandler`，cmd 測試無法編譯。
2. **testifylint 誤用**：`assert.True(t, ...)` 在 `assert.New(t)` 已綁定 `t` 的情況下多傳了 `t`（編譯錯誤）；另有多處 error 斷言應用 `require` 而非 `assert`（testifylint require-error）。
3. **`t.Setenv` 與 `t.Parallel` 衝突**：`TestNexusRepoExportBasicAuthFromEnv` 同時使用兩者，Go testing 直接 panic（"test using t.Setenv ... can not use t.Parallel"）。
4. **命令未註冊**：初版忘了在 `root.go` 的 `newRootCmd()` 加入 `newNexusExportCmd()`，所有 cmd 測試報 `unknown command "nexus-repo-export"`。
5. **usage error 分類**：測試期望 flag/env 驗證錯誤（缺 `--base-url`、非法 `--output`/`--type`）映射到 exit code 2（`errUsage`），但 (a) `resolveNexusConfig` 的錯誤未包 `errUsage`；(b) `MarkFlagRequired` 的錯誤發生在 `RunE` 之前，無法在命令內包裝。
6. **lint 首跑 20 issues**：gofumpt 格式（5）、goconst 重複字面量（5）、gosec G304（1）、testifylint（8）、wsl_v5（1）。
7. **`task` 不在 PATH**：`task security:all` 直接執行失敗（PowerShell 找不到 task 執行檔）。
8. **CI data race（PR #8 "Test (Go stable)" 失敗）**：本地四關全綠，但 CI 的 `go test -race` 偵測到 `nexus_export_test.go:72` 的 DATA RACE——`TestNexusRepoExportCommand` 在父層共用一個 test server，5 個 `t.Parallel()` 子測試併發打同一個 server；httptest 每個請求在獨立 goroutine 處理，handler 內 `*gotAuth = append(*gotAuth, ...)` 對同一 slice 做 read-modify-write。本地測不出是因為本機無 gcc 跑不了 `-race`（AGENTS.md §5 約定）。race 錯誤計數是全域的，導致同時段執行的 `TestVersionCommand`、`TestOpenaiChatTestTimeout` 也被歸因失敗（實際只有一個根因）。

## 如何解決

1. 補上 `newNexusTestHandler`（httptest.NewServer 包裝，記錄 Authorization header），並補 `net/http`、`net/http/httptest` imports。
2. 移除多餘的 `t` 參數（`assert.New(t)` 風格下直接 `assert.True(...)`）；error 斷言改 `require.Error/NoError(t, err)`；`assert.Equal("", x)` 改 `assert.Empty(x)`；排序比較改 `is.Less(a, b)`。
3. 移除該測試的 `t.Parallel()`（`t.Setenv` 不可並行），留註解說明。
4. 在 `root.go` 註冊 `rootCmd.AddCommand(newNexusExportCmd())`。
5. `resolveNexusConfig` 的回傳錯誤在 `RunE` 內以 `fmt.Errorf("%w: %w", errUsage, err)` 包裝；移除 `MarkFlagRequired("base-url")`，改由 `resolveNexusConfig` 統一驗證（錯誤訊息不變、可包裝、help 文字標注 required）。
6. `go tool golangci-lint fmt` 自動修 gofumpt；重複字面量抽常數（`argNexusExport`、`argFlagBaseURL`、`argFlagOutput`、`sectionStorage`）；`os.ReadFile` 加 `// #nosec G304 -- path comes from t.TempDir()`；wsl_v5 補空行。
7. go.mod 的 `tool` directive 已含 task：改用 `go tool task security:all`。
8. 新增 `authRecorder`（`sync.Mutex` 保護的 recorder，`record`/`snapshot` 方法）取代裸 slice append；httptest handler goroutine 併發寫入必須加鎖。`internal/nexus` 的測試 server 為每子測試各自建立、單請求後才讀取（HTTP round-trip happens-before 已同步），無需修改。

## 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `internal/nexus/client.go` | 新增：`Client`、`NewClient`、`get`（Basic Auth、401/非 2xx 錯誤）、`ListRepositoriesRaw`（lossless）、`truncateBody` |
| `internal/nexus/repositories.go` | 新增：`Repository` struct、`ListRepositories`（decode + 依 Name 排序）、`attributeValue`/`formatAttributeValue` |
| `internal/nexus/repositories_test.go` | 新增：table-driven 測試（成功/排序/401/500/invalid json）、Basic Auth/匿名 header 驗證、lossless 驗證、`TestAttributeValue` |
| `cmd/my-cli/nexus_export.go` | 新增：`newNexusExportCmd`（flags/completion）、`resolveNexusConfig`（flag→env、驗證包 errUsage）、`filterNexusRepositories`、`renderNexusRawJSON`/`renderNexusTable`/`renderNexusCSV`（24 欄）、`nexusCSVRow`/`nexusAttributeValue` |
| `cmd/my-cli/nexus_export_test.go` | 新增：命令層測試（table/json/csv/format/type filter、`-O` 檔案輸出、usage error 分類、env Basic Auth） |
| `cmd/my-cli/root.go` | 註冊 `newNexusExportCmd()` |
| `README.md` | Commands 表加入 `nexus-repo-export`；新增專屬小節（flags 表、憑證安全說明、三種輸出格式、JSON 可重建說明） |
| `CHANGELOG.md` | 新增 `[Unreleased]` 條目（nexus-repo-export 功能清單） |
| `specs/20260912-add-nexus-repo-export.md` | 本檔 |

（commit hash 於 PR 建立後補上）

## 驗證結果

- `go build ./...`：通過。
- `go vet ./...`：通過。
- `go tool golangci-lint run`：**0 issues**。
- `go test -shuffle=on ./...`：`ok cmd/my-cli`、`ok internal/falconcis`、`ok internal/nexus` 全綠。
- `go tool task security:all`：govulncheck "No vulnerabilities found"；osv-scanner "0 packages affected by 0 known vulnerabilities"；gosec SARIF 產出無阻擋。
- **CI（PR #8）**：首輪 "Test (Go stable)" 因 data race 失敗（見遭遇的困難 #8）；修復後重跑四關全綠，push 後 `gh pr checks` 全數通過。

### 可重建性驗證（Phase 5 #13，強制）
- 對真實 Nexus 跑 `--output json`，逐項比對 Nexus UI 各 repo 全部設定（routing rule、docker httpPort、maven versionPolicy/layoutPolicy、cleanup、blob store 等）。
- 若有缺口 → 補抓輔助端點併入 JSON。
- （待真實環境驗證後填入結果）
