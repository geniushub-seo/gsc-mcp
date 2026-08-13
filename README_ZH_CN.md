<div align="center">

# gsc-mcp

### Google Search Console 的本地 MCP server — 单个 Go 二进制文件，运行端零 runtime 依赖

[![ci](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/geniushub-seo/gsc-mcp)](https://github.com/geniushub-seo/gsc-mcp/releases)

[English](README.md) | [繁體中文](README_ZH_TW.md) | 简体中文

</div>

为 SEO 与内容团队设计：用你自己的 Google 账号登录（ADC），本地 agent（Claude Code / Codex / Cursor）就能查询你本来就有权限的 Search Console 数据——**不必**为每个资源（property）单独添加 service account。

## 一行安装

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
gsc-mcp setup
```

**Windows（PowerShell）**

```powershell
irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
gsc-mcp setup
```

安装脚本会下载对应平台的二进制文件、**校验 SHA-256**、安装到 `~/.local/bin`（Windows 是 `%LOCALAPPDATA%\Programs\gsc-mcp`），并解除 macOS quarantine / Windows SmartScreen 拦截标记。后面三行请自己运行（会打开浏览器 / 写入 MCP 配置）。详见 [INSTALL.md](INSTALL.md) 与 [Releases](https://github.com/geniushub-seo/gsc-mcp/releases)。

`YOUR_PROJECT_ID` 换成你启用了 Search Console API 的 GCP 项目 ID。**这一行不能省**——ADC 是个人账号、不隶属任何项目，少了它所有查询都会返回 403 `requires a quota project`，看起来很像权限问题但并不是。

装完有任何一步不顺，运行 `gsc-mcp doctor`——完整检查加一次真实 `list_sites`，不写入任何文件。

## 开始使用（ADC，推荐）

用你自己的 Google 账号登录一次，MCP 配置里**不需要**任何凭据环境变量。

最省事就是运行上面那三行。或者把仓库交给 AI agent，跟它说：按 [INSTALL.md](INSTALL.md) 帮我装好。

### 手动操作（二进制文件已安装之后）

1. **安装 gcloud**（一次性）：
   - macOS：`brew install --cask google-cloud-sdk`
   - Linux：`curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-latest-linux-x86_64.tar.gz && tar -xf google-cloud-cli-latest-linux-x86_64.tar.gz && ./google-cloud-sdk/install.sh`（或使用发行版自带的 `google-cloud-sdk` 包）
   - Windows：`winget install Google.CloudSDK`，或下载 [GoogleCloudSDKInstaller.exe](https://dl.google.com/dl/cloudsdk/channels/rapid/GoogleCloudSDKInstaller.exe)
   - **注意**：不要只解压 tarball 而不运行 `install.sh`——这样内置的 Python 不会被安装，启动脚本会退回到系统的 `python3`（macOS 自带 3.9，而 gcloud 需要 3.10–3.14），报出来的错误看起来像是本项目的 bug。那 713 MB 是 Google Cloud SDK 的体积，不是本项目的。
2. **启用 Search Console API**（如果还没启用）：在 [API Library](https://console.cloud.google.com/apis/library/searchconsole.googleapis.com) 点 Enable。先确认左上角选中的是你要用的项目，并记下它的项目 ID（小写那串，不是显示名称）。
3. **ADC 登录**（会打开浏览器）——上面第二行。
4. **设置 quota project**——上面第三行。ADC 必做，漏掉每次查询都会 403。
5. **运行 `gsc-mcp setup`**——合并 MCP 配置并试调用 `list_sites`。
6. 在 agent 里调用 `list_sites`，确认能看到哪些资源。

出问题时运行 `gsc-mcp doctor`：完整检查加一次真实 `list_sites`，**不写入任何文件**，并针对失败原因给出对应命令（缺 quota project、token 失效、API 未启用各有各的处理）。`setup --dry-run` 会跳过 API 调用，答不出凭据能不能用，别拿它当诊断工具。

### MCP 配置长这样（ADC：只要 command）

```json
{
  "mcpServers": {
    "gsc": {
      "command": "/Users/YOUR_USER/.local/bin/gsc-mcp"
    }
  }
}
```

凭据会自动从 `~/.config/gcloud/application_default_credentials.json` 读取。

### ADC refresh token 失效时

它迟早会失效——改密码、长期不用、或者组织策略生效之后。重新登录即可，不是程序坏了：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
```

### 写入（submit / delete sitemap）与 ADC

**`GSC_ENABLE_WRITE=true` 对 ADC 无效。** ADC 令牌的 OAuth scope 在运行 `gcloud auth application-default login` 的那一刻就固定了，事后无法扩大。要写入就必须重新登录，并带上：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform
```

（`webmasters` 是读写权限；只读请用 `webmasters.readonly`。）

## 环境变量

| 变量 | 默认值 | 作用 |
|---|---|---|
| `GOOGLE_APPLICATION_CREDENTIALS` | — | 官方 ADC 路径覆盖（优先级 2） |
| `GOOGLE_SERVICE_ACCOUNT_FILE` | — | service account key 的路径（优先级 3） |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | — | 内联 JSON（优先级 4，适合 CI） |
| `GSC_ENABLE_WRITE` | `false` | **仅 service account 有效**：把 scope 升级为 `webmasters` 并允许 `submit`。在 ADC 下只会告警，不起作用 |
| `GSC_ALLOW_DESTRUCTIVE` | `false` | 允许 `delete`；同时还需要 `GSC_ENABLE_WRITE=true`（service account） |
| `GSC_REQUEST_TIMEOUT` | `30s` | 单次 API 调用的超时时间 |
| `GSC_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

凭据优先级：`--credentials-file`（别名 `--service-account-file`）→ `GOOGLE_APPLICATION_CREDENTIALS` → `GOOGLE_SERVICE_ACCOUNT_FILE` → `GOOGLE_SERVICE_ACCOUNT_JSON` → `.env` → **ADC 默认路径**。

### macOS Gatekeeper

`install.sh` 会尝试移除 quarantine 标记。如果你是通过浏览器手动下载的二进制文件，仍然可能看到「无法验证开发者」：在 Finder 里对该文件**右键 → 打开 → 打开**。本项目没有做 Apple 代码签名。

## 进阶：service account（无人值守 / CI）

适用于非人类身份、CI，或者无法使用个人 Google 登录的场景。

1. 在 GCP 启用 Search Console API → 创建 service account → 下载 JSON key。
2. 在 Search Console 里，把它的 `client_email` 添加为**每一个**资源的用户。
3. 在 MCP 配置里加上环境变量：

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

## Skills（开箱即用的分析）

四个 skill 随 server 一起提供，不用自己琢磨 prompt：

| Skill | 用户会怎么问 | 做什么 |
|---|---|---|
| `nonbrand-performance` | 「除了搜我公司名，还有谁能找到我？」 | 用 regex 查询过滤器（`excludingRegex`）区分品牌与非品牌流量，能一次涵盖品牌词的所有拼写变体 |
| `monthly-report` | 「这个月网站表现怎么样？」 | 月度报告，内置品牌／非品牌拆分 |
| `index-health` | 「Google 有没有看到我的新页面？」 | sitemap 状态、逐页诊断、canonical 冲突检测 |
| `gsc-recipes` | 其他任何问题 | 参数路由表：问题 → 精确的 tool 调用 |

Claude Code 会自动加载 `skills/`。其他客户端可以把各个 `SKILL.md` 的内容当作 prompt 使用。

## 工具清单

| Tool | 底层 API | 写入 | 说明 |
|---|---|---|---|
| `list_sites` | `sites.list` | — | 列出你有权限的所有资源——多客户场景的入口 |
| `get_site` | `sites.get` | — | 查询单个资源的 `permissionLevel` |
| `query_search_analytics` | `searchanalytics.query` | — | 核心查询，参数完整透传 |
| `compare_periods` | `searchanalytics.query` ×2 | — | server 端封装：返回两个时间段的数据加 delta |
| `inspect_url` | `urlInspection.index.inspect` | — | 每次 1–10 个 URL，串行调用 |
| `manage_sitemaps` | `sitemaps.list/get/submit/delete` | 有条件 | 四合一；submit / delete 需要开启开关 |

只做六个是有意为之。总览、按页面拆解、「进阶」分析其实都是**同一个 API 调用的不同默认参数**，每个都做成独立 tool 的代价是每次请求都要把所有 schema 发给 LLM，选错的概率也随之上升。那些场景交给 `gsc-recipes` 这张路由表，不占 context。**只有 LLM 自己做不到的计算才值得封装成 tool**——比如对 25,000 行求中位数基准。

### 明确不做

- `sites.add` / `sites.delete`：add 之后仍然要在网页界面里验证，而 delete 太容易误触发。
- Indexing API：Google 只对 `JobPosting` 和 `BroadcastEvent` 开放。
- 覆盖率报告、Core Web Vitals、链接报告：官方 API 不存在。

## 数据特性（这些都写进了 LLM 会读到的 tool description）

- 日期使用 **PT 时区**（UTC−7/−8），既不是 UTC，也不是你的本地时间。
- 数据有 **2–4 天延迟**，只保留 **16 个月**；靠近当前的日期可能返回不完整的数据。
- API 只返回 top rows，**不保证数据完整**；求和结果不会等于网页后台显示的总计。
- `rowLimit` 上限为 **25,000**（网页后台一次只给 1,000 行，这是用 API 的主要好处）。超出部分要用 `startRow` 分页。
- `HOUR` 维度必须搭配 `dataState: HOURLY_ALL`，而且只有最近 **10 天**的数据。
- 过滤器只能作用于 `QUERY` / `PAGE` / `COUNTRY` / `DEVICE` / `SEARCH_APPEARANCE`，**不能**对 `DATE` 或 `HOUR` 过滤。
- 如果按 `page` 分组或过滤，`aggregationType` 就不能用 `BY_PROPERTY`。
- 平均排名是加权平均值，不等于「这个关键词排在第几位」这个事实。

`site_url` 可以直接给裸域名（`example.com`）、完整 URL 或标准的 `sc-domain:` 格式，server 会做规范化；如果猜错导致 403，它会查询你有权限的资源列表并重试一次。

## 开发

```bash
go build -trimpath -ldflags="-s -w -X main.version=dev" -o ./bin/gsc-mcp ./cmd/gsc-mcp
go test ./... && go vet ./... && golangci-lint run
```

## 相关文档

- [INSTALL.md](INSTALL.md) — 写给 AI agent 照着执行的安装指引
- [SPEC.md](SPEC.md) — 定稿的技术规格

---

<div align="center">

由 **[万智汇 Genius Hub](https://geniushub.cc/)** 开发 · 采用 MIT 许可

</div>
