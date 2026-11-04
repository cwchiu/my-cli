# free-games 支援更多免費遊戲資訊

- **Issue**: [#54](https://github.com/cwchiu/my-cli/issues/54)
- **Worktree**: `../my-cli-worktrees/53-free-games-more-info`
- **Branch**: `feat/53-free-games-more-info`
- **日期**: 2026-11-02

## Why

現有 `free-games` 子命令只輸出 `Title / Platforms / URL / Start / End` 五個欄位，且平台選擇器只有
`steam`、`epic`、`android` 三種。使用者參考
[FreeKeep-for-Steam ARCHITECTURE.md](https://github.com/SmailDot/FreeKeep-for-Steam/blob/main/docs/ARCHITECTURE.md)
的多來源（Steam / Epic / GOG）架構後，詢問「是否可支援更多 free game 資訊」。

GamerPower 的 `/api/giveaways` 回應其實已經帶有 `worth`（原價）、`description`、`thumbnail`、
`image`、`users`（領取人數）、`gamerpower_url`、`instructions` 等欄位，但目前的實作把它們丟掉了。
同時，實際資料中出現 `GOG`、`Itch.io`、`DRM-Free` 等平台 token，現有選擇器完全無法篩選，
導致 `PC, DRM-Free`（IndieGala）與 `PC, GOG, DRM-Free`（GOG）類的贈品永遠被過濾掉。

因此本次工作的目標是：**在不新增第三方依賴、不改動既有輸出契約的前提下，把 API 已有的資訊
暴露出來，並補上缺少的平台選擇器與排序／篩選能力。**

## What

### 範圍（In scope）

1. **擴充資料欄位**：`freeGameEntry` / `freeGame` 增加 `worth`、`description`、`thumbnail`、
   `image`、`users`、`gamerpower_url`、`instructions`。
2. **新增平台選擇器**：`gog`、`itch`、`drm-free`（對應 API token `GOG`、`Itch.io`、`DRM-Free`）。
3. **新增 `--type` 篩選**：`game`（預設）| `dlc` | `early-access` | `other` | `all`。
4. **新增 `--min-worth` 篩選**：只保留原價 ≥ 指定金額的贈品（`N/A` 視為 0）。
5. **新增 `--sort`**：`end`（預設）| `worth` | `published` | `title`。
6. **新增 `--details`**：table 模式額外顯示 `WORTH`、`USERS`、`DESCRIPTION`（截斷）欄位。
7. **CSV 欄位擴充**：`Title,Platforms,Type,Worth,Users,URL,Start,End`。
8. **JSON 維持無損**：仍輸出未經修改的 API entry，不新增也不刪除欄位。

### 非目標（Out of scope）

- 不新增 Steam / Epic / GOG 官方 API 的直接整合（FreeKeep 的多來源抓取）。
  理由：那需要各自的認證、速率限制與反爬處理，屬於另一個工作項；GamerPower 已聚合這些來源。
- 不實作「自動領取（claim）」流程。
- 不新增任何第三方依賴。
- 不改動 `--output json` 的既有語意（無損輸出）。

## How

### Source of Truth

依 `docs/api-integration-guidelines.md`，以**真實 API 回應**為唯一依據。本次擷取
`https://www.gamerpower.com/api/giveaways` 的即時回應（126 筆）作為證據：

**完整 entry schema（實測）**

```json
{
  "id": 3835,
  "title": "A New Beginning: Final Cut (GOG) Giveaway",
  "worth": "$9.99",
  "thumbnail": "https://www.gamerpower.com/offers/1b_anewbeginningfinalcut.jpg",
  "image": "https://www.gamerpower.com/offers/1_anewbeginningfinalcut.jpg",
  "description": "A New Beginning: Final Cut is a ...",
  "instructions": "1. Click the button to ...\r\n2. ...",
  "open_giveaway_url": "https://www.gamerpower.com/open/a-new-beginning-final-cut",
  "published_date": "2026-10-30 12:00:00",
  "type": "Game",
  "platforms": "PC, GOG, DRM-Free",
  "end_date": "2026-11-05 23:59:59",
  "users": 12345,
  "status": "Active",
  "gamerpower_url": "https://www.gamerpower.com/a-new-beginning-final-cut-gog-giveaway",
  "open_giveaway": "https://www.gamerpower.com/open/a-new-beginning-final-cut"
}
```

**type 分佈（126 筆）**：`DLC` 83、`Early Access` 6、`Game` 36、`Other` 1。

**platforms token 分佈（126 筆）**：`PC` 123、`Steam` 31、`DRM-Free` 25、`Itch.io` 16、
`Playstation 4` 13、`Android` 12、`Playstation 5` 12、`Xbox One` 12、`iOS` 11、
`Xbox Series X|S` 9、`Epic Games Store` 8、`Nintendo Switch` 7、`GOG` 1、`VR` 1、`Xbox 360` 1。

**worth 格式**：`"$6.99"`、`"$384.00"`、`"$0.99"`、`"$10.00"` 或 `"N/A"`（126 筆中 82 筆為 `N/A`）。
無千分位逗號、無非 USD 幣別。36 筆 `Game` 全部有數值 worth。

**users**：整數，實測範圍 1550–690710。

**description**：長度 56–1880 字元（平均約 196）；`instructions` 為多行，換行使用 `\r\n`。

**Game 類型實例（節錄）**

| id | worth | platforms | title |
|---|---|---|---|
| 3839 | $6.99 | PC, Epic Games Store | Epic World - Hunt For Quest (Epic Games) Giveaway |
| 3836 | $9.99 | PC, Steam | Asleep: Act 1 (Steam) Giveaway |
| 3835 | $9.99 | PC, GOG, DRM-Free | A New Beginning: Final Cut (GOG) Giveaway |
| 3830 | $384.00 | PC, Itch.io, DRM-Free | Free itch.io Games Bundle |
| 3346 | $4.99 | PC, DRM-Free | My Darkest Witch (IndieGala) Giveaway |
| 3446 | $24.99 | PC | GigaBash (Stove) Giveaway |
| 3251 | $1.99 | VR | Sharknado VR (Rift) Giveaway |

> 注意：`PC, DRM-Free`（IndieGala）與 `PC`（Stove）在現行選擇器下完全無法命中，
> 這是新增 `drm-free` 選擇器的直接動機。

### 設計決策

1. **`worth` 解析**：新增 `parseFreeGameWorth(string) (float64, bool)`，去除 `$` 與空白後
   `strconv.ParseFloat`；`"N/A"`、空字串、解析失敗一律回 `(0, false)`。保留原始字串
   `worth` 供顯示，數值 `worthValue` 供排序與 `--min-worth` 比較。
2. **平台選擇器以「token 子字串」比對**（沿用現有機制）。新增的 `GOG`、`Itch.io`、`DRM-Free`
   在實測資料中不與其他 token 互為子字串，安全。
3. **`--type` 對應**：`game`→`Game`、`dlc`→`DLC`、`early-access`→`Early Access`、
   `other`→`Other`、`all`→不過濾。預設 `game` 維持既有行為。
4. **`--sort`**：`end`（既有行為，無結束時間排最後）、`worth`（高→低，`N/A` 視為 0 排最後）、
   `published`（新→舊）、`title`（A→Z）。所有排序都以 title 作為 tie-breaker，確保輸出穩定。
5. **`--details`**：僅影響 table 輸出；CSV 欄位固定擴充（不因 `--details` 改變 schema），
   JSON 不受影響。
6. **description 截斷**：table 模式截斷至 80 字元並以 `...` 結尾，避免表格爆版；
   完整內容仍可從 `--output json` 取得。

## 遭遇的困難

1. **主 repo 有非預期變更**：`README.md`（刪 26 行）與 `cmd/my-cli/root.go`
   （少了 `rootCmd.AddCommand(newYouTubeTranscriptCmd())`）出現未提交的刪除，是前一工作項
   cwd 漂移留下的污染。
2. **Issue 編號與 worktree 名稱不一致**：worktree 依慣例命名為 `53-free-games-more-info`，
   但 `gh issue create` 實際配發 **#54**。
3. **終端指令被工具簡化**：帶 `Set-Location <path>;` 前綴的指令被剝除前綴後執行，
   一度出現 `git 無法辨識` 的錯誤。
4. **重複執行同一條分析指令**：連續多次執行相同的 PowerShell 列舉指令，浪費大量 context
   預算，且沒有取得任何新資訊。
5. **`worth` 不是純數字**：API 回傳的是帶 `$` 的字串或 `"N/A"`，不能直接反序列化成 `float64`。

## 如何解決

1. 以 `git checkout -- README.md cmd/my-cli/root.go` 還原主 repo，確認 `git status --short`
   只剩預期的 untracked 檔案（`.tmp/`、`nexus.json`）。
2. 以實際 Issue 編號 **#54** 作為 spec 檔名與 PR 的 `Closes #54`；worktree 目錄名保留
   `53-free-games-more-info`（僅為目錄名，不影響可稽核性，spec 內已明確記錄對應 Issue）。
3. 移除 `Set-Location` 前綴後重跑指令（cwd 本來就正確）。
4. 停止重複列舉；第一次成功輸出即為完整資料，後續直接進入設計與實作。
5. `worth` 以字串欄位接收，另以 `parseFreeGameWorth` 轉出 `(float64, bool)`，
   顯示用原字串、比較用數值，兩者分離。

## 最後變動了什麼

| 檔案 | 變更摘要 |
|---|---|
| `cmd/my-cli/free_games.go` | 擴充 entry/game 欄位；新增 `gog`/`itch`/`drm-free` 選擇器；新增 `--type`、`--min-worth`、`--sort`、`--details`；CSV 欄位擴充；table 欄位擴充 |
| `cmd/my-cli/free_games_test.go` | fixture 補上 `worth`/`description`/`users`/`thumbnail`/`gamerpower_url`；新增 `parseFreeGameWorth`、`--type`、`--min-worth`、`--sort`、`--details`、GOG 平台、CSV header 測試 |
| `specs/20261102-54-free-games-more-info.md` | 本紀錄 |

## 驗證結果

四關全綠（worktree `D:\tmp\my-cli-worktrees\53-free-games-more-info`）：

| 關卡 | 指令 | 結果 |
|---|---|---|
| build | `go build ./...` | 無輸出（成功） |
| vet | `go vet ./...` | 無輸出（成功） |
| lint | `go tool golangci-lint run` | `0 issues.` |
| test | `go test -shuffle=on ./...` | 全部 `ok` |

測試輸出：

```
ok      github.com/cwchiu/my-cli/cmd/my-cli     3.316s
ok      github.com/cwchiu/my-cli/internal/certinfo      3.787s
ok      github.com/cwchiu/my-cli/internal/falconcis     1.551s
ok      github.com/cwchiu/my-cli/internal/nexus 4.566s
```

### 過程中修掉的 lint 問題

1. `funlen`：`TestFreeGamesCommand` 223 行 > 120。拆成 `TestFreeGamesCommand`（表格/平台）、
   `TestFreeGamesCommandOutput`（csv/json）、`TestFreeGamesCommandFilters`（type/min-worth/sort/details）
   三個函式，共用 `freeGamesCase` 型別與 `runFreeGamesCases` 執行器。
2. `goconst`：`"Pricey Game"` 出現 4 次。抽出常數 `priceyGameTitle`。
3. `revive`（unused-parameter）：`runFreeGamesCases` 的 `baseURL` 參數未使用。移除該參數。

### 過程中修掉的測試 bug

`TestParseFreeGameWorth` 的 `N/A` / 空字串 / 垃圾值案例使用 `assert.InEpsilon`，
因 expected 為 0 而報 `expected value must have a value other than zero to calculate the
relative error`。改用 `assert.InDelta`（可處理 expected 為 0 的情況）。
