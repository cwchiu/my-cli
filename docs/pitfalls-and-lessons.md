# 歷史踩坑紀錄與經驗反思（Pitfalls & Lessons Learned）

> 本文件記錄專案開發過程中遭遇的實際問題、根本原因與防範處方。
> 目標是將實例細節獨立存放，使 `AGENTS.md` 保持精簡，同時提供完整的錯誤復線與查閱依據。

---

## 1. 安全檢查（SAST & SCA）陷阱

### 1.1 gosec G101 疑似硬編碼憑證假陽性
- **事故場景（PR #5）**：
  在新增 CrowdStrike Falcon API 客戶端時，宣告常數：
  ```go
  //nolint:gosec // This is an OAuth2 endpoint path, not a secret.
  const tokenURLPath = "/oauth2/token"
  ```
  提交 PR 後，GitHub Actions 的 `Security/SAST - gosec` 掃描失敗，github-advanced-security bot 在 PR 留言阻擋：
  `gosec / Potential hardcoded credentials`。
- **根本原因**：
  `gosec` 的 G101 規則採 AST 靜態比對，當常數或變數名稱包含 `token`、`secret`、`password`、`key`、`passwd` 且賦予字串常數時，會自動判定為潛在的硬編碼密鑰；且原本的 `//nolint:gosec` 註解因 `gosec` 獨立執行時不吃 golangci-lint 格式註解而未生效。
- **標準解法**：
  1. **命名避開敏感字眼**：非憑證之路徑常數命名改用 `path...`、`endpoint...`（如 `pathOAuth2 = "/oauth2/token"`）。
  2. **標準抑制註解**：在常數上一行加上 gosec 原生註解：
     ```go
     // #nosec G101 -- this is an OAuth2 endpoint path, not a secret.
     const pathOAuth2 = "/oauth2/token"
     ```

### 1.2 間接依賴漏洞阻擋 PR（SCA / osv-scanner）
- **事故場景（PR #5）**：
  PR 提交後，CI 的 `Security/SCA - osv-scanner` 失敗，回報既有 indirect 依賴 `google.golang.org/grpc` (v1.83.1) 存在中度漏洞 `GHSA-2v4p-qf9q-27wj`（修復版本為 `v1.83.2`）。
- **根本原因**：
  本機在提交前僅跑了 build、vet、lint、test 四關，未在 push 前執行依賴安全掃描；且間接依賴若有新揭露之 CVE，CI 的 lockfile 掃描會立即強制阻擋。
- **標準解法**：
  1. **本地執行第 4.5 關**：提交 PR 前執行 `task security:all`（或 `govulncheck ./...` 與 `osv-scanner`）。
  2. **主動升級修復版本**：若發現漏洞，使用 `go get <pkg>@<fixed-version>` 明確鎖定安全版本，並執行 `go mod tidy`。

---

## 2. API 整合與資料匯出陷阱

### 2.1 缺乏 Source of Truth 導致端點與欄位假設偏差
- **事故場景（PR #5）**：
  初版實作 `falcon-cis-export` 時，先入為主猜測使用 `cloud-security-assets` 端點，後續又依猜想使用 `compliance-by-framework/v2` 並只過濾 failed 項目輸出 9 個自訂欄位。然而使用者真實需要的，是與官方 Web UI Export 的 `ComplianceByRules-...csv` 完全一致的 12 欄規則資產評估報表（96 條規則）。
- **根本原因**：
  在動手開發前，沒有先向使用者確認或要求「Web 匯出的原始 CSV 範例」作為契約基準，憑空推導 schema。
- **標準解法**：
  見 [api-integration-guidelines.md](api-integration-guidelines.md)。實作前一律要求真實 export 檔案作為 Source of Truth，完全比對 Header、列數、欄位型別與排序。

### 2.2 數值型別反序列化崩潰（int vs float64）
- **事故場景（PR #5）**：
  解析 Falcon API 回傳之 `percentage_of_passed_rules` 時，Go struct 定義為 `int`，運行時拋出：
  `json: cannot unmarshal number 56.58 into Go struct field ... of type int` 導致程式中止。
- **根本原因**：
  API 中的比例與百分比常包含小數，不可假設所有整數看起來的計量都是 int。
- **標準解法**：
  所有比例、百分比、平均值或計量欄位，一律採用 `float64`。

---

## 3. Git 與環境紀律陷阱

### 3.1 終端工作目錄（CWD）漂移
- **事故場景（add-taskfile）**：
  在主 repo 目錄誤執行了針對 worktree 的 `go get` / `go mod tidy`，污染主 repo 的 `go.mod`。
- **防範手段**：
  執行任何終端指令前，務必確認當前目錄位於 worktree；或使用 `git -C <path>` 與明確的絕對路徑。

### 3.2 EOL 與 .gitattributes 合併後未套用
- **事故場景（fix-eol）**：
  fast-forward merge 引入 `.gitattributes` 後，主 repo 的工作區檔案未自動重新 checkout，導致主 repo 上的 lint 噴出 CRLF 錯誤。
- **防範手段**：
  合併回 main 不是結束，必須在主 repo 重跑完整驗證，確保環境與屬性一致。

### 3.3 YAML 與設定檔改動缺乏 Parser 驗證
- **事故場景（fix-security-yaml）**：
  文字取代落點錯誤造成 invalid YAML，直到 push 到 GitHub Actions 才爆出解析失敗。
- **防範手段**：
  改動 YAML 檔案後，強制使用 parser（如 `uv run --with pyyaml python`）執行 safe_load 驗證（第五關），嚴禁僅靠肉眼檢查。

### 3.4 GitHub Action 釘選幽靈 Tag
- **事故場景（fix-ci-action-tags）**：
  直接釘選 `@v2`，但遠端 repo 僅發布確切的 semver tag（如 `@v2.5.1`），導致 CI 報 `Unable to resolve action`。
- **防範手段**：
  引用 Action 前，先以 GitHub API（`GET /repos/<owner>/<repo>/tags`）確認 tag 存在。
