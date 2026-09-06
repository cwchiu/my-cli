# 20260906-add-ci-workflow

## Why

- AGENTS.md §9 明文要求 GitHub Actions 品質閘門（test → lint → security → release），但 repo 至今**沒有任何 workflow**——所有品質把關只靠本機自律。
- `-race` 測試（§5）本機無 gcc 無法執行，CI（Linux runner）是唯一補口。
- 使用者審核時追加要求：工作流程需補齊 **SAST、SCA、Secret 偵測**。
- 使用者決策（2026-09-06）：SAST = gosec + CodeQL(security-extended)；SCA = govulncheck + osv-scanner；Secret = gitleaks（CI 掃 git 歷史）。
- `.github/agents/*.agent.md`（VS Code 自訂 review agents）經使用者同意一併納入版控。

## What

**範圍**（本工作項交付）：

1. `.github/workflows/test.yml` — Go matrix、`-race`、tidy 檢查、build、coverage 產出
2. `.github/workflows/lint.yml` — go vet + golangci-lint-action
3. `.github/workflows/security.yml` — 四 job：govulncheck（SCA）、gosec（SAST）、CodeQL（SAST, security-extended）、osv-scanner（SCA）
4. `.github/workflows/secrets.yml` — gitleaks 掃歷史/PR
5. `.github/codeql/codeql-config.yml` — security-extended 查詢集
6. `.gitleaks.toml` — 最小自訂規則（allowlist 佔位）
7. `.github/dependabot.yml` — gomod + github-actions 週一成組更新
8. `AGENTS.md` §9 — 補強為五階段：test → lint → SAST/SCA/Secret → security gate → release，點名工具
9. `.github/agents/` 兩個自訂 agent 檔納入版控

**非目標**（明確排除，記錄理由）：

- Bearer（SAST 第三工具，邊際效益低）
- Codecov 上傳（無 token，後續再議；coverage 檔仍產出）
- dependabot-auto-merge（需 elevated permissions + branch protection 先就位）
- Docker 鏡像掃描（專案不產 Docker 鏡像）
- release workflow / GoReleaser（屬工作項 5）
- branch protection 設定（GitHub 網頁操作，合併後提醒使用者手動設 required checks）

## How

1. `git worktree add ../my-cli-worktrees/add-ci-workflow -b feat/add-ci-workflow`（自 main 26d21c8）
2. 模板取自 `samber/cc-skills-golang@golang-continuous-integration` skill assets（test.yml / lint.yml / security.yml / dependabot.yml / codeql-config.yml），依本專案調整：
   - Go matrix `["1.26", "stable"]`（go.mod 宣告 `go 1.26.0`）
   - 移除 Codecov 上傳步驟
   - security.yml 加 osv-scanner job、移除 Bearer job
   - 新增 secrets.yml（gitleaks-action@v2；個人/public repo 免 GITLEAKS_LICENSE）
   - dependabot 移除 docker ecosystem
3. 所有 workflow 最小 `permissions:`（test/lint/secrets: `contents: read`；security 需 `security-events: write` 上傳 SARIF）
4. Actions 釘 major 版本（checkout@v6、setup-go@v6、golangci-lint-action@v9、codeql-action@v4 等）
5. 從主 repo 複製 `.github/agents/*.agent.md` 進 worktree 並 `git add`
6. AGENTS.md §9 改寫（規範與實作同工作項，diff 可對照）
7. 四關驗證：build / vet / lint / test（本機無 -race，CI 補）

### 風險

- **CodeQL analyze 在 private repo 無 GHAS 授權可能失敗**——若 CI 實測如此，降級方案另行處理（開 fix 工作項）。
- gitleaks 掃歷史可能翻出既有 commit 誤報——`.gitleaks.toml` allowlist 處理。
- workflow YAML 無法本機完整驗證語意，需合併 push 後以 Actions 實際運行結果為準（§10.1 第 7 條合併後驗證的延伸）。

## 遭遇的困難

1. **osv-scanner scan-args 衝突**：模板預設帶 `--recursive`，與本專案採用的顯式 `--lockfile=go.mod` / `--lockfile=go.sum` 互相衝突（顯式 lockfile 模式下不能再 recursive）。
2. **gosec action 引用未釘版本**：skill 模板使用 `securego/gosec@master`，違反 AGENTS.md §9「Actions 釘 major 版本」。
3. **`.github/` 在主 repo 是 untracked**：`git worktree add` 只帶出已追蹤檔案，worktree 內看不到 `.github/agents/*.agent.md`，直接編輯 worktree 會漏掉這兩個檔。
4. **gitleaks allowlist 初版過寬**：初稿把 `'''_test\.go$'''` 整個排除，等於對測試檔內的真實秘密失明。
5. **workflow 語意無法本機驗證**：GitHub Actions 的 action 版本、權限、SARIF 上傳行為只能 push 後實測（已列風險）。

## 如何解決

1. 移除 `--recursive`，僅保留兩個顯式 `--lockfile` 參數（osv-scanner 文件：顯式 lockfile 掃描不需 recursive）。
2. **意圖**為改釘 `securego/gosec@v2`，但實際編輯套錯位置（`v2` 接到 `with:` 行變成 `with:v2`，`uses:` 仍為 `@master`），產出非法 YAML——**本檔原記載「已釘 @v2」為不實，經 Actions 實測發現後由 `specs/20260906-fix-security-yaml.md` 修復更正**。其餘 actions 逐一檢查均為 major tag（checkout@v6、setup-go@v6、upload-artifact@v4、golangci-lint-action@v9、codeql-action/*@v4、upload-sarif@v4、govulncheck-action@v1、osv-scanner-action@v2、gitleaks-action@v2）。
3. 用 `Copy-Item D:\tmp\my-cli\.github\agents\*.agent.md` 複製進 worktree 後一併 `git add`（主 repo 保持乾淨）。
4. allowlist 改為空佔位（`paths = []`），僅作為日後誤報時的文件化處理位置；掃描範圍不縮水。
5. specs 風險節明確記載：合併 push 後以 Actions 實際運行為最終驗證；CodeQL 於 private repo 無 GHAS 失敗時另開 fix 工作項。

## 最後變動了什麼

新增：

- `.github/workflows/test.yml` — Tests：matrix `["1.26","stable"]`、`go mod verify`、tidy 檢查（`git diff --exit-code go.mod go.sum`）、build、`go test -v -race -shuffle=on -coverprofile=coverage.out`、coverage 產物上傳（stable）
- `.github/workflows/lint.yml` — Lint：`go vet` + golangci-lint-action@v9（timeout 5m）
- `.github/workflows/security.yml` — Security 四 job：govulncheck@v1；osv-scanner@v2（lockfile 模式，SARIF 上傳）；gosec（`-no-fail -fmt sarif`，SARIF 上傳；**原記載「@v2」不實，實際為 `@master` + 損壞的 `with:v2`，由 fix-security-yaml 工作項修復**）；CodeQL（init→autobuild→analyze，config=`.github/codeql/codeql-config.yml`）
- `.github/workflows/secrets.yml` — Secrets：checkout `fetch-depth: 0` + gitleaks-action@v2（全歷史/PR 掃描）
- `.github/codeql/codeql-config.yml` — `security-extended` 查詢集（使用者決策）
- `.gitleaks.toml` — 最小自訂設定，allowlist 空佔位
- `.github/dependabot.yml` — gomod（週一、minor/patch 成組）+ github-actions（週一成組）；無 docker ecosystem
- `.github/agents/go-code-review.agent.md`、`.github/agents/go-security-review.agent.md` — 自主 repo untracked 狀態納入版控
- `specs/20260906-add-ci-workflow.md` — 本紀錄

修改：

- `AGENTS.md` §9 — 階段順序改為 **test → lint → security（SAST + SCA + Secret）→ release**，點名 gosec/CodeQL(security-extended)/govulncheck/osv-scanner/gitleaks，補 `-race` 僅 CI、Dependabot 指向 `.github/dependabot.yml`

## 驗證結果

本機（worktree，Windows 無 gcc，依 §5 不含 `-race`）：

- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `go tool golangci-lint run` → **0 issues**（僅已知 `gofumpt: extra-rules is deprecated` 警告，屬工作項 2 範圍）
- `go test -shuffle=on ./...` → `ok github.com/cwchiu/my-cli/cmd/my-cli 0.648s`

待合併 push 後：GitHub Actions 實際運行為最終驗證（workflow 語意、SARIF 上傳、CodeQL 授權）。
