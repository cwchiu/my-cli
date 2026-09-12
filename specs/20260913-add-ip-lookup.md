# specs/20260913-add-ip-lookup.md

> 工作項：新增 `ip-lookup` 子命令——透過 ipinfo.io 取得外部 IP 資訊（GitHub issue #9）。
> 狀態：完成（待使用者確認後合併）。

## Why

GitHub issue #9「feat: 取得外部ip資訊」建議整合 ipinfo.io。CLI 使用者常需快速確認
目前對外 IP 與其地理/ASN 資訊（除錯網路、確認 VPN 出口、填寫防火牆規則），
`my-cli ip-lookup` 提供零設定、免 token 的單一查詢入口。

## What

### 目標
- 新增子命令 `ip-lookup`：查詢自己的對外 IP（無參數）或指定 IP（一個位置參數）。
- 資料來源：`GET https://ipinfo.io/json`（免 token；實測驗證，見 How）。
- 輸出：`--output table|json`（預設 table）；table 為 key-value 對齊排版，
  json 為 lossless 原始回應（`json.RawMessage`，沿用 nexus 模式）。
- `--base-url`（預設 `https://ipinfo.io`，同時作為測試注入點）、`--timeout`（預設 30s）。

### 非目標
- 不做 token 輸入（legacy 端點免 token；Lite API 需 token 但欄位較少，不採用）。
- 不做 Lite API、batch API、欄位過濾（`/country`）、v4/v6 強制端點。
- 不引入新第三方依賴。

## How

### Source of Truth（docs/api-integration-guidelines.md §1，實測 curl）

`GET https://ipinfo.io/json`（查自己）與 `GET https://ipinfo.io/8.8.8.8/json`（查任意 IP）：

```json
{
  "ip": "36.239.107.27",
  "hostname": "36-239-107-27.dynamic-ip.hinet.net",
  "city": "Kaohsiung",
  "region": "Takao",
  "country": "TW",
  "loc": "22.6163,120.3133",
  "org": "AS3462 Data Communication Business Group",
  "timezone": "Asia/Taipei",
  "readme": "https://ipinfo.io/missingauth"
}
```

選填欄位（實測 8.8.8.8）：`postal`、`anycast`（boolean）。
錯誤回應（403/429，實測 invalid token）：

```json
{
  "status": 403,
  "error": { "title": "Unknown token", "message": "..." }
}
```

### 技術選型
- 完全複製 `openai-chat-test`（單一 HTTP 請求、table|json）與 `nexus-repo-export`
  （lossless JSON、errUsage 分類、httptest 測試）的既有模式。
- 標準庫：`net/http`、`encoding/json`、`text/tabwriter`、`net`（IP 驗證）。
- JSON 重排重用同 package 的 `renderNexusRawJSON`（行為一致，避免重複）。

### 檔案結構
```
cmd/my-cli/
├── ip_lookup.go        # newIPLookupCmd + resolveIPLookupConfig + fetch/render
├── ip_lookup_test.go   # table-driven + httptest（全 t.Parallel，無 env 依賴）
└── root.go             # 註冊 newIPLookupCmd()
```

### 關鍵設計決策
1. **免 token**：使用者確認「免費取 ip 資訊服務不應該需要 token」；實測 legacy
   `ipinfo.io/json` 確實免 token。不設任何憑證輸入，也無 G101 顧慮。
2. **table 輸出固定欄序**：`ip/hostname/city/region/country/loc/org/postal/timezone`，
   空欄位與 `readme` 跳過；`anycast` 僅在 true 時顯示。
3. **位置參數驗證**：0 或 1 個；有值必須通過 `net.ParseIP`，否則包 `errUsage` → exit 2。
4. **錯誤處理**：解析結構化 error envelope（403/429）；解析失敗退回截斷 body
   （`maxErrorBodyLen` 模式）；429 給額度用盡的明確訊息。
5. **測試無 env 依賴**：免 token 設計讓全部測試可 `t.Parallel()`，避開
   `t.Setenv` × `t.Parallel` 衝突（nexus 工作項教訓 #3）與 data race 風險（教訓 #8）。

## 遭遇的困難

1. **`truncateBody` undefined（build 失敗）**：原想在錯誤 fallback 重用
   `internal/nexus` 的 `truncateBody`，但它在 internal 套件，`cmd/my-cli` 無法 import。
2. **lint 首跑 12 issues**：gofumpt ×2（const block 對齊）、wsl_v5 ×1
   （`targetIP := ""` 後缺空行）、testifylint require-error ×1、goconst ×8
   （`"--base-url"`、`"xml"`、`"unsupported output format"` 等字面值重複）。
3. **`argFlagBaseURL` redeclared（typecheck 錯誤擋住 lint）**：我在
   `ip_lookup_test.go` 重新宣告了 `nexus_export_test.go` 已有的 `argFlagBaseURL`。
4. **table 測試期望與 tabwriter 實際對齊不符（test 失敗）**：我預期 key 欄寬為
   最寬 key + 1 空格，實際 tabwriter 以最寬 key（`hostname:`）+ padding 2 對齊，
   `ip:` 後是 8 個空格而非 7 個。

## 如何解決

1. 在 `ip_lookup.go` 新增本地 `truncateDetail`（重用同 package
   `openai_chat.go` 的 `maxErrorBodyLen` 常數），不跨 internal 邊界。
2. `go tool golangci-lint fmt` 修 gofumpt；手動補空行修 wsl_v5；
   `require.ErrorContains` 合併 Error+Contains 修 testifylint；
   字面值抽成 package 級測試常數（`argIPLookup`/`argOutputXML`/`wantBadFormat`），
   `"--base-url"` 直接重用 nexus 測試的 `argFlagBaseURL` 修 goconst。
3. 移除 `ip_lookup_test.go` 的重複宣告，直接使用 `nexus_export_test.go`
   的常數（同 package 測試檔共用）。
4. 以實際輸出為準修正測試期望字串（`ip:        8.8.8.8`、`ip:  36.239.107.27`），
   並加註解說明 tabwriter 對齊規則。

## 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `cmd/my-cli/ip_lookup.go` | 新增 |
| `cmd/my-cli/ip_lookup_test.go` | 新增 |
| `cmd/my-cli/root.go` | 註冊命令 |
| `README.md` / `CHANGELOG.md` | 文件 |
| `specs/20260913-add-ip-lookup.md` | 本檔 |

## 驗證結果

四關（worktree 內，2026-09-13）：

- `go build ./...`：通過（無輸出）。
- `go vet ./...`：通過（無輸出）。
- `go tool golangci-lint run`：**0 issues**。
- `go test -shuffle=on ./...`：三套件全 ok
  （`cmd/my-cli` 1.338s、`internal/falconcis` 0.487s、`internal/nexus` 0.864s）。

安全掃描 `go tool task security:all`：

- govulncheck：No vulnerabilities found。
- osv-scanner（go.mod，298 packages）：0 known vulnerabilities。
- gosec（SARIF）：掃描完成，無新增 finding。

> 本機無 gcc，未跑 `-race`（AGENTS.md §5；`-race` 由 CI Linux runner 執行）。
