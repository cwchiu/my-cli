# 20260906 — test/expand-coverage

## Why

路線圖工作項 3。coverage 基線 70.3%，缺口集中在：

- `main.go run()` 0%——exit code 映射（0/1/2）是 AGENTS.md §3 的核心契約，完全沒測。
- `root.go initConfig` 81%——設定檔存在/損壞/env 覆寫路徑未測。
- `version.go newVersionCmd` 68.2%——`-o xml` 可達的 default 分支未測。

## What

- 新增 `main_test.go`：`run()` 的三種 exit code（成功 0、用法錯誤 2、一般錯誤 1）。
- 新增 `root_test.go`：`initConfig` 有效檔、明確缺檔（錯誤）、損壞檔（錯誤）、
  搜尋路徑無檔（容忍）、env 覆寫。
- 目標：`go test -cover` ≥ 85%。
- 非目標：不動生產代碼；`main()` 本身（`os.Exit`）不可測，維持 0%。

## How

- `run()` 自建 command tree 且寫進程 stdout/stderr、讀 `os.Args`——測試以 `os.Pipe`
  重定向 stdout/stderr 並暫換 `os.Args`，故這類測試不能 `t.Parallel`
  （`//nolint:paralleltest` 附理由）；env 測試用 `t.Setenv`（同樣禁 parallel）。
- `initConfig` 直接以 `newRootCmd()` 呼叫，viper 執行緒由 `cmd.Context()` 取回斷言。
- 一般錯誤路徑用 `version -o xml` 觸發 default 分支（順帶覆蓋 version.go 缺口）。

## 遭遇的困難

1. **`initConfig` 直接呼叫時 panic**：`cannot create context from nil parent`——
   `initConfig` 內 `context.WithValue(cmd.Context(), ...)`，而 `cmd.Context()` 在
   cobra `Execute()` 之前是 nil。測試直接呼叫 `initConfig` 繞過了 Execute。
2. **語意誤判**：原設計以為「`--config` 指向不存在的檔」不是錯誤。實測 viper 對
   `SetConfigFile` 的缺檔一樣回傳 `ConfigFileNotFoundError`？——不，實測回傳的是
   一般錯誤（`read config`），只有「未指定 --config、搜尋路徑找不到」才容忍。
3. **Lint 11 項**：errcheck（pipe `Close()` 未檢查）、goconst（`"version"` 字面值
   達 5 次，既有常數 `argVersion` 未沿用）、gofumpt 格式、nolintlint
   （`TestInitConfigEnvOverride` 的 `//nolint:paralleltest` 多餘——paralleltest
   對用 `t.Setenv` 的測試本就不要求 parallel）、revive（未使用參數 `t`）、
   wsl_v5 空行規則 ×3。
4. **編輯事故**：移除 nolint 註解時替換錨點吃掉了 `func TestInitConfigEnvOverride(t *testing.T) {`
   行首，造成 `f//nolint...` 語法錯誤——gofumpt 報 parse error 才發現。

## 如何解決

1. 測試先 `cmd.SetContext(t.Context())` 模擬 Execute 行為，再直接呼叫 `initConfig`。
2. 改測試預期：缺檔（明確指定）→ 斷言 `read config` 錯誤；新增「無 --config 且
   搜尋路徑無檔 → 非錯誤」案例，同時覆蓋 else 分支與容忍分支。
3. 逐一修正：`require.NoError(t, outW.Close())`；沿用 `argVersion` 常數（並把
   `version_test.go` 殘留的 3 處 `"version"` 字面值也換掉，消除 goconst）；
   `go tool golangci-lint fmt` 統一格式；刪除多餘 nolint；`func(*testing.T)` 無名參數；
   wsl 空行。
4. 依 AGENTS.md §10.3「編輯後回讀」——回讀發現破損行，立即修復。

## 最後變動了什麼

- `cmd/my-cli/main_test.go`（新增）：`captureStdio` 輔助（os.Pipe 重定向
  stdout/stderr）+ `TestRunExitCodes` 表驅動三案例（0/2/1），覆蓋 `run()` 0%→100%。
- `cmd/my-cli/root_test.go`（新增）：`TestInitConfig` 四案例（有效檔/明確缺檔/
  損壞檔/無檔容忍）+ `TestInitConfigEnvOverride`（`MYCLI_LOG_LEVEL` → `log.level`），
  `initConfig` 81%→90.5%。
- `cmd/my-cli/version_test.go`：3 處 `"version"` 字面值改用 `argVersion` 常數（goconst）。
- `specs/20260906-expand-coverage.md`：本紀錄。

## 驗證結果

- `go build ./...` ✅
- `go vet ./...` ✅
- `go tool golangci-lint run` → **0 issues** ✅
- `go test -shuffle=on ./...` → ok ✅
- Coverage：**70.3% → 85.9%**（達標 ≥85%）。
  `run()` 100%、`newRootCmd` 100%、`initConfig` 90.5%、`newVersionCmd` 72.7%、
  `main()` 0%（`os.Exit` 不可測，刻意排除）。
- 未覆蓋殘留：`initConfig` 的 `UserHomeDir` 失敗分支與 `--config` flag 讀取失敗分支
  （正常環境不可達）；`newVersionCmd` 的 json 寫入錯誤分支（無法注入寫入失敗）。

