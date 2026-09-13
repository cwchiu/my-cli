# specs/20260913-add-password-gen.md

> 工作項：新增 `password-gen` 子命令——本地生成隨機密碼（GitHub issue #15）。
> 狀態：進行中。

## Why

GitHub issue #15「feat: 生成隨機密碼」要求 CLI 提供隨機密碼生成，參數規格：
長度（預設 16）、數字（預設開）、大寫（預設開）、小寫（預設開）、特殊符號（預設關）。
使用者（腳本、管線、密碼管理器匯入）常需一次性產生符合字元類別要求的密碼，
`my-cli password-gen` 提供零設定、純本地、無網路的單一入口。

## What

### 目標（依 issue #15 使用者回覆定案）

| 項目 | 決議 |
|---|---|
| 子命令名稱 | `password-gen` |
| 符號字元集 | `!@#$%^&*()-_=+[]{};:,.<>?/`（避開 shell 易混淆字元） |
| 每類至少一個 | **採用**——開啟的類別保證各出現至少一次 |
| `--count N` | 不做（單筆） |
| 預設長度 | 16 |

- Flags：`--length`（int，預設 16）、`--no-digits`、`--no-upper`、`--no-lower`、
  `--symbols`、`--output table|json`（預設 table，stdout 單行密碼）。
- 亂數：`crypto/rand`（禁 `math/rand`）；取樣用 rejection sampling 避免 modulo bias。
- 「每類至少一個」：先聯集取樣填滿，再對每個啟用類別各植入一個字元於前 k 位置，
  最後 Fisher-Yates 洗牌（`crypto/rand`）消除位置可預測性。
- 長度下限：啟用類別數（最多 4）；低於下限或全部類別關閉 → usage error（exit 2）。
- 密碼絕不進 log/error。

### 非目標

- 不做 `--count N` 多筆生成（使用者明確回覆不需要）。
- 不做密碼強度評分、passphrase 模式、自訂字元集。
- 不引入新第三方依賴（僅標準庫 `crypto/rand`、`math/big`）。

## How

### 技術選型

- 複製既有子命令模式：`resolveConfig` 驗證包 `errUsage`（exit 2）、
  `--output` completion、table-driven 測試（全 `t.Parallel()`，無 env 依賴）。
- 標準庫：`crypto/rand`、`math/big`、`encoding/json`。

### 檔案結構

```
cmd/my-cli/
├── password_gen.go       # newPasswordGenCmd + resolvePasswordGenConfig + generate
├── password_gen_test.go  # table-driven + 字元集/長度/保證驗證
└── root.go               # 註冊 newPasswordGenCmd()
README.md / CHANGELOG.md  # 文件
specs/20260913-add-password-gen.md  # 本檔
```

### 關鍵設計決策

1. **`--no-*` 關閉開關**：需求 2–4 預設「開」，提供 `--no-digits`/`--no-upper`/`--no-lower`
   讓零參數呼叫即最常用情境；符號預設「關」用正向 `--symbols`。
2. **rejection sampling**：`crypto/rand.Int(rand.Reader, big.NewInt(int64(len(charset))))`
   對聯集字元集均勻取樣，無 modulo bias。
3. **每類至少一個（使用者要求）**：取樣後對每個啟用類別隨機挑一個字元覆寫前 k 位置，
   再 Fisher-Yates 洗牌整串——保證成立且分佈仍均勻。
4. **長度下限 = 啟用類別數**：`--length` 低於下限報 usage error，訊息指出目前啟用類別數。
5. **無 env fallback、無設定檔依賴**：純本地生成，全部測試可 `t.Parallel()`。

## 遭遇的困難

1. **`--output` 與 `json` 成員命名不一致**：最初設計成 `outputJSON`，但其他命令已經統一用 `output`（`table|json`）並用 `outputFormatJSON` 常數，導致編譯錯誤與不一致。
2. **字元集保證同時保留隨機性**：僅「把每類一個字元塞到前面」會造成固定位置模式；需再做 Fisher-Yates shuffle，避免可預測性。
3. **檢查邏輯要防止錯誤的 false positive**：若只檢查「有字元存在」而不驗證「整串只來自啟用類別」，使用者可能拿到不符設定的字串。測試因此要求「所有字元都屬於啟用類別」與「每類至少出現一次」。

## 如何解決

1. 以既有命令的命名慣例為準，將 `passwordGenConfig.output` 命名統一為 `output`，並在 `resolvePasswordGenConfig` 直接判斷 `outputFormatJSON` / `outputFormatTable`，消除型別不一致。
2. 實作 `generatePassword`：先用 `crypto/rand` 與 rejection sampling 組成完整長度字串，再對每個啟用類別補上一個保證字元、最後 `shuffleBytes` 洗牌，兼顧隨機性與規則保證。
3. 測試以真實行為為準：`checkPasswordGenOutput` 會驗證（a）總長度、（b）字元只來自啟用集合、（c）每個啟用集合至少出現一次，避免僅測試「看起來像」密碼而已。

## 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `cmd/my-cli/password_gen.go` | 新增 |
| `cmd/my-cli/password_gen_test.go` | 新增 |
| `cmd/my-cli/root.go` | 註冊命令 |
| `README.md` / `CHANGELOG.md` | 文件 |
| `specs/20260913-add-password-gen.md` | 本檔 |

## 驗證結果

已在 worktree 中驗證（2026-09-13）：

- `go build ./...`：通過（無輸出）。
- `go vet ./...`：通過（無輸出）。
- `go tool golangci-lint run`：**0 issues**。
- `go test -shuffle=on ./...`：全綠，`cmd/my-cli`、`internal/certinfo`、`internal/falconcis`、`internal/nexus` 全部 `ok`。
- `go tool task security:all`：govulncheck 0 vulnerabilities；osv-scanner 0 known vulnerabilities；gosec 扫描完成未新增 finding。

> 本機無 gcc（AGENTS.md §5），未跑 `-race`；CI Linux runner 會補此檢查。
