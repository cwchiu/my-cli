# 20260907 — docs/add-license-changelog

## Why

- README 的 License 章節先前因尚無 `LICENSE` 檔而暫留 TBD 佔位（add-readme 工作項刻意留待後續）。
- 開放原始碼專案需要明確的授權宣告，才能被合法使用與重散布。
- 使用者核准 roadmap 項目 4b：LICENSE（Apache-2.0）+ CHANGELOG（Keep a Changelog）+ README License 章節改正式宣告。

## What

- 新增 `LICENSE`：Apache License 2.0 官方全文（canonical，不修改）。
- 新增 `CHANGELOG.md`：Keep a Changelog 1.1.0 格式 + SemVer 宣告，`[Unreleased]` 記錄至今全部變更。
- 更新 `README.md`：badges 列加 License badge；License 章節由 TBD 改為 Apache-2.0 正式宣告（含 Copyright 2026 cwchiu 與授權條款摘錄）。
- 非目標：不打 git tag、不做 release（屬後續 goreleaser 工作項）。

## How

1. `git worktree add ../my-cli-worktrees/add-license-changelog -b docs/add-license-changelog`（自 17b2f94）。
2. 以 `Invoke-WebRequest https://www.apache.org/licenses/LICENSE-2.0.txt` 下載官方全文存為 `LICENSE`（202 行，尾部 appendix 完整）。
   - 決策：LICENSE 保持 canonical 全文不動；appendix 的 `[yyyy]/[name]` 佔位留空為標準做法，著作權人資訊改由 README License 章節宣告。
3. 依 Keep a Changelog 格式寫 `CHANGELOG.md`：`[Unreleased]` 的 Added/Changed 涵蓋 CLI 骨架、設定分層、Taskfile、CI/安全管線、Dependabot、測試（85.9%）、README、Apache-2.0。
   - 決策：尚未發布任何版本，故僅有 `[Unreleased]`，不含虛構的版本號章節與 compare 連結。
4. README：badges 加 `[![License]...](LICENSE)`；License 章節替換為 Apache-2.0 宣告 + 授權條款摘錄 code block。
5. 四關驗證（docs 變更仍依 §10.3 強制）。

## 遭遇的困難

- `LICENSE` 副檔名不在 `.gitattributes` 的 eol 規則內（`*.txt` 未列），Windows `core.autocrlf=true`
  環境下 commit 時可能被轉為 CRLF，造成跨平台 diff 噪音。
- CHANGELOG 版本章節取舍：專案尚未發布任何 tag，若寫 `[0.1.0]` 章節與 compare 連結會引用不存在的 release。

## 如何解決

- 於 `.gitattributes` 增補 `LICENSE text eol=lf`，強制 LF；以 `Select-String` 確認工作區檔案無 CR 字元。
- CHANGELOG 僅保留 `[Unreleased]`（Added/Changed），等首次 release（goreleaser 工作項）時再切版本章節。

## 最後變動了什麼

- `LICENSE`（新增）：Apache License 2.0 官方全文（202 行，自 apache.org 下載，canonical 未修改）。
- `CHANGELOG.md`（新增）：Keep a Changelog 1.1.0 + SemVer 宣告；`[Unreleased]` 記錄 CLI 骨架、設定分層、
  exit-code 契約、Taskfile、CI/安全管線、Dependabot、測試（85.9%）、README、Apache-2.0。
- `README.md`（修改）：badges 列新增 License badge；License 章節由 TBD 改為 Apache-2.0 正式宣告
  （Copyright 2026 cwchiu + 授權條款摘錄）。
- `.gitattributes`（修改）：新增 `LICENSE text eol=lf`。
- `specs/20260907-add-license-changelog.md`（新增）：本紀錄。
- commit：`8e2d707` `docs: add Apache-2.0 LICENSE and CHANGELOG`

## 驗證結果

- `go build ./...` — 通過（無輸出）
- `go vet ./...` — 通過（無輸出）
- `go tool golangci-lint run` — `0 issues.`
- `go test -shuffle=on ./...` — `ok github.com/cwchiu/my-cli/cmd/my-cli 0.677s`
- `Select-String -Pattern "\r"` on LICENSE/CHANGELOG.md — 無 CRLF
- （docs-only 變更，無程式碼行為影響；四關依 §10.3 強制執行）
