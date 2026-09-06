# 20260906-harden-agents-verification

## Why

近期三個工作項（add-ci-workflow → fix-security-yaml → fix-ci-action-tags）暴露
四個流程漏洞：

1. 編輯工具回傳成功，但變更落點錯誤（gosec pin 錯位造成 invalid YAML）——
   沒有「編輯後回讀」的強制步驟。
2. YAML 檔只靠肉眼檢查，本機無 parser 驗證——invalid YAML 一路推到 GitHub
   Actions 才爆。
3. Action pin 未查證 tag 存在性——`securego/gosec@v2`、
   `osv-scanner-action@v2` 的 floating major tag 根本不存在，推到遠端才爆。
4. push 後沒有檢查遠端 Actions/Dependabot 的步驟——錯誤發現延遲到使用者回報。

使用者已批准（「Y」）將這些教訓強化回 AGENTS.md。

## What

- 目標：把上述四點寫入 AGENTS.md 的強制規範，杜絕同類錯誤再犯。
- 範圍：僅 AGENTS.md（§9、§10.1、§10.3、§11）＋本 specs 紀錄。
- 非目標：不動 workflow 檔、不動程式碼。

## How

1. §9：「Actions 釘 major」改為「pin 前以 GitHub API 驗證 tag 存在；無
   floating major tag 的 repo 釘確切 semver，靠 Dependabot 更新」。
2. §10.1 新增第 10 條：push 後遠端驗證（Actions run 解析/解析 action/job 結果
   ＋ Dependabot）。
3. §10.3 規則 3 加入「編輯後回讀」強制步驟與「YAML 第五關」
   （`uv run --with pyyaml python` 解析驗證）。
4. §11 對照表新增四列對應錯誤做法。

## 遭遇的困難

1. 規範文字要精確對應實例（附工作項名稱）才可稽核，又不能膨脹到難以執行。
2. 「四關」是本專案既定術語，新增 YAML 驗證需定位為「第五關（僅 YAML/設定檔）」
   以免混淆既有流程。

## 如何解決

1. 每條新規則都附實例出處（add-ci-workflow / fix-security-yaml /
   fix-ci-action-tags），與既有 §10 條文風格一致。
2. YAML 驗證明確限定適用對象（`.github/**`、`*.yml`/`*.yaml`），並附可直接
   複製的 uv 指令。

## 最後變動了什麼

- `AGENTS.md`：§9 action pin 規則修正；§10.1 新增第 10 條 push 後遠端驗證；
  §10.3 規則 3 加入編輯後回讀＋YAML 第五關；§11 新增四列。
- `specs/20260906-harden-agents-verification.md`：本紀錄

（commit hash 待提交後補）

## 驗證結果

- 編輯後回讀：`Select-String` + `read_file` 確認四處變更落點正確
  （§9 L239、§10.1 L273、§10.3 L298-299、§11 L330-331）
- `go build ./...` → OK；`go vet ./...` → OK
- `go tool golangci-lint run` → 0 issues
- `go test -shuffle=on ./...` → ok（cmd/my-cli 1.057s）
- `uv run --with pyyaml python` → YAML OK: 8 files
