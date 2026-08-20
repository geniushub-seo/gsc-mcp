<div align="center">

# gsc-mcp

### Google Search Console 的本地 MCP server — 单个 Go 二进制文件，运行端零 runtime 依赖

[![ci](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/geniushub-seo/gsc-mcp)](https://github.com/geniushub-seo/gsc-mcp/releases)

[English](README.md) | [繁體中文](README_ZH_TW.md) | 简体中文

</div>

为 SEO 与内容团队设计：用你自己的 Google 账号登录（ADC），本地 agent（Claude Code / Codex / Cursor / Hermes）就能查询你本来就有权限的 Search Console 数据——**不必**为每个资源（property）单独添加 service account。

## 一行安装

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

安装脚本会下载对应平台的二进制文件、**校验 SHA-256**、安装到 `~/.local/bin`（Windows 是 `%LOCALAPPDATA%\Programs\gsc-mcp`），并解除 macOS quarantine / Windows SmartScreen 拦截标记。若由 AI agent 协助，所有终端命令（包括启动 gcloud 登录）都由 agent 执行；用户只需在自动打开的 Google 页面选择账号并允许访问，不必打开终端或复制命令。详见 [INSTALL.md](INSTALL.md) 与 [Releases](https://github.com/geniushub-seo/gsc-mcp/releases)。

`PROJECT_ID` 换成 `gcloud projects list` 输出里的项目 ID，不要把占位符原样执行。**那两行都不能省**——ADC 是个人账号、不隶属任何项目，少了 quota project 所有查询都会返回 403 `requires a quota project`，看起来很像权限问题但并不是；而全新的项目本来就没有启用 Search Console API，少了 `gcloud services enable` 会返回同一个 403、详情里写 `"reason": "SERVICE_DISABLED"`。另外 `gcloud auth login` 和 ADC 登录是两次独立登录，少了它 `gcloud projects list` 会报凭据过期。

装完有任何一步不顺，运行 `gsc-mcp doctor`——完整检查加一次真实 `list_sites`，不写入任何文件。

## 开始使用（ADC，推荐）

用你自己的 Google 账号登录一次，MCP 配置里**不需要**任何凭据环境变量。

手动安装可以运行上面的命令。若交给 AI agent，只需说“按 [INSTALL.md](INSTALL.md) 帮我装好”；agent 必须负责所有终端操作，用户只负责 Google 浏览器授权。

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

写入有两道独立闸门，ADC 两道都要过：

1. 设置 `GSC_ENABLE_WRITE=true`。这是 gsc-mcp 的**本地闸门**，对 ADC 与 service account 都生效。只重登 write scope 不够。
2. ADC token 还必须在登录当下就带 `webmasters` scope（事后无法扩大）。重新登录并带上：

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform
```

（`webmasters` 是读写权限；只读请用 `webmasters.readonly`。）

## 环境变量

ADC 不需要设置任何凭据变量——按上面装好，二进制文件会自己读取
`~/.config/gcloud/application_default_credentials.json`。下表全部是选填。

| 变量 | 默认值 | 作用 |
|---|---|---|
| `GOOGLE_APPLICATION_CREDENTIALS` | — | 覆盖 ADC 凭据文件路径；默认路径可用时不需要设置 |
| `GSC_ENABLE_WRITE` | `false` | 本地写入闸门，允许 `submit`。ADC 还需要 token 本身带 `webmasters` scope（见上方「写入（submit / delete sitemap）与 ADC」） |
| `GSC_ALLOW_DESTRUCTIVE` | `false` | 允许 `delete`；还需同时 `GSC_ENABLE_WRITE=true` |
| `GSC_REQUEST_TIMEOUT` | `30s` | 单次 API 调用的超时时间 |
| `GSC_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

service account 的凭据变量与完整优先级见下方「进阶：service account（无人值守 / CI）」；普通安装不需要。

### macOS Gatekeeper

`install.sh` 会尝试移除 quarantine 标记。如果你是通过浏览器手动下载的二进制文件，仍然可能看到「无法验证开发者」：在 Finder 里对该文件**右键 → 打开 → 打开**。本项目没有做 Apple 代码签名。

## 进阶：service account（无人值守 / CI）

**普通用户不需要这一节。** 只有下列情况才走这条路：手上已经有 service account JSON
key、跑在 CI 或无人值守机器上、目标机器打不开浏览器、或者需要非人类身份。否则请用上面的
ADC——它不必给每个资源添加用户。

代价先说明：service account 是机器账号，GSC 不会自动认它，**每一个**要查的资源都得手动
把它的 `client_email` 添加为用户。换来的是写入比 ADC 简单：`GSC_ENABLE_WRITE=true`
会直接把 scope 升级为 `webmasters`，不必像 ADC 那样重新跑一遍登录。

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

### service account 专用变量与凭据优先级

| 变量 | 作用 |
|---|---|
| `GOOGLE_SERVICE_ACCOUNT_FILE` | key 文件路径 |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | 内联 JSON，适合把密钥存成字符串的 CI |

完整优先级（前五层都是显式指定，全都没设置才落到 ADC）：`--credentials-file`
（别名 `--service-account-file`）→ `GOOGLE_APPLICATION_CREDENTIALS` →
`GOOGLE_SERVICE_ACCOUNT_FILE` → `GOOGLE_SERVICE_ACCOUNT_JSON` → `.env` →
**ADC 默认路径**。权威定义见 [SPEC.md](SPEC.md) §4.2。

## 各 agent 的原生上手方式

想让 agent 读取项目专属设置与教学时，clone 此 repo。这些设置都不含凭证或本机绝对路径。

| Agent | 原生 repo 文件 | 怎么用 |
|---|---|---|
| Claude Code | `.mcp.json`、`.claude-plugin/`、`.agents/skills/` | 打开 repo 或安装 plugin。 |
| Codex | `AGENTS.md`、`.agents/skills/` | 从 repo 根目录启动 Codex。 |
| Cursor | `.cursor/mcp.json`、`.cursor/rules/gsc-mcp.mdc` | 将 repo 当作项目打开。 |
| Hermes | [`.hermes/`](.hermes/) 上手工具 | 让 Hermes 按 `.hermes/ONESHOT.md` 安装；所有终端操作由 Hermes 执行，成功后开启新的 session。 |

所有 client 的共同验收：先运行 `gsc-mcp doctor`，再让 agent 调用 `list_sites`。不要把 OAuth token 或 service-account key 放进这些文件。

## Skills（开箱即用的分析）

四个 skill 随 server 一起提供，不用自己琢磨 prompt：

| Skill | 用户会怎么问 | 做什么 |
|---|---|---|
| `nonbrand-performance` | 「除了搜我公司名，还有谁能找到我？」 | 用 regex 查询过滤器（`excludingRegex`）区分品牌与非品牌流量，能一次涵盖品牌词的所有拼写变体 |
| `monthly-report` | 「这个月网站表现怎么样？」 | 月度报告，内置品牌／非品牌拆分 |
| `index-health` | 「Google 有没有看到我的新页面？」 | sitemap 状态、逐页诊断、canonical 冲突检测 |
| `gsc-recipes` | 其他任何问题 | 参数路由表：问题 → 精确的 tool 调用 |

Claude Code plugin 与 Codex 都会读取共用的 `.agents/skills/`。Cursor 有同一套安全默认值的 project rule；需要特定分析时，agent 可读取对应的 `SKILL.md`。

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
