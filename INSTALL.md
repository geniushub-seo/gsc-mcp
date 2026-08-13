# INSTALL — 給 AI agent 的安裝指引

**這份文件是寫給 AI agent 讀的。** 使用者把整個 repo 交給你，要你幫他把這支 MCP server 裝好。照下面做。

> 若你是要**修改本專案程式碼**的 agent，這份不是給你的——請讀 [AGENTS.md](AGENTS.md)。

---

## 首選：一行安裝

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
```

Windows（PowerShell）：

```powershell
irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
```

**Windows 一定要用 `install.ps1`，不要自己 `Invoke-WebRequest` 抓 binary。** 手動下載會
跳過 SHA-256 驗證，也不會清掉 SmartScreen 的封鎖標記，而且通常會裝進當前專案目錄而非
PATH，害後面每個 MCP 設定都得寫死絕對路徑。

先 dry-run 看會做什麼：

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash -s -- --dry-run
```

```powershell
$env:DRY_RUN = '1'; irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
```

安裝腳本只負責：判平台、下載 release、驗 SHA-256、裝到 `~/.local/bin`（Windows 是
`%LOCALAPPDATA%\Programs\gsc-mcp`）、解除 quarantine / SmartScreen 封鎖、PATH 提示。

**不會**跑 `gcloud login`、不會改 MCP 設定、不會改 shell rc。

接著請使用者自己跑（印指令，不代跑）：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
gsc-mcp setup
```

第二行不能省，理由見步驟 1c。裝完或出錯時跑 `gsc-mcp doctor` 驗證（完整檢查 + 真實
`list_sites`，不寫檔）。

---

## 你能做什麼、不能做什麼

| 你可以自己做 | 必須由使用者本人做 |
|---|---|
| 跑 `install.sh` / `gsc-mcp setup` | 瀏覽器完成 `gcloud auth application-default login` |
| 合併 MCP 設定（先備份） | 在 Cloud Console 啟用 Search Console API（若尚未） |
| 驗證 `list_sites` | 重啟 Claude Desktop 等 GUI |
| 偵測 gcloud / ADC 是否存在 | — |

**主推路徑是 ADC（個人 Google 帳號）**，不必建 service account，也不必把 email 加進每個 property。

---

## 步驟 0：先問使用者

1. 要用哪個 AI 工具？（Claude Desktop / Claude Code / Codex / Cursor / Hermes / VS Code）
2. 機器上有沒有 gcloud？（`which gcloud`）
3. 是否已跑過 `gcloud auth application-default login`？

若使用者明確要 CI / headless，改走文末「進階：service account」。

---

## 步驟 1：gcloud + ADC 登入

### 1a. 安裝 gcloud（若沒有）

- macOS：`brew install --cask google-cloud-sdk`
- Linux：
  ```bash
  curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-latest-linux-x86_64.tar.gz
  tar -xf google-cloud-cli-latest-linux-x86_64.tar.gz
  ./google-cloud-sdk/install.sh
  ```
  或發行版套件 `google-cloud-sdk`。
- Windows：`winget install Google.CloudSDK`，或下載 [GoogleCloudSDKInstaller.exe](https://dl.google.com/dl/cloudsdk/channels/rapid/GoogleCloudSDKInstaller.exe)。

**警告**：不要只解壓 tarball 而不跑 `install.sh`——這樣 bundled Python 不會裝進去，啟動腳本會落到系統 `python3`（macOS 是 3.9，gcloud 需要 3.10–3.14），錯誤訊息會讓人以為是 `gsc-mcp` 有問題。下載的 ~713 MB 是 Google Cloud SDK 的大小，不是本專案的。

**不要自動執行會開瀏覽器的指令。** 把指令印給使用者自己跑。

### 1b. 請使用者登入 ADC

把下面原文貼給使用者：

> 請在終端機執行（會開瀏覽器登入你的 Google 帳號）：
>
> ```bash
> gcloud auth application-default login \
>   --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
> ```
>
> 登入完成後告訴我。Search Console API 若尚未啟用，請開  
> https://console.cloud.google.com/apis/library/searchconsole.googleapis.com 按 Enable。

確認檔案存在：

```bash
ls -la ~/.config/gcloud/application_default_credentials.json
```

（Windows：`%APPDATA%\gcloud\application_default_credentials.json`）

**不要把檔案內容印進對話或 log。**

### 1c. 設定 quota project（ADC 必做，漏掉會 403）

ADC 是使用者個人帳號，不隸屬任何 GCP 專案，Google 因此不知道這次呼叫要算在誰的配額上。
service account 的 key 自帶專案資訊，ADC 沒有，所以要另外指定一次。

請使用者跑（`YOUR_PROJECT_ID` 換成步驟 1b 啟用 Search Console API 的那個專案 ID，
是小寫那串 ID，不是專案顯示名稱）：

```bash
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
```

這條指令把 `quota_project_id` 寫進 ADC JSON，`gsc-mcp` 讀到後才會帶上去
（`internal/gscclient/client.go` 的 `option.WithQuotaProject`）。

**漏掉這步的症狀**：`list_sites` 回 403 `permission_denied`，訊息含 `requires a quota project`。
這**不是** GSC property 權限問題，別去 Search Console 後台加人。`gsc-mcp` 已針對這個字串
回傳專屬提示，直接照它給的指令跑即可。

### 1d. ADC 故障自我修復表

**這張表是給你（agent）用的。** 每一種症狀都會被誤診成別的問題——誤診的代價是你叫使用者去
改 Search Console 權限、重裝 binary、重裝 gcloud，或設一個無效的環境變數，全都不會有任何效果。
先對症狀，再照「處置」給指令，不要自己推理成因。

| 症狀 | 真正原因 | 常見誤診（別這樣做） | 處置 |
|---|---|---|---|
| 403 `permission_denied`，訊息含 `requires a quota project` | ADC 沒設 quota project | 以為使用者對該 property 沒權限，叫他去 GSC 後台加人 | 回到 1c 跑 `set-quota-project` |
| 突然 `auth_failed`，訊息含 `cannot fetch token` 或 `invalid_grant` | refresh token 失效（改密碼、久未使用、組織政策） | 以為 binary 壞了，叫他重裝或重跑 `install.sh` | 重跑 1b 的 `application-default login` |
| `submit` / `delete` sitemap 被拒或 403 | ADC scope 在登入當下就固定了 | 叫他設 `GSC_ENABLE_WRITE=true`——**對 ADC 完全無效** | 用 `webmasters`（非 readonly）重跑登入，見下 |
| `gcloud projects list`（或其他 gcloud 指令）說憑證過期，但 ADC 剛剛才登入成功 | 這是**另一套** token（見下） | 以為 ADC 登入沒生效，重跑 `application-default login`——修不到這個 | 跑 `gcloud auth login` |
| Windows 上 `gcloud` 回「not recognized」，但確定裝過 | 裝了但不在 PATH | 以為沒裝，重跑 `winget install`（會回報 already installed 然後卡住） | 用完整路徑：`%LOCALAPPDATA%\Google\Cloud SDK\google-cloud-sdk\bin\gcloud.cmd` |

**`gcloud auth login` 和 `gcloud auth application-default login` 是兩套獨立的憑證**，
各自登入、各自過期、互不影響：

| 指令 | 給誰用 | 存在哪 |
|---|---|---|
| `gcloud auth login` | `gcloud` CLI 本身（`projects list`、`services list` 等） | gcloud 自己的設定目錄 |
| `gcloud auth application-default login` | **你寫的程式**，包括 `gsc-mcp` | `application_default_credentials.json` |

實務上的陷阱：ADC 登入成功之後跑 `gcloud projects list` 仍然可能失敗，因為那條指令用的是
第一套。**兩套都要有**——`gsc-mcp` 要第二套才能查資料，而你要用 `gcloud` 幫使用者找 quota
project ID 時需要第一套。看到「剛登入卻說過期」不要繞圈子，直接補跑另一套。

**為什麼 `GSC_ENABLE_WRITE=true` 對 ADC 無效**：OAuth token 的權限範圍（scope）在使用者按下
授權的那一刻就寫死在 token 裡，程式端的 `option.WithScopes` 擴不了一個已經發出去的 token
（見 `internal/config/config.go` 的 `Scopes()` 註解）。這個旗標只對 service account 有效。
`gsc-mcp` 啟動時若偵測到 ADC + `GSC_ENABLE_WRITE=true`，會在 stderr 印警告——
**看到那行警告就直接照它做，不要再往下追查**。

ADC 要開寫入權限，唯一方法是重新登入並換 scope：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform
```

（`delete` 另外還要 `GSC_ALLOW_DESTRUCTIVE=true`，這個旗標對 ADC 和 service account 都有效，
因為它是本專案自己的閘門，不是 OAuth scope。）

**先跑 `gsc-mcp doctor`。** 它會跑完上述所有檢查、**真的呼叫一次 `list_sites`**，然後
**不寫入任何檔案**。ADC 出問題時這是第一個該跑的指令，不要自己手刻 JSON-RPC 去測——
它已經把那件事做完了，而且會針對失敗原因印出對應的修復指令。

```bash
gsc-mcp doctor
```

注意 `gsc-mcp setup --dry-run` **不等於** doctor：`--dry-run` 會跳過 API 呼叫，
所以它答不出「我的憑證到底能不能用」。要驗證憑證就用 `doctor`。

**再看 stderr。** `gsc-mcp` 所有日誌走 stderr（stdout 只有 MCP 協定訊息）。
預設 log level 是 info，這個級別**只**看得到上面那兩行旗標警告。

若要確認「憑證到底是從哪裡讀到的」——這是 ADC 問題最常見的盲點，使用者常有一個忘記的
`GOOGLE_APPLICATION_CREDENTIALS` 環境變數蓋掉了 ADC——必須開 debug：

```bash
GSC_LOG_LEVEL=debug gsc-mcp
```

debug 會多印一行 `credentials loaded from ...`，告訴你命中的是六個來源中的哪一個
（優先序見 `internal/config/config.go` 檔頭）。

---

## 步驟 2：取得 binary

### 首選：install.sh（見文首）

裝完後 binary 在 `~/.local/bin/gsc-mcp`（絕對路徑例如 `/Users/你/.local/bin/gsc-mcp`）。

### 進階：從原始碼建置

無 release、或平台不在五個目標內時：

```bash
go version   # 需要 Go 1.26+
go build -trimpath -ldflags="-s -w" -o ./bin/gsc-mcp ./cmd/gsc-mcp
mkdir -p ~/.local/bin && cp ./bin/gsc-mcp ~/.local/bin/gsc-mcp && chmod +x ~/.local/bin/gsc-mcp
```

然後：

```bash
gsc-mcp setup --dry-run
gsc-mcp setup
```

`setup` 輸出在 **stderr**，不進 MCP stdio。

### macOS Gatekeeper

`install.sh` 會 `xattr -d com.apple.quarantine`。若使用者從瀏覽器手動下載，仍可能被擋：Finder **右鍵 → 打開**。

---

## 步驟 3：寫入 MCP 設定

### 最重要的規則

**必須合併，不能整個覆寫。** 讀出 JSON → 在 `mcpServers` 新增 `gsc` → 寫回。壞 JSON 就停下。寫前備份 `<檔>.bak-<timestamp>`。

### ADC：不要 env 區塊

```json
{
  "mcpServers": {
    "gsc": {
      "command": "/Users/使用者名稱/bin/gsc-mcp"
    }
  }
}
```

憑證走 ADC 預設路徑，**不必**設 `GOOGLE_*`。

### Claude Code

```bash
claude mcp add --transport stdio gsc -- /Users/使用者名稱/bin/gsc-mcp
```

（ADC 路徑不需要 `-e` 憑證變數。）

### Claude Desktop

| 系統 | 路徑 |
|---|---|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

合併上述 `gsc` 片段。寫完請使用者 **Cmd+Q 完全退出** 再重開。

### Cursor

開啟本 repo 時，Cursor 會讀 `.cursor/mcp.json`，不用另建設定。若要在其他專案全域使用，合併同一個 `gsc` 片段到 `~/.cursor/mcp.json`。

### Codex

開啟本 repo 時，Codex 會讀根目錄 `AGENTS.md` 和 `.agents/skills/`。MCP server 請用 Codex 的 MCP 設定加入同一個 `gsc` 片段；不要將憑證放入 repo 的 `.codex/`。

### Hermes

Hermes 沒有 repo-local MCP discovery；將下面片段**合併**到 `~/.hermes/config.yaml` 的 `mcp_servers`，不要覆寫你原有的 servers：

```yaml
mcp_servers:
  gsc:
    command: gsc-mcp
    supports_parallel_tool_calls: false
```

ADC 不需要 `env` 區塊。重啟 Hermes 後，先叫它呼叫 `list_sites`。

### VS Code + Copilot

路徑因人而異，**不要猜**。印出 JSON 片段請使用者自己貼。

---

## 步驟 4：驗證

### 終端機（ADC，無 env）

```bash
{ printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 2; } \
  | ~/bin/gsc-mcp 2>/tmp/gsc-mcp-err.log
```

**`sleep 2` 必要**，否則 stdin EOF 太早會零輸出誤判失敗。

預期：stdout 有 JSON-RPC；`tools/list` 含 6 支 tool。stderr 不應有「no credentials」。

再測 tool（可在 agent 裡）：

> 用 gsc 的 list_sites 列出我的 Search Console 網站

空清單：帳號可能沒有任何 GSC property，或 API 未啟用，或 ADC scope 缺 webmasters（重跑 login）。

---

## 疑難排解

| 症狀 | 處置 |
|---|---|
| `no credentials found; checked: ...` | 未登入 ADC；跑 application-default login |
| `list_sites` 403 / auth | scope 不足或 API 未啟用；重跑帶 `--scopes` 的 login |
| 空 property 清單 | 此 Google 帳號在 GSC 沒有網站；或登錯帳號 |
| `GSC_ENABLE_WRITE` 仍不能 submit | ADC 下該旗標無效；需 webmasters scope 重登入 |
| GUI 看不到 tool | 設定未合併成功或未完全重啟 |
| refresh 突然失敗 | token 失效；重跑 login（見上文） |

---

## 安全守則

- 不要印出 ADC / service account JSON 內容
- 不要覆寫整個 MCP 設定檔；永遠合併並備份
- 不要自動跑 `gcloud auth login` / `application-default login`
- 預設唯讀；不要主動開 write 旗標

---

## 進階：service account（headless / CI）

僅在使用者明確要求機器人身分時使用。

1. Cloud Console 啟用 API → 建 service account → 下載 JSON  
2. **每個** GSC property 把 `client_email` 加成使用者  
3. 放置：

```bash
mkdir -p ~/.config/gsc-mcp && chmod 700 ~/.config/gsc-mcp
cp <key.json> ~/.config/gsc-mcp/service-account.json
chmod 600 ~/.config/gsc-mcp/service-account.json
```

4. MCP 設定**需要** env：

```json
{
  "mcpServers": {
    "gsc": {
      "command": "/絕對路徑/gsc-mcp",
      "env": {
        "GOOGLE_SERVICE_ACCOUNT_FILE": "/絕對路徑/service-account.json"
      }
    }
  }
}
```

Service account 下 `GSC_ENABLE_WRITE=true` 會把 scope 升為 `webmasters`；`delete` 另需 `GSC_ALLOW_DESTRUCTIVE=true`。

---

## 環境變數速查

| 變數 | 說明 |
|---|---|
| （無） | ADC 使用者可不設任何憑證變數 |
| `GOOGLE_APPLICATION_CREDENTIALS` | 覆寫 ADC 檔路徑 |
| `GOOGLE_SERVICE_ACCOUNT_FILE` | service account key |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | inline JSON（CI） |
| `GSC_LOG_LEVEL` | 建議 `warn` |
| `GSC_ENABLE_WRITE` | **僅 service account** 有效升級 scope |
| `GSC_ALLOW_DESTRUCTIVE` | 允許 delete |

完整說明見 [README.md](README.md) 與 [SPEC.md](SPEC.md) §4。
