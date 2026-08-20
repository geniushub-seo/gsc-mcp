<div align="center">

# gsc-mcp

### Google Search Console 的本地 MCP server — 單一 Go binary，執行端零 runtime 依賴

[![ci](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/geniushub-seo/gsc-mcp)](https://github.com/geniushub-seo/gsc-mcp/releases)

[English](README.md) | 繁體中文 | [简体中文](README_ZH_CN.md)

</div>

給 SEO 與內容團隊設計：用你自己的 Google 帳號登入（ADC），本地 agent（Claude Code / Codex / Cursor / Hermes）就能查你本來就有權限的 Search Console 資料——**不必**為每個 property 加 service account。

## 一行安裝

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
gcloud auth login
gcloud projects list
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project PROJECT_ID
gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID
gsc-mcp doctor
gsc-mcp setup
```

**Windows（PowerShell）**

```powershell
irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
gcloud auth login
gcloud projects list
gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project PROJECT_ID
gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID
gsc-mcp doctor
gsc-mcp setup
```

安裝腳本會下載對應平台 binary、**驗證 SHA-256**、裝到 `~/.local/bin`（Windows 是 `%LOCALAPPDATA%\Programs\gsc-mcp`），並解除 macOS quarantine / Windows SmartScreen 封鎖標記。若由 AI agent 協助，所有終端指令（包括啟動 gcloud 登入）都由 agent 執行；使用者只需在自動開啟的 Google 網頁選帳號並按下允許，不必開啟終端或複製指令。詳見 [INSTALL.md](INSTALL.md) 與 [Releases](https://github.com/geniushub-seo/gsc-mcp/releases)。

`PROJECT_ID` 換成 `gcloud projects list` 輸出裡的專案 ID，不要把佔位字原樣送出。**那兩行都不能省**——ADC 是個人帳號、不隸屬任何專案，少了 quota project 所有查詢會回 403 `requires a quota project`，看起來很像權限問題但不是；而全新的專案本來就沒有啟用 Search Console API，少了 `gcloud services enable` 會回同一個 403、細節裡寫 `"reason": "SERVICE_DISABLED"`。另外 `gcloud auth login` 跟 ADC 登入是兩次獨立登入，少了它 `gcloud projects list` 會說憑證過期。

裝完有任何一步不順，跑 `gsc-mcp doctor`——完整檢查加一次真實 `list_sites`，不寫任何檔案。

## 開始使用（ADC，主推）

用你自己的 Google 帳號登入一次，MCP 設定**不需要**任何憑證環境變數。

手動安裝可跑上面的指令。若交給 AI agent，只要說「照 [INSTALL.md](INSTALL.md) 幫我裝好」；agent 必須負責所有終端操作，使用者只負責 Google 瀏覽器授權。

### 自己動手（已裝 binary 之後）

1. **裝 gcloud**（一次）：
   - macOS：`brew install --cask google-cloud-sdk`
   - Linux：`curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-latest-linux-x86_64.tar.gz && tar -xf google-cloud-cli-latest-linux-x86_64.tar.gz && ./google-cloud-sdk/install.sh`（或發行版套件 `google-cloud-sdk`）
   - Windows：`winget install Google.CloudSDK`，或下載 [GoogleCloudSDKInstaller.exe](https://dl.google.com/dl/cloudsdk/channels/rapid/GoogleCloudSDKInstaller.exe)
   - **警告**：不要只解壓 tarball 而不跑 `install.sh`——這樣 bundled Python 不會裝進去，啟動腳本會落到系統 `python3`（macOS 是 3.9，gcloud 需要 3.10–3.14），錯誤訊息會讓人以為是本專案有問題。那 713 MB 是 Google Cloud SDK 的大小，不是本專案的。
2. **啟用 Search Console API**（若尚未）：於 [API Library](https://console.cloud.google.com/apis/library/searchconsole.googleapis.com) 按 Enable。先確認左上角選到的是你要用的專案，並記下它的專案 ID（小寫那串，不是顯示名稱）。
3. **ADC 登入**（會開瀏覽器）——上面第二行。
4. **設 quota project**——上面第三行。ADC 必做，漏掉每次查詢都會 403。
5. **跑 `gsc-mcp setup`**——合併 MCP 設定並試呼叫 `list_sites`。
6. 在 agent 裡呼叫 `list_sites` 確認看得到哪些 property。

出問題時跑 `gsc-mcp doctor`：完整檢查加一次真實 `list_sites`，**不寫任何檔案**，並針對失敗原因給出對應指令（缺 quota project、token 失效、API 未啟用各有各的處置）。`setup --dry-run` 會跳過 API 呼叫，答不出憑證能不能用，別拿它當診斷工具。

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

寫入有兩道獨立閘門，ADC 兩道都要過：

1. 設定 `GSC_ENABLE_WRITE=true`。這是 gsc-mcp 的**本地閘門**，對 ADC 與 service account 都生效。只重登 write scope 不夠。
2. ADC token 還必須在登入當下就帶 `webmasters` scope（事後無法擴大）。重跑登入並帶上：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform
```

（`webmasters` 含讀寫；唯讀請用 `webmasters.readonly`。）

## 環境變數

ADC 不需要設任何憑證變數——照上面裝好，binary 會自己讀
`~/.config/gcloud/application_default_credentials.json`。下表全部是選填。

| 變數 | 預設 | 說明 |
|---|---|---|
| `GOOGLE_APPLICATION_CREDENTIALS` | — | 覆寫 ADC 憑證檔路徑；預設路徑可用時不需要設 |
| `GSC_ENABLE_WRITE` | `false` | 本地寫入閘門，允許 `submit`。ADC 另需 token 本身帶 `webmasters` scope（見上方「寫入（submit / delete sitemap）與 ADC」） |
| `GSC_ALLOW_DESTRUCTIVE` | `false` | 允許 `delete`；需同時 `GSC_ENABLE_WRITE=true` |
| `GSC_REQUEST_TIMEOUT` | `30s` | 單次 API 呼叫 timeout |
| `GSC_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

service account 的憑證變數與完整優先序見下方「進階：service account（headless / CI）」；一般安裝不需要。

### macOS Gatekeeper

`install.sh` 會嘗試移除 quarantine。若你是從瀏覽器手動下載 binary，仍可能看到「無法驗證開發者」：Finder 對該檔**右鍵 → 打開 → 打開**。本專案不做 Apple 程式碼簽章。

## 進階：service account（headless / CI）

**一般使用者不需要這節。** 只有下列情況才走這條路：手上已經有 service account JSON
key、跑在 CI 或無人機器上、目標機器開不了瀏覽器、或需要非人類身分。否則請用上方的
ADC——它不必為每個 property 加使用者。

代價先講明：service account 是機器帳號，GSC 不會自動認它，**每一個**要查的 property
都得手動把它的 `client_email` 加成使用者。換來的是寫入比 ADC 單純：`GSC_ENABLE_WRITE=true`
會直接把 scope 升級為 `webmasters`，不必像 ADC 那樣重跑登入。

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

### service account 專用變數與憑證優先序

| 變數 | 說明 |
|---|---|
| `GOOGLE_SERVICE_ACCOUNT_FILE` | key 檔路徑 |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | inline JSON，適合把密鑰存成字串的 CI |

完整優先序（前五層都是顯式指定，全都沒設才落到 ADC）：`--credentials-file`
（別名 `--service-account-file`）→ `GOOGLE_APPLICATION_CREDENTIALS` →
`GOOGLE_SERVICE_ACCOUNT_FILE` → `GOOGLE_SERVICE_ACCOUNT_JSON` → `.env` →
**ADC 預設路徑**。權威定義見 [SPEC.md](SPEC.md) §4.2。

## 各 agent 的原生上手方式

想讓 agent 讀到專案專屬設定與教學時，clone 此 repo。這些設定都不含憑證或本機絕對路徑。

| Agent | 原生 repo 檔案 | 怎麼用 |
|---|---|---|
| Claude Code | `.mcp.json`、`.claude-plugin/`、`.agents/skills/` | 開啟 repo 或安裝 plugin。 |
| Codex | `AGENTS.md`、`.agents/skills/` | 從 repo 根目錄啟動 Codex。 |
| Cursor | `.cursor/mcp.json`、`.cursor/rules/gsc-mcp.mdc` | 將 repo 當專案開啟。 |
| Hermes | [`.hermes/`](.hermes/) 上手工具 | 請 Hermes 依 `.hermes/ONESHOT.md` 安裝；所有終端操作由 Hermes 執行，成功後開新的 session。 |

所有 client 的共同驗收：先跑 `gsc-mcp doctor`，再叫 agent 呼叫 `list_sites`。不要把 OAuth token 或 service-account key 放入這些檔案。

## Skills（開箱即用的分析）

四個 skill 隨 server 附上，不用自己想 prompt：

| Skill | 使用者會怎麼問 | 做什麼 |
|---|---|---|
| `nonbrand-performance` | 「除了搜我公司名，還有誰找得到我？」 | 用 regex query filter（`excludingRegex`）切分品牌與非品牌流量，能一次涵蓋品牌詞的所有拼法變體 |
| `monthly-report` | 「這個月網站表現如何？」 | 月報，內建品牌／非品牌切分 |
| `index-health` | 「Google 有沒有看到我的新頁面？」 | sitemap 狀態、逐頁診斷、canonical 衝突偵測 |
| `gsc-recipes` | 其餘任何問題 | 參數路由表：問題 → 精確的 tool 呼叫 |

Claude Code plugin 與 Codex 都會讀取共用的 `.agents/skills/`。Cursor 有同一套安全預設的 project rule；需要特定分析時，agent 可讀對應的 `SKILL.md`。

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

---

<div align="center">

由 **[萬智匯 Genius Hub](https://geniushub.cc/)** 開發 · 採 MIT 授權

</div>
