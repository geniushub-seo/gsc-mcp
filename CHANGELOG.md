# Changelog / 更新紀錄

Older versions: see [Releases](https://github.com/geniushub-seo/gsc-mcp/releases).
更早的版本請看 [Releases](https://github.com/geniushub-seo/gsc-mcp/releases)。

## v0.6.2 — 2026-11-15

### Changed / 變更

- A wrong or unauthorized `site_url` no longer switches to another property. The server queries only the normalized property; on 403 it returns `permission_denied` and lists every accessible property with its permission level.
  `site_url` 錯誤或無權限時，不再自動改打別的 property。只打正規化後的那一個；403 時回 `permission_denied`，並列出所有可存取的 property 與權限等級。
- With `data_state=final`, the three computed tools (`find_ctr_gaps`, `find_page_two_queries`, `find_declining_pages`) skip the extra date probe, because Google only reports the first incomplete date under `data_state=all`.
  `data_state=final` 時，三支計算型 tool 不再多發一次日期探針，因為 Google 只在 `data_state=all` 回報第一個不完整日期。

### Removed / 移除

- Hermes setup script and its documentation.
  Hermes 上手腳本與相關文件。

## v0.6.1 — 2026-09-04

### Fixed / 修正

- `monthly-report` and `traffic-decline-diagnosis` skill descriptions both used "update" with opposite meanings (client report update vs. Google algorithm update). Both are now disambiguated.
  `monthly-report` 與 `traffic-decline-diagnosis` 兩支 skill 的 description 都用到「update」但意思相反（客戶更新報告 vs. Google 演算法更新），已消歧。

## v0.6.0 — 2026-09-03

### Added / 新增

- Three computed tools: `find_ctr_gaps`, `find_page_two_queries`, `find_declining_pages` (nine tools in total).
  三支計算型 tool：`find_ctr_gaps`、`find_page_two_queries`、`find_declining_pages`（共九支）。
- Six skills under `.agents/skills/`; `install.sh` and `install.ps1` now unpack them into `~/.claude/skills`. An existing skill directory is never overwritten.
  六支 skill 放在 `.agents/skills/`；`install.sh`、`install.ps1` 現在會把它們解到 `~/.claude/skills`，已存在的 skill 目錄不會被覆寫。
