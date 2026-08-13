# SPEC — gsc-mcp v1

定稿技術規格。

## 1. 目標與非目標

**目標**

- 單一原生 binary，執行端零 runtime 依賴
- MCP stdio transport（stdin/stdout JSON-RPC）
- Service Account 認證（多客戶代理商場景，不需 OAuth 瀏覽器流程）
- 預設唯讀；寫入能力需顯式旗標
- 業務邏輯與 transport 解耦，保留未來加 HTTP 的空間

**非目標（v1）**

- HTTP / Streamable HTTP transport
- 使用者互動式 OAuth 流程
- `sites.add` / `sites.delete`
- Indexing API
- 官方 API 不存在的報告（涵蓋範圍、CWV、外部連結）

## 2. 官方 API 全貌

| 服務群 | Method | v1 是否包成 tool |
|---|---|---|
| Search Analytics | `searchanalytics.query` | ✅ |
| Sitemaps | `sitemaps.list` | ✅ |
| Sitemaps | `sitemaps.get` | ✅ |
| Sitemaps | `sitemaps.submit` | ✅（旗標） |
| Sitemaps | `sitemaps.delete` | ✅（雙旗標） |
| Sites | `sites.list` | ✅ |
| Sites | `sites.get` | ✅ |
| Sites | `sites.add` | ❌ |
| Sites | `sites.delete` | ❌ |
| URL Inspection | `urlInspection.index.inspect` | ✅ |

**實作要點**：`urlInspection` 的 base URL 是 `searchconsole.googleapis.com/v1`，其餘是 `www.googleapis.com/webmasters/v3`。使用 `google.golang.org/api/searchconsole/v1` 時**這個差異由生成碼處理**，不需自己拼 URL。

## 3. Tool 規格

### 3.0 六支 tool 的共用行為

#### `site_url` 正規化

所有帶 `site_url` 的 tool 先跑 `NormalizeSiteURL`：

| 輸入 | 輸出 | 依據 |
|---|---|---|
| `sc-domain:example.com` | 原樣 | 已是標準格式 |
| `example.com` | `sc-domain:example.com` | 裸網域推定 Domain property |
| `https://example.com` | `sc-domain:example.com` | 根 URL 無尾斜線 → 推定 Domain |
| `https://example.com/` | `https://example.com/` | **尾斜線 = 呼叫端明示 URL-prefix 意圖** |
| `https://example.com/blog` | `https://example.com/blog/` | 非根路徑 → URL-prefix，補尾斜線 |

`www.` 前綴與 port 在推導 apex 時剝除。

#### 403 property 探索救援

正規化後的呼叫回 **403**（僅 403，其他錯誤直接往上拋）時：

1. 呼叫 `sites.list`
2. 用輸入的 apex domain 比對，在**尚未 403 失敗的候選**裡偏好 `sc-domain:` > URL-prefix
3. 排除步驟 1 剛失敗的那個 URL（`ResolveSiteURL` 的 `exclude`），找到就用解析結果重試一次
4. 找不到則回 `permission_denied`，**訊息中列出所有可存取的 property**

同一 apex 下若 `sc-domain:` 無權限而 URL-prefix 有權限，救援必須落到後者，不得再挑回剛 403 的那支。

用泛型包裝器 `WithResolvedSiteURL[T]` 實作，六支共用一條路徑，不逐支重寫。

#### Stringified array 參數修復

以 `srv.AddReceivingMiddleware` 在 SDK schema 驗證**之前**修復 client 把陣列編碼成字串的 bug。維護表：

| Tool | 欄位 |
|---|---|
| `query_search_analytics` | `dimensions`、`dimension_filter_groups` |
| `compare_periods` | `dimensions` |
| `inspect_url` | `urls` |

判斷保守：僅當值是 JSON 字串**且**該字串本身可解析成 JSON 陣列時才改寫，其餘放行給正常驗證。

#### 共用輸出欄位

每支 tool 的成功輸出都含 `queried_at`（UTC RFC3339），以及實際送出的 `site_url`（正規化或救援後的值，不是使用者原始輸入）。

### 3.1 `query_search_analytics`

`POST searchAnalytics/query`

| 參數 | 型別 | 必填 | 預設 | 備註 |
|---|---|---|---|---|
| `site_url` | string | ✅ | — | 支援 `sc-domain:` |
| `start_date` / `end_date` | string | ✅ | — | `YYYY-MM-DD`，PT 時區 |
| `dimensions` | string[] | — | `[]` | `query` `page` `country` `device` `date` `hour` `searchAppearance` |
| `search_type` | string | — | `web` | `web` `image` `video` `news` `discover` `googleNews` |
| `dimension_filter_groups` | object[] | — | — | 完整透傳；operator 見下 |
| `aggregation_type` | string | — | `auto` | `auto` `byProperty` `byPage` `byNewsShowcasePanel` |
| `row_limit` | int | — | `150` | **1–25,000**；匯出請顯式指定 |
| `start_row` | int | — | `0` | 分頁 offset |
| `data_state` | string | — | **`all`** | `all` `final` `hourly_all` |

Filter operator：`equals` `notEquals` `contains` `notContains` `includingRegex` `excludingRegex`（RE2 語法）

**約束（必須在 handler 驗證，不能只靠 Google 回錯）**

- `row_limit` 超過 25,000 → `invalid_input`
- `dimensions` 含 `hour` 但 `data_state != hourly_all` → `invalid_input`，訊息說明 HOUR 需要 HOURLY_ALL 且只有近 10 天資料
- filter 的 `dimension` 只允許 `query` `page` `country` `device` `searchAppearance`；填 `date` 或 `hour` → `invalid_input`
- group 或 filter 了 `page` 時，`aggregation_type` 不得為 `byProperty` → `invalid_input`
- `start_date > end_date` → `invalid_input`

**`data_state` 預設為 `all` 的理由**：對齊 GSC 後台顯示，避免代理商交付報表時被客戶質疑「數字跟後台對不上」。要嚴謹分析時 LLM 可顯式指定 `final`。

**Description 必寫**

- 日期為 PT 時區、資料有 2–4 天延遲、僅保留 16 個月
- API 只回 top rows，不保證完整；各列總和不等於後台總計
- `row_limit` 上限 25,000（後台介面一次只給 1,000）
- 平均排名是加權平均，非「該字排第幾」的事實
- 非品牌詞分析範例：用 `excludingRegex` 排除品牌詞

**輸出**

```json
{
  "site_url": "sc-domain:example.com",
  "start_date": "2026-07-01",
  "end_date": "2026-07-31",
  "dimensions": ["query", "page"],
  "row_count": 842,
  "rows": [
    { "keys": ["關鍵字", "https://example.com/page/"], "clicks": 42, "impressions": 1200, "ctr": 0.035, "position": 8.4 }
  ]
}
```

### 3.2 `compare_periods`

Server 端封裝，內部呼叫兩次 `searchanalytics.query`。非官方 API。

參數：`site_url`、`period_a_start`、`period_a_end`、`period_b_start`、`period_b_end`、`dimensions`、`search_type`、`row_limit`（預設 100）、`data_state`

輸出每列含：A 期與 B 期的 clicks / impressions / ctr / position，以及各自的絕對差與百分比差。只出現在單一期間的 key 也要列出（另一期補 0，並標記 `only_in`）。

理由：SEO 月報最高頻需求。讓 LLM 呼叫兩次再自己心算，token 貴且算錯機率高。

### 3.3 `inspect_url`

`POST urlInspection/index:inspect`

| 參數 | 型別 | 必填 | 預設 |
|---|---|---|---|
| `site_url` | string | ✅ | — |
| `urls` | string[] | ✅ | — |
| `language_code` | string | — | `zh-TW` |

`urls` 長度 1–10。server 內部**序列**呼叫，間隔 ≥100ms，不併發。

回傳精簡欄位（原始 response 很肥，不要整包丟給 LLM）。已從 `searchconsole-gen.go` 逐欄驗證：

`UrlInspectionResult` 頂層有 `indexStatusResult`、`inspectionResultLink`、`mobileUsabilityResult`、`richResultsResult`、`ampResult`——**後三者是 optional 指標，Google 對不適用的 URL 直接省略，取值前必須判 nil**。

`IndexStatusInspectionResult` 全欄位：`verdict`、`coverageState`、`indexingState`、`crawledAs`、`lastCrawlTime`、`pageFetchState`、`robotsTxtState`、`googleCanonical`、`userCanonical`、`referringUrls[]`、`sitemap[]`。

全部保留（這層本來就精簡），另加 `inspectionResultLink` 與三個 optional 區塊的 `verdict` 摘要。不回傳 `richResultsResult.detectedItems` 的完整明細。

Description 必寫：**不要寫死配額數字**，改連結官方 limits 頁（數字會變，寫死會過期）。並用「它不做什麼」列舉法：不測試線上版本、不請求索引、不列舉全站已索引 URL、不等同後台的網頁索引狀態報告。

### 3.4 `list_sites`

`GET sites`。無參數。回傳 `siteUrl` + `permissionLevel` 清單。多客戶入口 tool，description 要引導 LLM「不確定 property 格式時先呼叫這支」。

**InputSchema 必須顯式寫死**，不能讓 SDK 從 `struct{}` 推導：

```go
var listSitesInputSchema = json.RawMessage(
    `{"type":"object","properties":{},"required":[],"additionalProperties":false}`)
```

推導出的 schema 缺 `properties` / `required` / `additionalProperties`，嚴格 client（Copilot CLI）會拒絕整個 tool list，導致**所有** tool 不可用。

### 3.5 `get_site`

`GET sites/{siteUrl}`。參數 `site_url`。回傳 `permissionLevel`。

### 3.6 `manage_sitemaps`

四支合併。

| 參數 | 型別 | 必填 | 說明 |
|---|---|---|---|
| `site_url` | string | ✅ | |
| `action` | string | ✅ | `list` / `get` / `submit` / `delete` |
| `feedpath` | string | 條件 | `get` / `submit` / `delete` 必填 |
| `sitemap_index` | string | — | 僅 `list` 使用 |

`list` / `get` 回傳：`path`、`lastSubmitted`、`lastDownloaded`、`isPending`、`isSitemapsIndex`、`warnings`、`errors`、`contents[]`

旗標檢查在**進 API 之前**做，回 `write_disabled` 並在 message 說明要設哪個環境變數。

## 4. 認證

**分發模式：使用者自備憑證。** 不做集中式 OAuth app（我方持有 OAuth client、使用者只需登入）。理由：External 發布需通過 Google 驗證審核（隱私權政策、首頁、網域所有權、示範影片、數週等待），Testing 模式則要手動維護測試者名單且 refresh token 7 天過期——兩者都與「零維護分發」相衝。

「自備憑證」有**兩種**形態，兩者都要支援。

### 4.1 兩種憑證型別

| | `authorized_user`（ADC） | `service_account` |
|---|---|---|
| 定位 | **主推**，文件放前面 | 進階，文件放後面 |
| 適用 | 一般使用者、個人分析 | headless / CI、需要非人類身分、未來的 HTTP transport |
| 身分 | 使用者自己的 Google 帳號 | 專用的機器帳號 |
| 取得方式 | `gcloud auth application-default login --scopes=...` 跳瀏覽器登入 | GCP console 建帳號 + 下載 JSON key |
| **加進 GSC property** | **不需要**——帳號本來就有權限的 property 全部看得到 | 每個 property 逐一把 `client_email` 加為使用者 |
| 前置安裝 | 需要 gcloud CLI（一次） | 無 |
| 憑證壽命 | refresh token 可能因改密碼／久未使用／組織政策失效 | key 不過期 |
| 綁定風險 | 綁在一個人的帳號上，該帳號異動即全斷 | 無 |

ADC 免掉「把 email 加進每個 property」這一步，那是 service account 路徑最痛的部分。這也是它成為主推的唯一理由。

### 4.2 憑證優先序

| # | 來源 | 說明 |
|---|---|---|
| 1 | CLI `--credentials-file` | 明示指定，最高優先。保留 `--service-account-file` 為別名以相容既有設定 |
| 2 | `GOOGLE_APPLICATION_CREDENTIALS` | Google 官方標準的 ADC 環境變數名 |
| 3 | `GOOGLE_SERVICE_ACCOUNT_FILE` | 本專案原有名稱，維持相容 |
| 4 | `GOOGLE_SERVICE_ACCOUNT_JSON` | inline JSON，給容器與 CI |
| 5 | `.env` 檔 | 開發便利，讀上述任一變數名 |
| 6 | **ADC 預設路徑** | `$HOME/.config/gcloud/application_default_credentials.json`（Windows 為 `%APPDATA%\gcloud\...`）。**這一層讓 ADC 使用者完全不必設任何環境變數** |

第 6 層是「零設定」的關鍵：跑完 `gcloud auth application-default login` 之後，MCP 設定檔只需要 `command`，不需要 `env` 區塊。

### 4.3 型別分派

從憑證 JSON 讀 `type` 欄位分派，不要寫死：

| `type` 值 | 傳入的 `option.CredentialsType` |
|---|---|
| `service_account` | `option.ServiceAccount` |
| `authorized_user` | `option.AuthorizedUser` |
| 其他 | 回明確錯誤，列出支援的兩種 |

用 `option.WithAuthCredentialsJSON(credType, json)`。**不要用已 deprecated 的 `option.WithCredentialsJSON`**（官方標註 security risk）。

### 4.4 `quota_project_id`

ADC 使用者憑證**沒有隱含的配額專案**，少了它 Google 會拒絕請求。憑證 JSON 若含 `quota_project_id`，必須傳 `option.WithQuotaProject(id)`。

`service_account` 憑證自帶專案，不需要這個。

### 4.5 Scope：兩種型別行為不同

| 條件 | `service_account` | `authorized_user`（ADC） |
|---|---|---|
| 預設 | `webmasters.readonly` | 登入時決定 |
| `GSC_ENABLE_WRITE=true` | 升級為 `webmasters` | **無效**——見下 |

**ADC 的 scope 在 `gcloud auth application-default login` 當下就固定在 refresh token 上。** OAuth 的 refresh grant 不允許擴大 scope，所以事後傳 `option.WithScopes` 加不上去。

這會製造一個**靜默失效的旗標**：ADC 使用者設 `GSC_ENABLE_WRITE=true`，程式以為開了寫入，Google 卻回 403。這與 R1 的 `RequestTimeout` 死旗標、R3 的 `searchType` 靜默失效是同一類問題，必須處理：

- ADC 模式下若 `GSC_ENABLE_WRITE=true`，啟動時發 `slog.Warn`，說明 ADC 的 scope 由登入決定，要寫入必須重跑登入並在 `--scopes` 帶上 `https://www.googleapis.com/auth/webmasters`
- `README.md` 與 `INSTALL.md` 都要寫明這件事
- 需要一條測試證明該警告會出現（新增設定項要有測試證明它真的改變行為）

ADC 使用者要用讀取功能，登入時的 scope 至少要包含：

```
https://www.googleapis.com/auth/webmasters.readonly
```

### 4.6 其他

token 快取與 refresh 由 `golang.org/x/oauth2` 處理，**不要自己實作 JWT 簽章或 token 交換**。

啟動時若憑證無效或找不到：往 stderr 印一則清楚錯誤（要指出檢查過哪些來源），non-zero 退出，不要進 MCP loop。

## 5. 錯誤模型

工具層錯誤回 `mcp.CallToolResult{IsError: true}`，body 為下列 JSON，**不回 Go error**。

- 回 Go error → 變成 protocol error，LLM 收到無法行動的東西
- 回成功 result 但漏設 `IsError` → client 無法在協定層分辨成敗

Go error 只保留給「連 result 都組不出來」的情況。

```json
{
  "error": "<code>",
  "message": "<人類可讀，已 sanitize，≤300 字元>",
  "suggestion": "<下一步該做什麼>"
}
```

上游 error body 只取 HTTP status 與 Google 的 `message` 欄位，不整包回傳——請求可能含客戶品牌詞（`dimension_filter_groups`）或客戶網址（`urls`）。

| code | 觸發 |
|---|---|
| `invalid_input` | 參數驗證失敗（含約束檢查） |
| `auth_failed` | 401 |
| `permission_denied` | 403，service account 未被加進該 property |
| `not_found` | 404 |
| `quota_exceeded` | 429 或退避重試耗盡 |
| `upstream_error` | 5xx 或無法解析的回應 |
| `write_disabled` | 旗標未開卻呼叫寫入 action |

## 6. 規模基準

本節只保留規格層面的估算基準。

### 人工閘（不計輪數，建議現在就做完）

建 GCP 專案 → 啟用 Search Console API → 建 service account 並下載 key → 把 email 加進至少一個真實 property。約五分鐘，步驟見 `README.md`「開始使用」。

驗收標準 2、3、4 全部卡在這裡；第 1–2 輪就需要真實憑證來抓契約 fixture。

**這同時是每個使用者的一次性設定**——本專案採「使用者自備憑證」的分發模式（與參考實作相同），不做集中式 OAuth app。因此 `README.md` 的三步驟說明是**產品的一部分**，不是附屬文件：寫不清楚等於產品不能用。輪 8 的「README 對齊」要照著它實際跑一次驗證。

### 規模基準

參考實作（`../old-refence`，4 支 tool、參數不完整、無配額防護）實測：產品碼 1,349 行（含我們不做的 HTTP transport 253 行）、測試碼 2,123 行，**測試:產品約 1.6 : 1**。

本專案是 6 支 tool、完整參數面、retry、配額計數、結構化錯誤、雙旗標，但改用 `google.golang.org/api/searchconsole/v1` 省掉參考實作 `client.go` 裡近 400 行 HTTP 拼裝：

下表的估計欄是規劃時寫的，實測欄量於 2026-08-07、HEAD `ac53455`：

| 區塊 | 原估計 | 實測 |
|---|---|---|
| `cmd/` + `internal/config` | ~230 | 428 |
| `internal/mcpfix`（stringified args） | ~90 | 88 |
| `internal/gscclient`（client / siteurl / retry / quota / errors / models） | ~720 | 588 |
| `internal/tools`（6 支 handler + validate + register + description 常數） | ~980 | 1,575 |
| `internal/setup`（`gsc-mcp setup`，規劃時不存在） | — | 365 |
| **產品碼小計** | **~2,000** | **3,044** |
| 測試（1.6× + 契約 fixture） | ~3,200 | 3,409 |
| Makefile / `.golangci.yml` / CI workflow / `install.sh` | ~150 | 286 |
| **總計** | **約 5,400 行** | **6,453** |

產品碼超估 52%，主要在 `internal/tools`（description 常數與約束檢查比預期厚）與規劃時不存在的 `internal/setup`。測試:產品實際為 1.12 : 1，低於參考實作的 1.6 : 1。

`internal/tools/validate.go` 是單一最大檔，現況 372 行（規劃時估 ~180）。**若超過 400 行，拆成 `validate_enum.go` 與 `validate_constraints.go`。** 原本寫 250 行，該線在無人察覺的情況下早已跨過——門檻要維持在跨線時真的會被注意到的位置，否則等於沒有門檻。

**人工閘**（不計入 session 輪數）：建立 GCP 專案、啟用 API、把 service account 加進真實 property——這些要使用者自己在 Google 介面做完才能跑端到端驗證。

## 7. v2 候選（v1 穩定後再做）

以下都是本地運算編排，不是新的 Google API：

| Tool | 用途 |
|---|---|
| `find_ctr_opportunities` | 高曝光、排名 4–15、CTR 低於同位置 baseline 的機會 |
| `find_content_decay` | 兩期之間點擊/曝光衰退的頁面 |
| `find_keyword_cannibalization` | 同一 query 被多頁承接且曝光重疊顯著 |
| `find_low_hanging_keywords` | 接近第一頁、曝光量足夠的字 |
| `get_page_query_matrix` | query × page 矩陣 |

這些 tool 必須：在輸出 metadata 說明評分公式與假設；附上支撐建議的原始或彙總列；不得把 GSC 平均排名當成逐字排名事實；計算需可用 fixture 決定性測試。

## 8. 官方文件索引

- API Reference：`developers.google.com/webmaster-tools/v1/api_reference_index`
- searchAnalytics.query：`developers.google.com/webmaster-tools/v1/searchanalytics/query`
- 配額：`developers.google.com/webmaster-tools/limits`
- urlInspection.index.inspect：`developers.google.com/webmaster-tools/v1/urlInspection.index/inspect`
- Go client 參考：`pkg.go.dev/google.golang.org/api/searchconsole/v1`
- MCP Go SDK：`pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp`
