# specs: update-agents-reflection

> 需求來源：PR #5 合併後，反思本次任務遭遇的錯誤（gosec G101 假陽性、osv-scanner 漏洞阻擋、API 端點與 Web Export 欄位假設偏差），更新規範至 `AGENTS.md`；若 `AGENTS.md` 過長，將細節抽離至獨立文件並在 `AGENTS.md` 建立參照。

## Why

在完成 `falcon-cis-export` 子命令並提交 PR #5 的過程中，經歷了以下問題：
1. **API 契約與 Web Export 對齊延遲**：初版憑空假設 API 端點與輸出欄位（9 欄且僅過濾 failed），直到拿到 Web 匯出的 `ComplianceByRules-...csv` 才發現端點實為 `/container-compliance/aggregates/rules/v2`，欄位為 12 欄且包含所有規則之資產通過率統計。
2. **數值型別反序列化崩潰**：Falcon API 回傳之百分比欄位為浮點數（如 `56.58`），Go struct 原定義為 `int` 造成 unmarshal error。
3. **gosec G101 命名誤判**：常數命名為 `tokenURLPath = "/oauth2/token"`，包含 `token` 關鍵字且賦予常數字串，被 gosec SAST 判定為 hardcoded credential，導致 GitHub Actions 安全關卡報錯並留言阻擋。
4. **SCA 依賴漏洞阻擋 PR**：既有 indirect 依賴 `google.golang.org/grpc` (1.83.1) 存在 GHSA-2v4p-qf9q-27wj 漏洞，推送到遠端後 `Security/SCA - osv-scanner` 失敗。
5. **AGENTS.md 篇幅膨脹**：隨著每次教訓增加，`AGENTS.md` 已累積大量案例與細節，需要模組化拆分，保留最高規範，將詳細踩坑紀錄與 API 整合細則抽離至 `docs/` 並保留參照。

## What

1. 新增 `docs/pitfalls-and-lessons.md`：
   - 彙整歷史上所有重大踩坑事件（YAML 語法、Action Tag、EOL 換行、CWD 漂移、gosec G101、SCA 漏洞、浮點反序列化等）及防範手段。
2. 新增 `docs/api-integration-guidelines.md`：
   - 確立第三方 API 與資料匯出規範（Source of Truth 原則、Web Export 比對清單、分頁與過濾策略、浮點與數值型別防禦）。
3. 更新 `AGENTS.md`：
   - 增補「第三方 API / 資料匯出之 Source of Truth 原則」。
   - 增補「gosec G101 避免憑證命名誤判」規範。
   - 增補「PR 提交前安全與 SCA 檢查（第 4.5 關）」與「PR 提交後主動追蹤 CI checks 紀律」。
   - 簡化過長的歷史案例細節，改為指向 `docs/pitfalls-and-lessons.md` 與 `docs/api-integration-guidelines.md` 參照。

## How

1. 建立 `docs/` 目錄與對應文件：
   - `docs/pitfalls-and-lessons.md`
   - `docs/api-integration-guidelines.md`
2. 精簡與補充 `AGENTS.md`：
   - 在 §3 / §4 加入外部 API 與安全命名原則。
   - 在 §9 / §10 加入安全檢查第 4.5 關與 PR CI 追蹤要求。
   - 在 §11 錯誤對照表增補本次事故重點，並指引至 docs 詳情。
3. 執行品質關卡（build, vet, lint, test, markdown / file 檢查）。
4. 依 worktree 流程合併並驗證。

## 遭遇的困難

1. 規範若全部堆疊於單一檔案 `AGENTS.md`，隨著事故案例與細部指引增加會使核心原則難以快速查閱。
2. 規範中既有規則包含大量歷史事故（如 `fix-security-yaml`、`fix-ci-action-tags` 等），若只增不減會失去檢核表的精煉特性。

## 如何解決

1. 採用模組化結構，將具體指引與細節抽離：
   - 建立 `docs/api-integration-guidelines.md`：明確規範 Source of Truth、Web Export 欄位 100% 對齊、浮點型別宣告與分頁拉取。
   - 建立 `docs/pitfalls-and-lessons.md`：詳述各歷史事故與本次 PR #5 的根本原因及標準解法。
2. 在 `AGENTS.md` 核心規範中僅保留精要的「鐵則」與「檢核清單」，並透過相對 markdown 連結參照至 `docs/` 文件。

## 最後變動了什麼

- `docs/pitfalls-and-lessons.md`：新增歷史踩坑紀錄與經驗反思文件（涵蓋 gosec G101、SCA osv-scanner、API 契約偏差、浮點反序列化、CWD 漂移、EOL 與 Action tag 等）。
- `docs/api-integration-guidelines.md`：新增外部 API 整合與資料匯出規範（Source of Truth 優先、CSV 檢核清單、型別安全、分頁策略）。
- `AGENTS.md`：
  - §3 增補外部 API 匯出遵循 Source of Truth 條款並連結至 docs。
  - §4 增補憑證命名安全規範（防 gosec G101 假陽性）。
  - §7 增補依賴漏洞掃描與間接依賴主動修復規範。
  - §9 增補 PR 提交後主動追蹤 CI checks 紀律。
  - §10 增補提交前安全第 4.5 關掃描。
  - §11 常見錯誤對照表補充本次錯誤項目並連結至 docs。
- `specs/20260912-update-agents-reflection.md`：記錄本工作項規格與過程。

## 驗證結果

1. `go build ./...`：通過。
2. `go vet ./...`：通過。
3. `go tool golangci-lint run`：通過（0 issues）。
4. `go test -shuffle=on ./...`：全數通過。
5. `govulncheck ./...`：No vulnerabilities found。
