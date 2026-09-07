# 20260908 — fix-xcrypto-cves：升級 golang.org/x/crypto 修復 CodeQL 安全警告

## Why

GitHub code-scanning（CodeQL SCA）在 `go.mod` 上報了 3 個 open 安全警告，全部指向間接依賴 `golang.org/x/crypto v0.55.0`：

| Alert | 識別碼 | 內容 | 修復版本 |
|---|---|---|---|
| #4 | CVE-2026-56855 | ssh 已建立 channel 可被惡意 peer deadlock（DoS，CVSS 3.1 7.5 HIGH，CWE-770） | v0.56.0 |
| #3 | CVE-2026-78662 | ssh 未建立 channel 的 incomingRequests 可被灌爆導致整條連線 deadlock（DoS，CVSS 3.1 7.5 HIGH，CWE-770） | v0.56.0 |
| #2 | GO-2026-5932 | `x/crypto/openpgp` 套件 unmaintained、unsafe by design | **無**（所有版本皆受影響） |

（Alert #1 CVE-2026-84304 gRPC OOM 已是 fixed 狀態，不在本次範圍。）

## What

- **目標**：將 `golang.org/x/crypto` 從 v0.55.0 升級到 v0.56.0，使 alert #3、#4 在合併後的 CodeQL 掃描自動轉為 fixed。
- **非目標**：不處理 alert #2（openpgp）——無修復版本存在；本專案程式碼不 import openpgp（govulncheck 呼叫圖證明），保留 open 作為已評估的 accepted risk，理由記錄於本檔。
- **範圍**：僅 `go.mod` / `go.sum` 兩檔的依賴版本變更，無程式碼變更。

## How

1. 從 main（9e1511d）建立 worktree `../my-cli-worktrees/fix-xcrypto` + branch `fix/xcrypto-cves`。
2. 在 worktree 內執行 `go get golang.org/x/crypto@v0.56.0` + `go mod tidy`（cwd �紀律：只在 worktree 執行）。
3. 驗證 diff 僅涉及 x/crypto 版本行。
4. 四關驗證：build → vet → lint → test。
5. commit → push → GitHub PR（新流程：不本地合併，由使用者在 GitHub 審核合併）。
6. 合併後：pull main、重跑四關、清理 worktree、驗證遠端 CodeQL 掃描將 #3/#4 轉 fixed。

**選型理由**：
- 升級既有間接依賴不需徵求同意（AGENTS.md §7：僅新增第三方依賴需同意）。
- v0.56.0 是目前最新版本（`go list -m -versions` 確認），同時是兩個 ssh CVE 的最低修復版本。
- x/crypto 進入模組圖的路徑：`go-task → go-getter → google.golang.org/api → s2a-go → x/crypto/cryptobyte`（`go mod why -m` 確認）；main module 本身不需要任何 x/crypto 套件（`go mod why golang.org/x/crypto` → "(main module does not need package)"）。
- `govulncheck ./...` 在升級前即為 "No vulnerabilities found"（呼叫圖分析：vulnerable symbols 不可達）；本升級屬縱深防禦 + 消除模組級掃描警報。

## 遭遇的困難

- 無重大困難。升級為單一間接依賴的 patch 級操作，`go get` 一次到位，`go mod tidy` 無殘留 diff。

## 如何解決

- 不適用（無困難需解決）。

## 最後變動了什麼

| 檔案 | 變更 |
|---|---|
| `go.mod` | `golang.org/x/crypto v0.55.0 → v0.56.0`（indirect 行，1 行） |
| `go.sum` | x/crypto v0.55.0 → v0.56.0 的 h1/go.mod 雜湊各 1 行（共 2 行替換） |
| `specs/20260908-fix-xcrypto-cves.md` | 本紀錄檔（新增） |

- Commit：`72ddf10` — `fix: upgrade golang.org/x/crypto to v0.56.0 (CVE-2026-56855, CVE-2026-78662)`
- PR：[#3](https://github.com/cwchiu/my-cli/pull/3)，merge commit `2874617`（2026-09-07T16:52:33Z 合併，11 checks 全綠）。

## 驗證結果

**四關（worktree 內，2026-09-08）：**

| 關卡 | 結果 |
|---|---|
| `go build ./...` | 通過（無輸出） |
| `go vet ./...` | 通過（無輸出） |
| `go tool golangci-lint run` | **0 issues** |
| `go test -shuffle=on ./...` | `ok github.com/cwchiu/my-cli/cmd/my-cli 0.762s` |

**diff 範圍確認**：`git diff --stat` → 僅 `go.mod`（2 行）、`go.sum`（4 行）；`git status` 無其他非預期變更；主 repo `D:\tmp\my-cli` 乾淨未受污染。

**合併後（2026-09-08 驗證）：**

- main（`2874617`）四關重跑：build ✅、vet ✅、lint **0 issues**、test `ok ... 0.731s`。
- 遠端 code-scanning alerts（`gh api .../code-scanning/alerts`）：

| Alert | 規則 | 狀態 | 說明 |
|---|---|---|---|
| #4 | CVE-2026-56855 | **fixed** | v0.56.0 超出受影響範圍，自動關閉 |
| #3 | CVE-2026-78662 | **fixed** | 同上 |
| #2 | GO-2026-5932 | fixed | 舊實例隨 v0.55.0 消失而關閉 |
| #5 | GO-2026-5932 | **open** | 新實例：v0.56.0 同樣無修復版本（openpgp 所有版本皆受影響） |
| #1 | CVE-2026-84304 | fixed | gRPC OOM（先前已修復） |

- **淨結果**：open 警告 3 → 1，剩餘的 #5 即 openpgp accepted risk——本專案不 import openpgp（govulncheck 呼叫圖證明），無修復版本存在，保留 open 並以本檔作為評估紀錄；日後若要讓 Security tab 歸零，可將 #5 dismiss 為 "Not used" 並引用本檔理由。
- worktree `fix-xcrypto` 與分支 `fix/xcrypto-cves`（本地+遠端）已清理；`git worktree list` 僅剩主 repo。

## 流程檢討（2026-09-08）

- **錯誤**：合併後才發現 specs 缺少合併後驗證結果，另開了 `docs/xcrypto-spec-result` worktree 準備補交——這違反「一個工作項一份完整紀錄」的精神，也多製造一次 PR 往返。
- **正確做法**：specs 的「合併後驗證」段落應在**合併前就預留並盡量填寫可預知的內容**（如預期 alert 狀態變化）；合併後若僅需補實測數測數據，屬於**同一工作項的收尾**，直接在 main 上以小 commit 補齊即可，不另開工作項/PR。
- **本次處置**：經使用者同意，本檔直接在 main 合併（docs commit），不另開 PR。
