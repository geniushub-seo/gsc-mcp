<div align="center">

# gsc-mcp

### Google Search Console 的本地 MCP server — 單一 Go binary，執行端零 runtime 依賴

[![ci](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/geniushub-seo/gsc-mcp)](https://github.com/geniushub-seo/gsc-mcp/releases)

[English](README.md) | 繁體中文 | [简体中文](README_ZH_CN.md)

</div>

給 SEO 與內容團隊設計：用你自己的 Google 帳號登入（ADC），本地 agent（Claude Code / Codex / Cursor）就能查你本來就有權限的 Search Console 資料——**不必**為每個 property 加 service account。

## 一行安裝

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gsc-mcp setup
```

`install.sh` 會下載對應平台 binary、驗證 SHA-256、裝到 `~/.local/bin`。之後兩行請自己跑（會開瀏覽器 / 寫 MCP 設定）。詳見 [INSTALL.md](INSTALL.md) 與 [Releases](https://github.com/geniushub-seo/gsc-mcp/releases)。

## 開始使用（ADC，主推）

用你自己的 Google 帳號登入一次，MCP 設定**不需要**任何憑證環境變數。

最省事就是跑上面那三行。或把 repo 給 AI agent，說：照 [INSTALL.md](INSTALL.md) 幫我裝好。

### 自己動手（已裝 binary 之後）

1. **裝 gcloud**（一次）：
   - macOS：`brew install --cask google-cloud-sdk`
   - Linux：`curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-latest-linux-x86_64.tar.gz && tar -xf google-cloud-cli-latest-linux-x86_64.tar.gz && ./google-cloud-sdk/install.sh`（或發行版套件 `google-cloud-sdk`）
   - Windows：`winget install Google.CloudSDK`，或下載 [GoogleCloudSDKInstaller.exe](https://dl.google.com/dl/cloudsdk/channels/rapid/GoogleCloudSDKInstaller.exe)
   - **警告**：不要只解壓 tarball 而不跑 `install.sh`——這樣 bundled Python 不會裝進去，啟動腳本會落到系統 `python3`（macOS 是 3.9，gcloud 需要 3.10–3.14），錯誤訊息會讓人以為是本專案有問題。那 713 MB 是 Google Cloud SDK 的大小，不是本專案的。
2. **ADC 登入**（會開瀏覽器）——上面第二行。
3. **啟用 Search Console API**（若尚未）：
   https://console.cloud.google.com/apis/library/searchconsole.googleapis.com
4. **跑 `gsc-mcp setup`**——合併 MCP 設定並試呼叫 `list_sites`。
5. 在 agent 裡呼叫 `list_sites` 確認看得到哪些 property。

### MCP 設定長這樣（ADC：只要 command）

```json
{
  "mcpServers": {
    "gsc": {
      "command": "/Users/YOUR_USER/.local/bin/gsc-mcp"
    }
  }
}
```

憑證自動從 `~/.config/gcloud/application_default_credentials.json` 讀取。

### ADC refresh token 失效時

會因改密碼、久未使用、組織政策而失效。重新登入即可，不是程式壞了：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
```

### 寫入（submit / delete sitemap）與 ADC

**`GSC_ENABLE_WRITE=true` 對 ADC 無效。** ADC 的 OAuth scope 在 `gcloud auth application-default login` 當下就固定了，事後無法擴大。若要寫入，必須重跑登入並帶上：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform
```

（`webmasters` 含讀寫；唯讀請用 `webmasters.readonly`。）

## 環境變數

| 變數 | 預設 | 說明 |
|---|---|---|
| `GOOGLE_APPLICATION_CREDENTIALS` | — | 官方 ADC 路徑覆寫（優先序 2） |
| `GOOGLE_SERVICE_ACCOUNT_FILE` | — | service account key 路徑（優先序 3） |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | — | inline JSON（優先序 4，適合 CI） |
| `GSC_ENABLE_WRITE` | `false` | **僅 service account**：scope 升級為 `webmasters`，允許 `submit`。ADC 下會警告且無效 |
| `GSC_ALLOW_DESTRUCTIVE` | `false` | 允許 `delete`；需同時 `GSC_ENABLE_WRITE=true`（service account） |
| `GSC_REQUEST_TIMEOUT` | `30s` | 單次 API 呼叫 timeout |
| `GSC_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

憑證優先序：`--credentials-file`（別名 `--service-account-file`）→ `GOOGLE_APPLICATION_CREDENTIALS` → `GOOGLE_SERVICE_ACCOUNT_FILE` → `GOOGLE_SERVICE_ACCOUNT_JSON` → `.env` → **ADC 預設路徑**。

### macOS Gatekeeper

`install.sh` 會嘗試移除 quarantine。若你是從瀏覽器手動下載 binary，仍可能看到「無法驗證開發者」：Finder 對該檔**右鍵 → 打開 → 打開**。本專案不做 Apple 程式碼簽章。

## 進階：service account（headless / CI）

適用非人類身分、CI、或無法使用個人 Google 登入的情況。

1. GCP 啟用 Search Console API → 建 service account → 下載 JSON key。
2. 到 Search Console **每個** property 把 `client_email` 加成使用者。
3. MCP 設定加上 env：

```json
{
  "mcpServers": {
    "gsc": {
      "command": "/Users/YOUR_USER/bin/gsc-mcp",
      "env": {
        "GOOGLE_SERVICE_ACCOUNT_FILE": "/Users/YOUR_USER/.config/gsc-mcp/service-account.json"
      }
    }
  }
}
```

```bash
mkdir -p ~/.config/gsc-mcp && chmod 700 ~/.config/gsc-mcp
chmod 600 ~/.config/gsc-mcp/service-account.json
```

## Skills（開箱即用的分析）

四個 skill 隨 server 附上，不用自己想 prompt：

| Skill | 使用者會怎麼問 | 做什麼 |
|---|---|---|
| `nonbrand-performance` | 「除了搜我公司名，還有誰找得到我？」 | 用 regex query filter（`excludingRegex`）切分品牌與非品牌流量，能一次涵蓋品牌詞的所有拼法變體 |
| `monthly-report` | 「這個月網站表現如何？」 | 月報，內建品牌／非品牌切分 |
| `index-health` | 「Google 有沒有看到我的新頁面？」 | sitemap 狀態、逐頁診斷、canonical 衝突偵測 |
| `gsc-recipes` | 其餘任何問題 | 參數路由表：問題 → 精確的 tool 呼叫 |

Claude Code 會自動載入 `skills/`。其他 client 可把各 `SKILL.md` 的內容當 prompt 用。

## 工具清單

| Tool | 底層 API | 寫入 | 說明 |
|---|---|---|---|
| `list_sites` | `sites.list` | — | 列出所有已授權 property——多客戶的入口 |
| `get_site` | `sites.get` | — | 查單一 property 的 `permissionLevel` |
| `query_search_analytics` | `searchanalytics.query` | — | 核心查詢，完整參數透傳 |
| `compare_periods` | `searchanalytics.query` ×2 | — | server 端封裝，回傳兩期數據加 delta |
| `inspect_url` | `urlInspection.index.inspect` | — | 一次 1–10 個 URL，序列呼叫 |
| `manage_sitemaps` | `sitemaps.list/get/submit/delete` | 條件 | 四合一；submit / delete 需開旗標 |

只有六支是刻意的。總覽、逐頁拆解、「進階」分析其實是**同一個 API 呼叫的不同預設參數**，各做一支的代價是每次請求都要把所有 schema 傳給 LLM，選錯的機率也跟著上升。那些情況交給 `gsc-recipes` 這張路由表，不佔 context。**只有 LLM 自己做不到的運算才值得包成 tool**——例如對 25,000 列算中位數基準。

### 明確不做

- `sites.add` / `sites.delete`：add 後仍需在 UI 驗證，delete 誤刪風險過高。
- Indexing API：Google 只對 `JobPosting` 與 `BroadcastEvent` 開放。
- 涵蓋範圍報告、Core Web Vitals、連結報告：官方 API 不存在。

## 資料特性（這些都寫進了 LLM 讀的 tool description）

- 日期是 **PT 時區**（UTC−7/−8），不是 UTC 也不是台北時間。
- 資料有 **2–4 天延遲**，只保留 **16 個月**；近期日期可能回傳不完整資料。
- API 只回傳 top rows，**不保證完整**；加總不會等於後台的總計。
- `rowLimit` 上限 **25,000**（後台介面一次只給 1,000，這是用 API 的主要好處）。超過要用 `startRow` 分頁。
- `HOUR` 維度必須搭配 `dataState: HOURLY_ALL`，且只有近 **10 天**資料。
- filter 只能作用於 `QUERY` / `PAGE` / `COUNTRY` / `DEVICE` / `SEARCH_APPEARANCE`，**不能** filter `DATE` 或 `HOUR`。
- 若 group 或 filter 了 `page`，`aggregationType` 不能用 `BY_PROPERTY`。
- 平均排名是加權平均，不是「這個字排第幾名」的事實。

`site_url` 可以直接給裸網域（`example.com`）、完整 URL 或標準 `sc-domain:` 格式，server 會正規化；猜錯導致 403 時會自動查詢可存取的 property 重試一次。

## 開發

```bash
go build -trimpath -ldflags="-s -w -X main.version=dev" -o ./bin/gsc-mcp ./cmd/gsc-mcp
go test ./... && go vet ./... && golangci-lint run
```

## 相關文件

- [INSTALL.md](INSTALL.md) — 寫給 AI agent 照著跑的安裝指引
- [SPEC.md](SPEC.md) — 定稿技術規格
- [docs/blog-gsc-mcp.md](docs/blog-gsc-mcp.md) — 安裝與使用情境的白話導覽

---

<div align="center">

由 **[萬智匯 Genius Hub](https://geniushub.cc/)** 開發 · 採 MIT 授權

</div>
