# 20260913 — fix-ip-lookup-test-race

## Why

PR #13（cert-info）的 CI 在 Test (Go 1.26) 與 Test (Go stable) 兩個 check 上出現
`WARNING: DATA RACE`，導致該 PR 無法合併。race 的來源不是 cert-info 的程式碼，
而是 PR #14（ip-lookup）合入 main 的測試輔助函式 `startIPInfoTestServer`：
`httptest` 的 handler goroutine 會並行地對 `*paths` 做 append（read/write race，
`ip_lookup_test.go:51`），在 Linux runner 的 `-race` 模式下被偵測到。
本機（Windows，無 gcc）跑 `-shuffle=on` 不會開 race detector，因此 PR #14 合併前
本機四關全綠也沒抓到——只有 CI 的 `-race` 會爆。

## What

- 修復 `startIPInfoTestServer` 的資料競爭，讓 CI 的 `-race` 測試穩定通過。
- 非目標：不改動任何 production code；不調整 CI workflow。

## How

1. `*paths` 的 append 加 `sync.Mutex` 保護。
2. handler 內以 `sync.WaitGroup` 追蹤 in-flight 請求；helper 回傳
   `(url, wait, paths)`，呼叫端在驅動完命令、讀取 paths 斷言前先 `wait()`，
   確保 handler goroutine 全部結束（避免 cleanup 後仍寫入）。
3. 更新所有呼叫端到新簽名（`TestIPLookupCommand`、`TestIPLookupRequestPath`、
   `TestIPLookupSkipsEmptyFields`、`TestIPLookupErrors`）。

## 遭遇的困難

1. **vet 編譯錯誤**：改簽名後有 4 個呼叫端仍是兩值接收
   （`assignment mismatch: 2 variables but startIPInfoTestServer returns 3 values`）。
2. **wsl_v5 lint 錯誤 ×3**：`var mu` / `paths` / `var wg` 連續宣告被
   `never cuddle decl` / `missing whitespace` 擋下；handler 內
   `mu.Lock(); append; mu.Unlock()` 被 `no shared variables above append` 擋下。
3. **本機無法驗證 race 修復本身**：`-race` 需要 cgo（gcc），Windows 本機沒有，
   只能靠 CI 驗證。

## 如何解決

1. 逐一更新呼叫端為三值接收（不需要 wait 的呼叫端用 `_, _` 捨棄）。
2. 宣告改為單一 `var ( mu sync.Mutex; wg sync.WaitGroup )` 區塊並與
   `paths` 之間留空行；handler 內改用 `defer mu.Unlock()`。
3. 本機以 `go test -shuffle=on -count=5 ./cmd/my-cli/` 壓力測試提高信心，
   race detector 的最終驗證交給 CI（Linux runner 有 gcc）。

## 最後變動了什麼

- `cmd/my-cli/ip_lookup_test.go`（+27/−10，commit `b88d4e9`）：
  - `startIPInfoTestServer` 簽名改為 `(t, status, body) (string, func(), *[]string)`，
    內部加 mutex + WaitGroup。
  - `TestIPLookupRequestPath` 在兩次斷言前各呼叫 `wait()`。
  - 其餘呼叫端改為三值接收。

## 驗證結果

- `go build ./...`：乾淨。
- `go vet ./...`：乾淨。
- `go tool golangci-lint run`：0 issues。
- `go test -shuffle=on ./...`：ok（cmd/my-cli 1.507s、falconcis 0.622s、nexus 0.929s）。
- `go test -shuffle=on -count=5 ./cmd/my-cli/`：ok（3.624s）。
- `go test -race`：本機不可用（無 cgo/gcc），由 CI 驗證。
- CI（PR #17）：Test (Go 1.26)、Test (Go stable)、Lint、SAST（CodeQL/gosec）、
  SCA（govulncheck/osv-scanner）、Secrets（gitleaks）全數 pass——
  即原本在 PR #13 上失敗的兩個 Test check 修復。
