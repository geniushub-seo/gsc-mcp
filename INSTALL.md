# INSTALL — 給 AI agent 的安裝指引

**這份文件是寫給 AI agent 讀的。** 使用者把整個 repo 交給你，要你幫他把這支 MCP server 裝好。照下面做。

---

## 首選：一行安裝

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
```

先 dry-run 看會做什麼：

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash -s -- --dry-run
```

`install.sh` 只負責：判平台、下載 release、驗 SHA-256、裝到 `~/.local/bin`、macOS 移除 quarantine、PATH 提示。  
**不會**跑 `gcloud login`、不會改 MCP 設定、不會改 shell rc。

接著請使用者自己跑（印指令，不代跑）：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gsc-mcp setup
```

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

1. 要用哪個 AI 工具？（Claude Desktop / Claude Code / Cursor / VS Code）
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

### ADC refresh token 失效時

使用者改密碼、久未使用、或組織政策可能讓 token 失效。症狀像「突然 auth 失敗」。處置是重跑上面的 `application-default login`，不是重裝 binary。

### 寫入 sitemap 時

ADC 的 scope **在登入當下固定**。`GSC_ENABLE_WRITE=true` **對 ADC 無效**。若使用者要 submit/delete，請他們用 `webmasters`（非 readonly）重跑登入：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform
```

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

`~/.cursor/mcp.json`，格式同 Desktop。

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
