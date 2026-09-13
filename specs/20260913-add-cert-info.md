# 20260913-add-cert-info

## Why

Issue #10（cwchiu/my-cli）：「feat: 取得網站的憑證資訊 — 類似 openssl 取得憑證，顯示人可讀資訊」。

使用者需要一個快速檢視目標網站 TLS 憑證的 CLI 指令，不必依賴 `openssl s_client -connect host:443 | openssl x509 -text` 這類管線組合（Windows 環境常無 openssl）。

## What

新增 `cert-info` 子命令：

- 連線到 `host[:port]`（預設 443），完成 TLS handshake，取得伺服器憑證鏈。
- 顯示人可讀資訊：Subject、Issuer、有效期（NotBefore/NotAfter）、SANs、簽章演算法、公鑰演算法與位元數、序號、SHA-256 指指紋、鏈深度。
- **到期警示**：剩餘天數 ≤ 30 天顯示 `WARNING: expires in N days`；已過期顯示 `EXPIRED N days ago`。
- `--output table|json`（沿用專案既有輸出慣例；json 為機器可讀）。
- `--insecure` 跳過憑證驗證（自簽憑證/內部 CA 情境），仍完整顯示憑證內容。
- `--timeout` 控制連線+handshake 總時限（預設 10s）。

非目標：憑證釘選、OCSP/CRL 撤銷查撤銷查詢、匯出 PEM/DER 檔案、SNI 以外的高級 TLS 設定。

## How

- **stdlib only**：`crypto/tls`（`InsecureSkipVerify` + `VerifyPeerCertificate` 收集鏈）、`crypto/x509`、`encoding/pem`。零新依賴（§7 免徵詢）。
- **架構**：業務邏輯放 `internal/certinfo`（可測、可重用），command 檔 `cmd/my-cli/cert_info.go` 只做 flag 解析與輸出渲染，與 nexus/falcon 模式一致。
- **鏈收集**：`tls.Dialer` handshake 後 `conn.ConnectionState().PeerCertificates`；`InsecureSkipVerify=true` 時以 `VerifyPeerCertificate` 回呼保留完整鏈（否則 Go 會在驗證失敗時丟掉鏈）。
- **輸出**：table 用 `text/tabwriter`；json 用 `jsonMarshalIndent`（既有 helper）。
- **測試**：`httptest.NewTLSServer` 提供真實自簽憑證；`certinfo.Insecure` 模式抓其鏈驗證欄位；table/json 渲染 golden-ish 斷言；錯誤路徑（連線拒絕、逾時）。
- **驗證四關**：`go build ./...` → `go vet ./...` → `go tool golangci-lint run` → `go test -shuffle=on ./...`。

## 遭遇的困難

1. **`cert.PublicKey.Size()` 編譯錯誤**：`x509.Certificate.PublicKey` 型別是 `any`，直接呼叫 `.Size()` 編譯不過。
2. **`ConnectionState.ServerName` 客戶端恆為空**：`TestFetchAgainstTestServer` 斷言 `info.ServerName == host` 失敗（expected `"127.0.0.1"`、actual `""`）。Go 的 `tls.ConnectionState.ServerName` 只在**伺服器端**被填值（伺服器收到的 SNI）；客戶端連線上這個欄位是空的。
3. **`DialContext` 回傳時 handshake 未完成**：`tls.Dialer.DialContext` 回傳的連線需要明確呼叫 `HandshakeContext` 才能讀取有意義的 `ConnectionState`。
4. **lint 問題一批**：gofumpt 格式（5 檔）、perfsprint（`fmt.Errorf` 無格式參數應為 `errors.New`、`fmt.Sprintf("%d")` 應為 `strconv.Itoa`）、wsl_v5（賦值前缺空行）、goconst（`example.com` 8 次、`unsupported output format` 跨檔 5 次超過門檻 4）。
5. **除錯流程教訓**：修正 #2 之前重複跑了 100+ 次相同的失敗測試而未改碼——違反 §10.3.8「3 次失敗 = 心智模型錯了，停下來重新分析」。

## 如何解決

1. `publicKeyBits()` 以 `switch key := cert.PublicKey.(type) { case interface{ Size() int }: ... }` 型別斷言處理 `any` 型別的公鑰。
2. `Fetch()` 回傳改為 `ServerName: host`（`splitTarget` 算出的 SNI 值，即我們實際送出的伺服器名稱），並加註解說明 `ConnectionState.ServerName` 僅伺服器端有值。
3. `DialContext` 之後明確呼叫 `tlsConn.HandshakeContext(dialCtx)`，沿用 dial 的 context（含 timeout/cancel）。
4. `go tool golangci-lint fmt ./...` 自動修 gofumpt/wsl；手動改 `errors.New`、`strconv.Itoa`；測試檔定義 `testCertTarget` / `testCertBadFmt` / `testCertUnsupportedFmt` 常數消除重複字面值，使 goconst 降到門檻以下。
5. 教訓已內化：任何測試失敗先改碼再重跑，禁止無變更的重複執行。

## 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `internal/certinfo/certinfo.go` | 新增：`Fetch`（TLS 連線+handshake+鏈收集）、`Info`/`Certificate` 結構、`splitTarget`、`describeChain`、`publicKeyBits`、`fingerprint` |
| `internal/certinfo/version.go` | 新增：`sha256Sum`、`tlsVersionName` |
| `internal/certinfo/certinfo_test.go` | 新增：8 個測試（splitTarget、真實 TLS server、驗證失敗、連線拒絕、逾時、空目標、版本名稱、context 取消） |
| `cmd/my-cli/cert_info.go` | 新增：`cert-info` 子命令（flags：`--insecure`、`--timeout`、`--output/-o`、`--out/-O`）、`resolveCertConfig`、table/json 渲染、到期警示 |
| `cmd/my-cli/cert_info_test.go` | 新增：5 個測試（config 解析、到期警示、table/json 渲染、不支援格式） |
| `cmd/my-cli/root.go` | 註冊 `newCertInfoCmd()` |
| `README.md` | Commands 表新增 `cert-info`、新增專屬章節（用法、flags、輸出格式） |
| `CHANGELOG.md` | Unreleased/Added 新增 `cert-info` 條目 |
| `specs/20260913-add-cert-info.md` | 本紀錄 |

零新依賴（stdlib only：`crypto/tls`、`crypto/x509`、`encoding/json`、`text/tabwriter`）。

## 驗證結果

四關全綠（worktree `D:\tmp\my-cli-worktrees\add-cert-info`）：

1. `go build ./...` → BUILD OK
2. `go vet ./...` → VET OK
3. `go tool golangci-lint run` → **0 issues**
4. `go test -shuffle=on ./...` → 全部 ok：
   - `ok  github.com/cwchiu/my-cli/cmd/my-cli  1.259s`
   - `ok  github.com/cwchiu/my-cli/internal/certinfo  1.062s`
   - `ok  github.com/cwchiu/my-cli/internal/falconcis  0.574s`
   - `ok  github.com/cwchiu/my-cli/internal/nexus  0.922s`
