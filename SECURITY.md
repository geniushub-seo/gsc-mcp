# Security Policy / 資安政策

## Supported versions / 支援版本

Only the [latest release](https://github.com/geniushub-seo/gsc-mcp/releases/latest) receives fixes.
只有[最新 release](https://github.com/geniushub-seo/gsc-mcp/releases/latest) 會收到修補。

## Reporting a vulnerability / 回報漏洞

Please do **not** post vulnerability details, credentials, or real Search Console data in a public issue.
請**不要**在公開 issue 貼漏洞細節、憑證或真實的 Search Console 資料。

1. Open an issue titled `Security: contact request` with no technical details.
   開一個標題為 `Security: contact request` 的 issue，不要寫技術細節。
2. The maintainer will reply there and arrange a private channel.
   維護者會在該 issue 回覆，並安排私下的溝通管道。

## What this server touches / 本 server 會碰到什麼

- It runs locally over stdio and uses the user's own Google credentials (ADC or a service account file). It does not send credentials anywhere except to Google's own endpoints.
  它在本機以 stdio 執行，使用使用者自己的 Google 憑證（ADC 或 service account 檔），憑證只會送到 Google 官方端點。
- Write actions (`manage_sitemaps` submit / delete) are disabled by default and need `GSC_ENABLE_WRITE=true`; delete additionally needs `GSC_ALLOW_DESTRUCTIVE=true`.
  寫入動作（`manage_sitemaps` 的 submit / delete）預設關閉，需設 `GSC_ENABLE_WRITE=true`；delete 另需 `GSC_ALLOW_DESTRUCTIVE=true`。
- stdout carries MCP JSON-RPC only; logs go to stderr.
  stdout 只有 MCP JSON-RPC，日誌走 stderr。
