# AGENTS.md — my-cli 專案開發規範

> 本文件是本專案所有 AI Agent 與人類協作者的**強制規範**。
> 目標：多子命令 CLI 工具、符合業界 Go 開發規範、產品等級的安全與品質、每次變更顆粒度小到人可以 review。

---

## 1. 必載技能宣告（最高優先級）

Before any Go coding, review, debugging, troubleshooting, or setup task, load the `samber/cc-skills-golang@golang-how-to` skill first — it routes to whichever other Go skills the task needs.

### Required Go skills

依任務性質載入對應技能（皆為 `samber/cc-skills-golang@` 前綴）：

**核心（任何 Go 任務都可能需要）：**

- `samber/cc-skills-golang@golang-code-style`
- `samber/cc-skills-golang@golang-naming`
- `samber/cc-skills-golang@golang-error-handling`
- `samber/cc-skills-golang@golang-safety`
- `samber/cc-skills-golang@golang-security`
- `samber/cc-skills-golang@golang-testing`
- `samber/cc-skills-golang@golang-modernize`
- `samber/cc-skills-golang@golang-documentation`
- `samber/cc-skills-golang@golang-troubleshooting`

**本專案（CLI）專屬：**

- `samber/cc-skills-golang@golang-cli`
- `samber/cc-skills-golang@golang-spf13-cobra`
- `samber/cc-skills-golang@golang-spf13-viper`
- `samber/cc-skills-golang@golang-lint`
- `samber/cc-skills-golang@golang-continuous-integration`
- `samber/cc-skills-golang@golang-dependency-management`
- `samber/cc-skills-golang@golang-structs-interfaces`
- `samber/cc-skills-golang@golang-context`
- `samber/cc-skills-golang@golang-concurrency`
- `samber/cc-skills-golang@golang-observability`

**按需載入：** `golang-refactoring`（重構）、`golang-performance` / `golang-benchmark`（效能）、`golang-project-layout`（結構調整）、`golang-gopls`（導航/重構工具）、`golang-stretchr-testify`（測試斷言）。

---

## 2. 專案概述與技術選型

| 項目 | 決定 |
|---|---|
| 類型 | 多子命令 CLI 工具 |
| 語言 | Go（使用最新 stable；新代碼直接採用 1.21–1.26 現代化特性） |
| 命令框架 | [spf13/cobra](https://github.com/spf13/cobra) |
| 設定管理 | [spf13/viper](https://github.com/spf13/viper)（分層設定：flag > env > file > default） |
| 日誌 | `log/slog`（結構化；正式環境 JSON） |
| 測試 | 標準 `testing` + [stretchr/testify](https://github.com/stretchr/testify) |
| Lint | golangci-lint v2（設定見 `.golangci.yml`） |
| 安全掃描 | `govulncheck` + `gosec`（lint 內建） |
| 模組路徑 | `github.com/<owner>/my-cli`（小寫、連字號、**必須**與 repo URL 一致） |

> DI 框架：小型 CLI 預設**不引入**；若依賴圖變複雜，先與使用者討論再選型（dig / fx / do / wire）。

### 目錄結構

```
my-cli/
├── AGENTS.md              # 本規範
├── cmd/my-cli/
│   ├── main.go            # 極薄：只呼叫 Execute()
│   ├── root.go            # root command + viper 初始化
│   ├── <subcommand>.go    # 一個子命令一個檔案
│   └── version.go         # 版本資訊（ldflags 注入）
├── internal/              # 私有業務邏輯（不可被外部 import）
├── testdata/              # 測試 fixtures
├── go.mod / go.sum        # go.sum 必須 commit
├── Makefile
├── .gitignore
├── .golangci.yml
└── .github/workflows/     # CI
```

> **變更工作區**：所有開發在 git worktree（`../my-cli-worktrees/<topic>`）進行，主 repo 停在 `main` 只負責合併；流程見 §10。

---

## 3. CLI 專屬規範（Cobra + Viper）

### Cobra

- **一律用 `RunE`**，禁止 `Run`（錯誤要回傳，不要吞掉）。
- Root command 必須設定 `SilenceUsage: true` 與 `SilenceErrors: true`，由 main 統一處理錯誤輸出與 exit code。
- 參數驗證用 `Args:` 欄位（`NoArgs` / `ExactArgs(n)` / `MinimumNArgs` / `MaximumNArgs` / `RangeArgs` / `OnlyValidArgs`，可用 `MatchAll` 組合），**禁止**在 `RunE` 裡手寫 `len(args)` 檢查。
- 設定初始化放在 root 的 `PersistentPreRunE`；注意**子命令的 `PersistentPreRunE` 會取代父層的**，需要時明確呼叫父層版本。
- Hook 執行順序：`PersistentPreRunE → PreRunE → RunE → PostRunE → PersistentPostRunE`；任一 hook 出錯即停止。
- Flag 約束：`MarkFlagRequired` / `MarkFlagsMutuallyExclusive` / `MarkFlagsOneRequired` / `MarkFlagsRequiredTogether`。
- `StringSliceP` 會以逗號切分；`StringArrayP` 不切分——依語意選擇。
- Completion：窮舉列表用 `ValidArgs` + 回傳 `cobra.ShellCompDirectiveNoFileComp`；動態補全用 `ValidArgsFunction`；檔案參數用 `MarkFlagFilename` / `MarkFlagDirname`。
- 指令分組：先 `AddGroup` 再 `AddCommand`。

### Viper

- 優先序（固定不可調整）：`Set > flag > env > file > KV remote > default`。
- 三件套必須一起設定：`SetEnvPrefix("MYCLI")` + `SetEnvKeyReplacer(strings.NewReplacer(".", "_"))` + `AutomaticEnv()`。
- 設定檔是**可選的**：用 `errors.As` 優雅處理 `viper.ConfigFileNotFoundError`。
- Config struct 一律加 `mapstructure` tag；優先用 `UnmarshalKey` 而非 `Sub()`（後者在 key 不存在時回傳 nil）。
- Flag 綁定放在 `init()` 或 `PersistentPreRunE`，**禁止**放在 `RunE`。
- `time.Duration` 欄位需 `mapstructure.StringToTimeDurationHookFunc()`。
- **測試一律 `viper.New()` 建立獨立實例**，並將 `*viper.Viper` 注入應用結構；禁止測試直接操作全域 viper；env 測試用 `t.Setenv`。

### CLI 行為約定

- **stdout 放資料、stderr 放日誌與錯誤**——管線友善是硬需求。
- Exit codes：`0` 成功；`1` 一般錯誤；`2` 使用方式錯誤（flag/args）；必要時採用 sysexits（64–78）；被訊號終止為 `128+N`。
- 支援 `--output table|json`（或 `json|yaml`）機器可讀輸出。
- 版本注入：`version.go` 宣告 `var version = "dev"`，建置時以 `-ldflags "-X main.version=..."` 注入；提供 `version` 子命令。
- 訊號處理：`signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`，確保清理路徑執行。
- `--help` 是主要文件：每個命令的 `Short` / `Long` / `Example` 必須完整。

---

## 4. 程式碼規範

### 命名（`golang-naming`）

- `MixedCaps`；套件名小寫、單數、不與目錄名重複（no stutter）。
- Constructor：`New()` / `NewTypeName()`。
- Error 變數 `Err` 前綴（`ErrNotFound`）；error 型別 `Error` 後綴（`ParseError`）。
- 縮寫全大寫：`URL`、`HTTPServer`、`ID`。
- Enum zero value 放 `StatusUnknown`（iota 0）。
- Boolean 用 `is/has/can` 前綴；getter 省略 `Get` 但保留 `Is/Has/Can`。
- Interface：1–3 個方法、定義在**消費端**、`-er` 命名；「accept interfaces, return structs」；不預先抽象。
- 編譯期檢查：`var _ Iface = (*Type)(nil)`。

### 風格（`golang-code-style`）

- 每行約 120 字元為上限，在語意邊界換行。
- Early return / guard clauses；`switch` 優先於 if-else 鏈。
- 零值用 `var`，非零初始用 `:=`；slice/map 明確初始化；已知大小先配置容量。
- Composite literal 一律寫欄位名。
- 函式參數 ≤ 4 個（超過改用 options struct）；`ctx` 永遠是第一個參數且命名為 `ctx`。
- 優先使用 `slices` / `maps` / `cmp` 標準庫；泛型優先於 `any`。
- 最小化 exported surface；有序列化的 exported 欄位必須有 struct tag。
- 避免 `init()` 與可變全域狀態。

### 錯誤處理（`golang-error-handling`）

- **永遠檢查 error**，禁止 `_ = err`。
- 包裝：`fmt.Errorf("open config: %w", err)`；訊息小寫開頭、無句尾標點。
- 內部傳遞用 `%w`，邊界（給使用者的訊息）用 `%v`。
- 判斷用 `errors.Is` / `errors.As`（Go 1.26+ 可用 `errors.AsType[T]`）。
- 多個獨立錯誤用 `errors.Join`。
- **單一處理原則**：一個錯誤只 log **或** 只回傳，絕不同時做。
- 預期中的錯誤禁止 panic；panic 僅用於程式錯誤（bug）。
- Sentinel errors：`var ErrNotFound = errors.New("not found")`。
- 日誌用 `slog`，禁止 `fmt.Println` 做正式輸出。

### 安全（`golang-security`）

- SQL 一律參數化查詢。
- 子程序：`exec.Command(name, args...)` 分開傳參，**禁止** `bash -c` 字串拼接。
- Token/亂數用 `crypto/rand`；秘密比較用 `crypto/subtle.ConstantTimeCompare`；密碼用 Argon2id/bcrypt；對稱加密用 AES-GCM。
- 禁止硬編碼秘密；從 env / 設定檔讀取。
- 路徑處理防 traversal：Go 1.24+ 用 `os.Root`，否則 `filepath.IsLocal` + `filepath.Rel`。
- 在信任邊界驗證所有輸入；給使用者的錯誤訊息保持通用，細節進 log。
- 每次發布前跑 `govulncheck`。
- 思考模型：trust boundaries → attacker control → blast radius；嚴重性只能「調降並留下 inline 註解說明」，不得直接忽略。

### 併發與 Context（`golang-concurrency` / `golang-context`）

- 每個 goroutine 都有明確退出條件；每個 `select` 都要處理 `ctx.Done()`。
- Worker pool 用 `errgroup`（`SetLimit(n)`），不手刻。
- 只有 sender 關閉 channel；`wg.Add` 在 `go` 之前（Go 1.25+ 可用 `wg.Go`）。
- Mutex 不得跨越 I/O 持有。
- `ctx` 不存進 struct、不為 nil（用 `context.TODO()`）；`cancel()` 在所有路徑 defer。
- `context.Background()` 只在頂層使用；背景工作脫離請求生命週期用 `context.WithoutCancel`。

---

## 5. 測試規範（`golang-testing` / `golang-stretchr-testify`）

- Table-driven tests + 具名子測試：`t.Run(tt.name, func(t *testing.T) { ... })`。
- 測試檔與來源檔同名（`foo.go` → `foo_test.go`），測試順序對應來源順序。
- `t.Parallel()` 盡量加；測試之間不得有順序依賴。
- testify：**`assert.New(t)` 必須放在 `t.Run` 內部**（放父層會錯誤歸因失敗）；前置條件用 `require`、驗證用 `assert`；參數順序 `(expected, actual)`；包裝錯誤用 `is.ErrorIs`；mock 收尾 `AssertExpectations(t)`。
- Suite 必須有 `suite.Run(t, new(XxxSuite))` launcher。
- 整合測試加 `//go:build integration` build tag，預設不跑。
- Cobra 測試：每個測試建立**全新的 command tree**（cobra 會累積 flag 狀態），用 `SetArgs` / `SetOut` / `SetErr`；golden files 支援 `-update` flag。
- Viper 測試：`viper.New()` per test + 注入（見 §3）。
- goroutine 洩漏檢測：`goleak.VerifyTestMain`。
- 時間相依邏輯：Go 1.25+ 用 `synctest.Test`（不是舊的 `synctest.Run`）或 fake clock，禁止 `time.Sleep`。
- Benchmark：`b.Loop()` + `b.ReportAllocs()`（Go 1.24+）。
- CI 測試命令：`go test -race -shuffle=on ./...`。
- **本機（Windows，無 gcc）**：race detector 需要 cgo（gcc），本機測試改用 `go test -shuffle=on ./...`；`-race` 只在 CI（Linux runner）執行。本專案為純 Go，建置與正式測試皆不需要 CGO。

---

## 6. Lint 與格式化（`golang-lint`）

- 以 `.golangci.yml`（v2 格式）為唯一準則；本專案採用完整建議設定。
- Formatters：`gofumpt` + `goimports`。
- 重點 linters：`govet`、`staticcheck`、`errcheck`、`errorlint`、`nilerr`、`gosec`、`gocritic`、`revive`、`gocyclo(13)`、`funlen(120/80)`、`goconst(3/4)`、`sloglint`、`testifylint`、`paralleltest`、`modernize`、`exhaustive`、`nolintlint` 等（完整清單見設定檔）。
- `//nolint` 必須指名 linter 並附說明：`//nolint:gosec // 理由`——`nolintlint` 會強制。
- 提交前：`golangci-lint run` 零錯誤。

---

## 7. 依賴管理（`golang-dependency-management`）

- **AI Agent 鐵則：新增任何第三方依賴（`go get` 新套件）之前，必須先徵得使用者同意。** 升級既有依賴不需要。
- `go.sum` 必須 commit；提交前跑 `go mod tidy` 並確認無殘留 diff。
- 例行升級用 `go get -u=patch`；重大升級逐一處理。
- Go 1.24+ 開發工具用 go.mod 的 `tool` directive（`go get -tool <cmd>`、`go tool <name>`），不用 tools.go。
- 鎖定版本前先查 pkg.go.dev 確認維護狀態與授權。
- 每次發布前：`govulncheck ./...`。

---

## 8. 文件規範（`golang-documentation`）

- 所有 exported 符號都要有 doc comment，開頭以名稱起句 + 動詞片語。
- 每個套件必須有 package comment。
- README 順序：標題 → badges → 摘要 → demo → getting started → features → contributing → license。
- CHANGELOG 採 [Keep a Changelog](https://keepachangelog.com) 格式。
- CLI 的主要文件是 `--help`：`Short`/`Long`/`Example` 必須完整且與行為同步。

---

## 9. CI / 品質關卡（`golang-continuous-integration`）

GitHub Actions 階段順序：**test → lint → security → release**。

- test：`go test -race -shuffle=on -coverprofile=coverage.out ./...`
- 依賴整潔：`go mod tidy && git diff --exit-code`
- lint：golangci-lint-action
- security：`govulncheck` + `gosec`（或 CodeQL）
- release：GoReleaser（CLI 發布標配）
- Dependabot/Renovate 自動更新；Actions 釘 major 版本；workflow 設最小權限 `permissions:`。

---

## 10. 變更顆粒度工作約定（本專案最高原則）

> 使用者要求：**每次變更顆粒度要小到人可以 review。**

### 10.1 Git worktree 工作流程（強制）

> 使用者要求：**每次變動前先用 `git worktree` 建立獨立空間；確認功能符合需求後才合併回 `main`。**

1. **建立隔離工作區**：每次變更開始前，從最新 `main` 建立 worktree + branch：
   ```powershell
   git worktree add ../my-cli-worktrees/<topic> -b feat/<topic>
   ```
   - `<topic>` 用短連字號命名（如 `add-makefile`、`add-lint-config`）。
   - 主 repo 目錄保持在 `main`，**不在主 repo 直接改碼**。
2. **在 worktree 內完成變更**：所有編輯、build、test、lint 都在 worktree 目錄執行。
3. **驗證**：worktree 內跑 build / vet / test（本機 `go test -shuffle=on ./...`）全綠。
4. **使用者確認**：向使用者展示變更摘要與驗證結果，**取得同意後**才合併。
5. **合併回 main**：
   ```powershell
   git -C ../my-cli-worktrees/<topic> add -A
   git -C ../my-cli-worktrees/<topic> commit -m "<type>: <summary>"
   git checkout main
   git merge --ff-only feat/<topic>   # 保持線性歷史；不行就先 rebase
   git worktree remove ../my-cli-worktrees/<topic>
   git branch -d feat/<topic>
   ```
6. **清理**：合併後立即移除 worktree 與分支；`git worktree list` 應只剩主 repo。

### 10.2 顆粒度規則

1. **一次只做一件事**：一個 commit / 一次編輯只處理一個關注點（新功能、重構、修 bug、格式化不得混在同一變更）。
2. **小而聚焦的 diff**：單一變更以 100–500 行為上限（`golang-refactoring` 標準）；能拆就拆。
3. **每步驗證**：每次編輯後立即以 build / vet / lint / 相關測試驗證，不通過不進下一步。
4. **先說明再動手**：變更前簡述要做什麼、為什麼；重大變更（跨套件搬移、exported API 變更、刪除）先取得使用者同意。
5. **重構與行為變更分離**：絕不混合結構性與行為性變更。
6. **風險分級**：Low（改名/抽變數）→ build+vet+test 即可；Medium（抽函式）→ 加目標測試；High（簽名變更/搬套件）→ 完整安全網 + 人工 checkpoint。
7. **出錯就回退**：從乾淨的 committed baseline 出發；修不好就 revert，不往前硬 debug。
8. **除錯守則**（`golang-troubleshooting`）：沒有根因不修；先重現（失敗測試）再修；一次只驗證一個假設；嘗試 3 次以上仍失敗 = 心智模型錯了，停下來重新分析。

---

## 11. 常見錯誤對照表

| ✗ 錯誤做法 | ✓ 正確做法 |
|---|---|
| `Run:` 欄位 | `RunE` |
| `RunE` 裡 `len(args)` 檢查 | `Args: cobra.ExactArgs(1)` |
| 吞掉 error（`_ =`） | 檢查並處理每個 error |
| `fmt.Errorf("...: %v", err)` 內部傳遞 | `%w` 包裝 |
| `fmt.Println` 輸出資訊 | `slog`（stderr）+ stdout 只放資料 |
| 全域 viper 直接操作 | `viper.New()` + 注入 `*viper.Viper` |
| flag 綁定寫在 `RunE` | 綁定在 `init()` / `PersistentPreRunE` |
| 測試共用一個 command tree | 每測試重建 command tree |
| `assert.New(t)` 放在 `t.Run` 外 | 放在 `t.Run` 內部 |
| `time.Sleep` 等待非同步 | `synctest.Test` / fake clock / channel |
| `bash -c "..."` 拼字串 | `exec.Command(name, args...)` |
| `math/rand` 產生 token | `crypto/rand` |
| 手刻 worker pool | `errgroup` + `SetLimit` |
| 新依賴直接 `go get` | 先問使用者再 `go get` |
| 一個 PR 混雜重構+新功能 | 分開、各 100–500 行 |
| 憑直覺最佳化 | 先 profile，benchstat 對比 |

---

## 12. 現代化備忘（`golang-modernize`）

新代碼直接使用：`math/rand/v2`、`slices`/`maps`/`cmp`、`log/slog`、range-over-int、`min`/`max` 內建、iterators（1.23+）、`os.Root`（1.24+）、`b.Loop()`（1.24+）、`synctest.Test`（1.25+）、`t.Context()`、`t.ArtifactDir()`（1.26+）、`errors.AsType[T]`（1.26+）、stdlib `crypto/sha3`/`hkdf`/`pbkdf2`（1.24+）。

---

*本規範隨專案演進更新；更新本身也遵守 §10 的顆粒度約定。*
