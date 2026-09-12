# 外部 API 整合與資料匯出規範（API Integration Guidelines）

> 本文件定義 `my-cli` 專案在對接第三方 API、Cloud 提供商或產出匯出報表（CSV、JSON、YAML）時的**最高遵循原則**。

---

## 1. 核心原則：Source of Truth 優先

1. **嚴禁憑空推測 API 契約與輸出格式**：
   - 在撰寫任何 API Client 或匯出命令前，必須先取得：
     a. **真實 Web UI Export 檔案**（如使用者由管理後台導出的原始 `.csv` / `.json`），或
     b. **以 curl 實際調用成功的完整 JSON response payload**。
   - 以上兩者為功能的 **Source of Truth**，任何實作必須完全與其對齊。
2. **區分聚合維度**：
   - 清楚區分「摘要型聚合（如 By Framework）」與「明細／規則型聚合（如 By Rule / By Finding）」，勿將摘要 API 誤作明細匯出來源。

---

## 2. CSV 匯出對齊檢核清單

若命令目標為產生可與官方 Web 系統比對的 CSV，必須逐項滿足：

| 檢核項目 | 要求 | 說明 |
|---|---|---|
| **標頭名稱（Header）** | 100% 精確匹配 | 大小寫、空格必須完全一致（例如 `Framework Name Version`，不可擅自改為 `framework_name_version`）。 |
| **欄位順序與數量** | 100% 依序對齊 | 欄位數量不可增減，順序必須與 Web export 相同。 |
| **資料筆數（Count）** | 預設包含全量 | 除非使用者明確要求只留失敗（failed），否則預設應產出與 Web export 相同的全量評估規則（如通過與失敗皆需納入）。 |
| **欄位跳脫規範** | 強制使用標準庫 | 一律使用 Go 標準庫 `encoding/csv.Writer` 處理輸出，確保欄位含有逗號（`,`）或引號（`"`）時自動被 RFC 4180 標準跳脫。 |
| **排序確定性** | 穩定排序 | 匯出結果必須在寫入前進行穩定排序（如依照主鍵 `ID` 遞增排序），確保每次執行產出的 diff 一致且可驗證。 |

---

## 3. Go 型別定義與反序列化安全

1. **百分比與比例欄位**：
   - 任何涉及「百分比（percentage）」、「比例（ratio）」、「平均值（average）」之 API 欄位，Go struct 中一律宣告為 **`float64`**，嚴禁宣告為 `int`。
   - 匯出為 CSV 字串時，建議使用 `strconv.FormatFloat(val, 'f', -1, 64)` 保留原始有效位數，避免小數被截斷。
2. **資產與項目計數**：
   - 明確標示計數欄位（如 `passed_count`, `failed_count`, `total_count`）為 `int`。

---

## 4. API 查詢與分頁防禦

1. **過濾器（Filter）實測驗證**：
   - 部分聚合端點（如 Falcon `/container-compliance/aggregates/rules/v2`）不支援任意 FQL 或 filter 參數（可能回傳 `400 invalid filter`）。
   - 若遠端端點不支援過濾，應透過分頁讀取全量資料後，在 Client 端記憶體中安全篩選。
2. **完整分頁遍歷**：
   - 避免使用固定 limit 只抓取第一頁（如 limit=20）；必須實作 while/for 迴圈，以 `limit` + `offset` 讀取直到全部資料收齊（`offset >= total`）。
