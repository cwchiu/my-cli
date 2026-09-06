# 20260906-fix-security-yaml

## Why

- 工作項 1（`feat/add-ci-workflow`，ba9cd99）合併 push 後，GitHub Actions 實測報錯：
  `Invalid workflow file: You have an error in your yaml syntax on line 58`（run 34021200986，`.github/workflows/security.yml`）。
- 連帶症狀：Dependabot 報 `dependency_file_not_parseable`（run 34021204517）——github-actions ecosystem 無法解析同一個損壞的 workflow 檔。
- 整個 security workflow 無法載入，且 YAML 損壞屬合併後驗證（§10.1 第 7 條延伸：workflow 語意以 Actions 實運行為最終驗證）發現的缺陷，依規範開獨立 fix 工作項。

## What

**範圍**：修復 `security.yml` gosec job 的 YAML 語法錯誤，使 security workflow 可被 Actions 解析、Dependabot 可正常掃描。

**非目標**：

- 不調整其他 workflow、不動 gosec 以外的 action 釘版。
- CodeQL private repo 授權問題（若後續 Actions 實測失敗）另開工作項。

## How

根因（讀檔確認，L56–59）：

```yaml
      - name: Run gosec
        uses: securego/gosec@master
        with:v2
          args: -no-fail -fmt sarif -out gosec-results.sarif ./...
```

工作項 1 執行期間，「把 gosec 從 @master 釘為 @v2」的替換編輯**套錯位置**：`v2` 被附加到 `with:` 行變成 `with:v2`（非法 YAML key），而 `uses:` 行仍是 `@master`。當時 specs（20260906-add-ci-workflow.md「如何解決 #2」）宣稱已釘為 @v2，與檔案實際內容不符——編輯工具回報成功不等於結果正確，未回讀驗證。

修復（兩行）：

```yaml
      - name: Run gosec
        uses: securego/gosec@v2
        with:
          args: -no-fail -fmt sarif -out gosec-results.sarif ./...
```

步驟：

1. `git worktree add ../my-cli-worktrees/fix-security-yaml -b fix/security-yaml`（自 main ba9cd99）
2. 修正 L57–58 兩行；**回讀檔案**確認（前案教訓：編輯後必須 re-read 驗證）。
3. 全庫掃描 4 個 workflow 的所有 `uses:` / `with:` 行，確認無其他同類錯位（結果：全部正常，Actions 均釘 major tag）。
4. 四關驗證（流程要求；YAML 變更不影響 Go 構建，但仍跑全）。

## 遭遇的困難

1. **編輯錯位未被發現**：`replace_string_in_file` 回報成功，但實際產出 `with:v2` + `@master`——old/new string 對齊錯誤導致替換落在非預期位置；specs 隨手記錄了「已釘 @v2」的錯誤結論。
2. **本機四關驗不到 YAML**：build/vet/lint/test 只覆蓋 Go 原始碼，workflow YAML 語法完全在閘門之外。
3. **本機無 Python/PyYAML**：`python` 是 Microsoft Store 佔位別名、`py` 啟動器不存在，無法直接 `yaml.safe_load` 驗證全部 YAML 檔。

## 如何解決

1. 修復後立即 `read_file` 回讀 L40–75 目視確認，再用 `Select-String` 列出全部 4 個 workflow 的 `uses:`/`with:` 行逐一檢查——32 處全部格式正確。教訓記入流程：**編輯後回讀是驗證步驟，不是可選項**。
2. 將「workflow YAML push 後以 Actions 解析結果為準」列為合併後驗證的固定檢查項（本工作項即此機制的實例）。
3. 經使用者提示，本機以 **`uv run --with pyyaml python`** 做離線 YAML 驗證（uv 臨時環境，不引入 repo 依賴）：6 個 YAML 檔全部 `safe_load` 通過。

## 最後變動了什麼

修改：

- `.github/workflows/security.yml` L57–58 — `uses: securego/gosec@master` + `with:v2` → `uses: securego/gosec@v2` + `with:`（修復 YAML 語法錯誤，同時完成原工作項意圖的 gosec major 釘版）
- `specs/20260906-fix-security-yaml.md` — 本紀錄
- `specs/20260906-add-ci-workflow.md` — 更正「如何解決 #2」與「最後變動了什麼」中「gosec 已釘 @v2」的不實記載，指向本工作項

## 驗證結果

本機（worktree）：

- 回讀 `security.yml` L40–75：gosec step 為 `uses: securego/gosec@v2` + `with:`，結構正確
- `Select-String` 全 workflow `uses:`/`with:` 掃描：32 處全部正常，無殘留 `@master` 或 `with:v2`
- `uv run --with pyyaml python`（PyYAML `safe_load`）：4 個 workflow + codeql-config + dependabot 共 6 檔全部解析通過
- `go build ./...` → exit 0
- `go vet ./...` → exit 0
- `go tool golangci-lint run` → 0 issues
- `go test -shuffle=on ./...` → ok

合併 push 後（最終驗證）：GitHub Actions 應能成功解析 security.yml 並啟動 4 個 job；Dependabot 手動觸發（"Update dependencies now"）應不再報 `dependency_file_not_parseable`。
