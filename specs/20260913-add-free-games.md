# specs/20260913-add-free-games.md

> 工作項：新增 `free-games` 子命令——收集限時免費遊戲（GamerPower API，GitHub issue #11）。
> 狀態：完成（待 PR 合併）。

## Why

GitHub issue #11「feat: 限時免費遊戲收集」：使用者想用一條命令掌握
Steam / Epic / Android 平台目前的限時免費遊戲，包含遊戲名稱、限時免費網址、
限時區間，並支援 table 與 csv 輸出（機器可讀、可排程彙整）。

## What

### 目標
- 新增子命令 `free-games`：查詢 GamerPower giveaways API，過濾出
  Steam / Epic Games Store / Android 平台、type=Game 的限時免費遊戲。
- 輸出欄位：遊戲名稱（title）、限時免費網址（open_giveaway_url）、
  限時區間（published_date → end_date）。
- 輸出格式：`--output table|json|csv`（預設 table）；`--out/-O` 寫檔。
- `--platform steam|epic|android` 可複選（逗號或重複 flag），預設三平台全查。
- `--base-url`（測試注入點）、`--timeout`（預設 30s）。

### 非目標
- 不做 DLC / Early Access / Loot 類型（type=Game 之外）。
- 不做 iOS / Battle.net / GOG / itch.io 等其他平台。
- 不引入新第三方依賴（stdlib：net/http、encoding/json、encoding/csv、text/tabwriter）。

## How

### Source of Truth（docs/api-integration-guidelines.md §1，實測 curl）

`GET https://www.gamerpower.com/api/giveaways`（無參數，99 筆 Active）：

```json
{
  "id": 1552,
  "title": "Alone With You (Mobile) Giveaway",
  "worth": "$4.99",
  "thumbnail": "https://www.gamerpower.com/thumb/...",
  "image": "https://www.gamerpower.com/thumb/...",
  "description": "...",
  "instructions": "...",
  "open_giveaway_url": "https://www.gamerpower.com/open/alone-with-you-mobile-giveaway",
  "published_date": "2026-09-10 19:02:45",
  "type": "Game",
  "platforms": "PC, Android, iOS",
  "end_date": "2026-09-17 19:02:45",
  "users": 123,
  "status": "Active",
  "gamerpower_url": "https://www.gamerpower.com/giveaway/...",
  "open_giveaway": "https://www.gamerpower.com/open/alone-with-you-mobile-giveaway"
}
```

實測行為（`$env:TEMP\gp-*.json` 留檔）：
- `platform=steam` / `epic-games-store` / `android` 單平台參數可用；
  **逗號多平台語法（官方文件宣稱）實測 404**：
  `{"status":0,"status_message":"No category found, please check the correct parameters."}`
- 3 次單平台呼叫的聯集 == 單次全量 + client 端過濾（ID diff 為空，實測 5 筆相同）
  → 依 guidelines §4.1 採單次呼叫 + client 端過濾（1 個請求）。
- `end_date` 可能為 `"N/A"`（80/99 筆，無結束時間）；其餘為
  `yyyy-MM-dd HH:mm:ss`（實測 0 筆解析失敗）。`published_date` 同格式。
- **API 會保留過期活動**（status 仍 "Active"，最舊滯留 5 年）→ client 端依
  end_date 過濾已過期條目（N/A 視為未過期保留）。
- `type` 分布：DLC 74、Game 20、Early Access 4、Other 1；type=Game + 三平台
  子字串比對 = 5 筆（即 issue 目標集合）。
- 平台子字串比對安全：無 SteamOS / Android-X 誤報。

### 技術選型
- 複製 `ip-lookup`（單一 HTTP GET、errUsage 分類、httptest）與
  `nexus-repo-export`（CSV writer、`--out/-O`、lossless JSON）既有模式。
- 排序：end_date 升序（急迫者優先，N/A 最後），tiebreak 用 title。

### 遭遇的困難

1. **`cobra.StringSliceP` 簽名錯誤**：初次實作用了 `StringSliceP(name, shorthand, value, usage)`，
   但該函式第一個參數是 `*cobra.Command`——正確的是 `StringSliceVarP(&slice, name, ...)`。
   build 關卡當場抓到（`cannot use ... as *pflag.FlagSet value`）。
2. **gofumpt 多行 `fmt.Errorf` 規則**：多行呼叫的收尾 `)` 必須獨立一行；
   `go tool golangci-lint fmt` 不會修這個，需以
   `go run mvdan.cc/gofumpt@latest -extra -d <file>` 診斷後手動修正。
3. **28 個 `unused` lint 問題**：命令檔完成但尚未註冊進 root command 時，
   所有 exported/unexported 符號都被判 unused——屬預期中間狀態，
   註冊 `rootCmd.AddCommand(newFreeGamesCmd())` 後歸零。
4. **測試 fixture 依賴時鐘**：初版 fixture 日期用 2026-09（當下「未來」），
   時間一過測試就會 rot。修正為時鐘無關設計：存活條目 end_date 用 2099
   （永不過期）、過期條目用 2001（永遠過期），命令層測試不需注入時鐘。
5. **CSV 期望字串錯誤**：初版期望整行被引號包住；實際 `encoding/csv` 只對
   含逗號的欄位加引號（RFC 4180）——title 無逗號不引、platforms 欄位引號。
   以實際輸出修正期望：`Astral Ascent Giveaway,"PC, Epic Games Store",...`。
6. **JSON 數量期望錯誤**：fixture 有 4 筆存活 Game（Astral Ascent、
   Alone With You、Dwarven Realms、GamerPower Mobile App），初版誤寫 3。
7. **dupl 重複 helper**：`startFreeGamesTestServer` 與 `ip_lookup_test.go` 的
   `startIPInfoTestServer` 完全重複（16 行）。合併為共用
   `startJSONTestServer`（定義在 ip_lookup_test.go，free_games_test.go 直接用）。
8. **funlen 121 行超限**：`TestFreeGamesCommand` 超過 120 行上限。
   以迴圈壓縮重複的 header 斷言、內聯 `first` 變數解決。
9. **goconst / testifylint / wsl_v5**：字面值 `"steam"` 重複 4 次改用
   `platformSteam` 常數；float 比較改 `is.InEpsilon`；`is.Greater` 前補空行。
10. **pwsh 批次取代後殘留重複宣告**：用 `-replace` 批次改名 helper 時，
    兩個檔案各自留下同名 `startJSONTestServer` 定義（vet 報 redeclared），
    刪除 free_games_test.go 的重複定義與未用的 `httptest` import 解決。

### 如何解決

- 每個問題都由四關（build → vet → lint → test）當場抓出、當場修：
  簽名錯誤由 build、redeclared/unused import 由 vet、
  dupl/funlen/goconst/testifylint/wsl_v5/gofumpt 由 lint、
  CSV/JSON 期望由 test 的實際輸出對照修正。
- 時鐘依賴問題以「2099 永遠存活 / 2001 永遠過期」的 fixture 設計根治，
  並在 payload 註解與 `executeCommand` 呼叫處說明設計意圖。
- 工具層教訓：`multi_replace_string_in_file` 對大型 payload 連續失敗 3 次
  （工具驗證錯誤），改用小型單次 `replace_string_in_file` 逐一套用即成功。

### 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `cmd/my-cli/free_games.go` | 新增：`free-games` 子命令（flags、config 解析、平台正規化、HTTP fetch、收集/排序、table/json/csv 渲染） |
| `cmd/my-cli/root.go` | 註冊 `newFreeGamesCmd()` |
| `cmd/my-cli/free_games_test.go` | 新增：命令層（table/platform/csv/json）、單一請求、錯誤分類、平台正規化、collector、comparator 測試；時鐘無關 fixture |
| `cmd/my-cli/ip_lookup_test.go` | `startIPInfoTestServer` 改名共用 `startJSONTestServer`（消除 dupl） |
| `README.md` | Commands 表加入 `free-games`；新增專屬小節（flags 表、平台過濾說明、三種輸出格式） |
| `CHANGELOG.md` | `[Unreleased]` Added 加入 `free-games` 條目 |
| `specs/20260913-add-free-games.md` | 本檔 |

### 驗證結果

- `go build ./...`：無輸出（通過）。
- `go vet ./...`：無輸出（通過）。
- `go tool golangci-lint run`：**0 issues**。
- `go test -shuffle=on ./...`：三套件全 ok
  （cmd/my-cli、internal/falconcis、internal/nexus）。
- `go tool task security:all`：govulncheck 0 vulnerabilities、
  osv-scanner 0/298 packages affected、gosec Issues: 0。
- Live smoke test（真實 GamerPower API）：
  - `free-games`（table）：4 筆（Alone With You、Astral Ascent、Luftrausers、
    Dwarven Realms N/A），欄位與排序符合設計。
  - `-p steam,epic -o csv`：header + 3 筆，platforms 欄位正確引號。
  - `-o json`：完整保留 API 欄位（worth/thumbnail/description/instructions 等）。
  - `--help`：Short/Long/Example/Flags 完整。
