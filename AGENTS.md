# AGENTS.md — gsc-mcp

給拿到這個 repo 的 AI agent。這支工具是 Google Search Console 的本地 MCP server：
單一 Go binary、stdio、用**使用者自己的 Google 帳號**登入，查他本來就有權限的 GSC 資料。

先分流，兩種任務讀的東西不同：

| 你的任務 | 讀這裡 |
|---|---|
| 幫使用者**把它裝起來** | 本檔「安裝」段 → [INSTALL.md](INSTALL.md)（那份是逐步指引） |
| 已經裝好，要**拿它查資料** | 本檔「使用」段 |

---

## 安裝

### 最短路徑

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
```

```powershell
irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
```

然後**印給使用者自己跑**（會開瀏覽器）：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
gsc-mcp setup
```

裝完跑 `gsc-mcp doctor` 驗證。細節照 [INSTALL.md](INSTALL.md)。

### 四件最常搞砸的事

1. **代跑會開瀏覽器的登入指令。** 印出來給使用者跑，等他回報。
2. **漏掉 `set-quota-project`。** ADC 是個人帳號、不隸屬任何 GCP 專案，少了它每一次查詢都回 403。這是最常見的卡關點。
3. **Windows 自己抓 exe。** 用 `install.ps1`——手抓會跳過 SHA-256 驗證、不清 SmartScreen 封鎖標記、還會裝到專案目錄而不是 PATH。
4. **把憑證檔內容印出來。** 確認存在只用 `ls`，不要 `cat`。

### 驗證用 `gsc-mcp doctor`，不要手刻 JSON-RPC

```bash
gsc-mcp doctor
```

它做完整環境檢查 + **一次真實 `list_sites`**，**不寫任何檔案**，失敗時直接印出該跑的指令。

`gsc-mcp setup --dry-run` **不等於** doctor：它會跳過 API 呼叫，所以答不出「憑證能不能用」。

---

## 使用

### 六支 tool

| Tool | 做什麼 | 關鍵限制 |
|---|---|---|
| `list_sites` | 列出所有可存取的 property | 無參數。**不確定 property 格式時先叫這支** |
| `get_site` | 查單一 property 的權限層級 | — |
| `query_search_analytics` | 核心查詢：clicks / impressions / ctr / position | `row_limit` 預設 **150**，上限 25,000 |
| `compare_periods` | 兩個期間對比含 delta | 兩期天數必須相同，否則 `invalid_input`。`row_limit` 預設 **100** |
| `inspect_url` | 查 Google 索引狀態 | 一次 1–10 個 URL，序列呼叫 |
| `manage_sitemaps` | list / get / submit / delete sitemap | `submit`、`delete` 預設關閉（見下） |

### `site_url` 給什麼都行

會自動正規化，不用先問使用者格式：

| 你給 | 送出去變成 |
|---|---|
| `example.com` | `sc-domain:example.com` |
| `https://example.com` | `sc-domain:example.com` |
| `https://example.com/` | `https://example.com/`（尾斜線＝明示 URL-prefix） |
| `https://example.com/blog` | `https://example.com/blog/` |

猜錯而回 403 時，server 會自動去 `sites.list` 找對的那支重試一次。真的找不到，錯誤訊息會**列出所有可存取的 property**——直接把那份清單拿給使用者看，不要自己猜。

### 讀輸出時最容易誤讀的四件事

**`truncated` 和 `scan_capped` 一定要讀。** `truncated=true` 表示你拿到的是更大集合的前 N 列；`scan_capped=true` 表示連掃描都不完整，top-N 可能漏東西。忽略它們就會把「前 150 名的總和」當成全站數字報給客戶。

**API 只回 top rows，各列加總不等於後台總計。** 這不是 bug，不要試圖用加總去對帳。

**平均排名是加權平均，不是「這個字排第幾」。** `position: 8.4` 的意思是該維度組合所有曝光的加權平均位置，不是排名第 8.4 名。

**`compare_periods` 的方向與單位**：所有 delta 都是 **B 減 A**（A = 較早的基準期）。`ctr_a` / `ctr_b` 是 0–1 小數，但 `ctr_delta_pp` 是**百分點**（0.10 → 0.125 得到 2.5）。`position_change` 為負代表排名**進步**（位置數字變小）。只出現在單一期間的 key 會標 `only_in`，並且**省略** `position_change`——不要把缺值當成 0。

### 幾個會靜默出錯的參數

- **要排除品牌詞**：`dimension_filter_groups` 用 `excludingRegex`（RE2 語法，不支援 lookahead / lookbehind / backreference，送出前會先驗）。
- **要按曝光 / CTR / 排名取 top-N**：一定要給 `sort_by`。GSC API 本身**只能按 clicks 排序**，不給 `sort_by` 就會拿到「剛好點擊數高的那些列」，而不是你要的排名。給了 `sort_by`，server 會掃最多 25,000 列在本地排序——比較慢，但這是唯一正確的方法。

  | Tool | `sort_by` 可填 | 預設方向 |
  |---|---|---|
  | `query_search_analytics` | `clicks`（預設）、`impressions`、`ctr`、`position` | 前三者 desc；`position` 是 asc（位置數字小＝排名好） |
  | `compare_periods` | `clicks_delta`（預設）、`impressions_delta`、`ctr_delta_pp`、`position_change` | delta 類 desc（漲最多在前，要跌最多就給 `sort_order=asc`）；`position_change` 是 asc（進步最多在前） |

  「排名進步最多的字」＝ `compare_periods` 加 `sort_by=position_change`。不給就會拿到一堆點擊數高但排名沒動的列。
- **`dimensions` 含 `hour`**：必須同時 `data_state=hourly_all`，且 `start_date` 在近 10 天內。
- **`data_state` 預設 `all`**，對齊 GSC 後台顯示（含未定案資料）。要嚴謹分析才指定 `final`。
- **日期是 PT 時區**，資料延遲 2–4 天，只保留約 16 個月。查最近三天回空值通常是「還沒有資料」而不是「沒有流量」。

### 寫入預設是關的

| 動作 | 需要 |
|---|---|
| `list` / `get` | 無條件可用 |
| `submit` | `GSC_ENABLE_WRITE=true` |
| `delete` | `GSC_ENABLE_WRITE=true` **且** `GSC_ALLOW_DESTRUCTIVE=true` |

旗標沒開時 tool 仍會出現在 `tools/list`，但寫入動作會回 `write_disabled` 而**不會**呼叫 API。

**ADC 使用者注意**：`GSC_ENABLE_WRITE=true` **對 ADC 無效**。OAuth 的權限範圍在登入當下就寫死在 token 裡，事後擴不了。要寫入必須用 `webmasters`（非 `readonly`）重跑一次登入。

### 現成的 skills

`.agents/skills/` 底下有四個，不用自己想 prompt：

| Skill | 使用者會怎麼問 |
|---|---|
| `nonbrand-performance` | 「除了搜我公司名，還有誰找得到我？」 |
| `monthly-report` | 「這個月網站表現如何？」 |
| `index-health` | 「Google 有沒有看到我的新頁面？」 |
| `gsc-recipes` | 其餘任何問題——問題 → 精確 tool 呼叫的路由表 |

## Agent 原生設定

這個 repo 刻意把每種 agent 的設定放在它會辨識的位置；不要複製成另一套、也不要把任何憑證寫進 repo。

| Agent | 自動讀取的內容 |
|---|---|
| Claude Code | `.mcp.json`、`.claude-plugin/`，以及 plugin 指向的 `.agents/skills/` |
| Codex | 根目錄本檔 `AGENTS.md` 與 `.agents/skills/` |
| Cursor | `.cursor/mcp.json` 與 `.cursor/rules/gsc-mcp.mdc` |
| Hermes | Hermes 目前只讀使用者的 `~/.hermes/config.yaml`；照 [INSTALL.md](INSTALL.md) 的片段合併，repo 內沒有會被它自動載入的設定目錄 |

任何 agent 的第一個真實查詢都應該是 `list_sites`。在不確定 property 時先列出可用 property；不要猜 site URL，也不要讀取或輸出憑證內容。

---

## 故障排除

每一種症狀都會被誤診。先對症狀，不要自己推理成因。

| 症狀 | 真正原因 | 別這樣做 | 處置 |
|---|---|---|---|
| 403，訊息含 `requires a quota project` | ADC 沒設 quota project | 以為使用者沒 property 權限，叫他去 GSC 後台加人 | `gcloud auth application-default set-quota-project YOUR_PROJECT_ID` |
| `auth_failed`，含 `cannot fetch token` 或 `invalid_grant` | refresh token 失效 | 以為 binary 壞了，叫他重裝 | 重跑 `application-default login` |
| 剛登入成功，`gcloud projects list` 卻說憑證過期 | 那是**另一套** token | 再跑一次 `application-default login`（修不到） | 跑 `gcloud auth login` |
| Windows 說 `gcloud` not recognized，但確定裝過 | 裝了但不在 PATH | 重跑 `winget install`（會回 already installed） | 用完整路徑 `%LOCALAPPDATA%\Google\Cloud SDK\google-cloud-sdk\bin\gcloud.cmd` |
| `submit` / `delete` 被拒 | 旗標沒開，或 ADC scope 不含寫入 | 對 ADC 設 `GSC_ENABLE_WRITE=true`（無效） | 見上方「寫入預設是關的」 |
| 查詢回空但確定有流量 | 日期落在 2–4 天延遲區間，或超出 16 個月 | 以為權限有問題 | 把 `end_date` 往前挪幾天再試 |

`gcloud auth login` 和 `gcloud auth application-default login` 是**兩套獨立憑證**，各自登入、各自過期：前者給 `gcloud` CLI 自己用（`projects list` 等），後者給 `gsc-mcp` 這類程式用。兩套都要有。

錯誤碼固定為：`invalid_input`、`auth_failed`、`permission_denied`、`not_found`、`quota_exceeded`、`upstream_error`、`write_disabled`。

診斷第一步永遠是 `gsc-mcp doctor`。要看憑證是從哪裡讀到的（常見盲點：使用者有個忘記的 `GOOGLE_APPLICATION_CREDENTIALS` 蓋掉了 ADC）：

```bash
GSC_LOG_LEVEL=debug gsc-mcp
```

---

## 這支工具不做什麼

問到下面這些，直接說沒有，不要試圖用現有 tool 湊：

- **請求索引**（Indexing API 官方只開放 JobPosting / BroadcastEvent）
- **新增或刪除 property**（`sites.add` 之後仍需 UI 驗證；delete 誤刪風險太高）
- **涵蓋範圍報告、Core Web Vitals、連結報告**——官方 API 根本不存在這些端點
- **列出全站已索引 URL**（`inspect_url` 只能查你已經知道的 URL，一次 10 個）
- **測試線上版本的頁面**（`inspect_url` 回的是 Google 索引裡的版本，會落後線上頁面）

## 硬性邊界

- **憑證內容不得印出。** 確認檔案存在用 `ls`，不要 `cat`，不要貼進對話。
- **錯誤訊息可能含客戶網域與品牌詞**，往外傳之前先看過。
- **不要對真實客戶 property 做寫入操作**來「測試看看」。`submit` / `delete` 是真的會生效的。
- **stdout 只有 MCP JSON-RPC**，所有日誌走 stderr。debug 時看 stderr。
