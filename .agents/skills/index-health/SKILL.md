---
name: index-health
description: Check whether Google has actually indexed a site's pages, and diagnose the ones it has not. Use when asked whether pages are indexed, why a page is not showing in search, about crawling problems, canonical issues, or sitemap errors.
---

# Index Health Check

A page that is not indexed earns nothing, no matter how good it is. This finds the ones Google is skipping and says why.

## Step 1 — Property and sitemaps

Call `list_sites` to resolve the `site_url`, then:

```
manage_sitemaps
  site_url: <resolved>
  action: "list"
```

Report per sitemap: `path`, `last_downloaded`, `is_pending`, `warnings`, `errors`. A sitemap Google has not downloaded in weeks, or one carrying errors, explains a lot of missing pages before you inspect a single URL.

## Step 2 — Pick which URLs to inspect

`inspect_url` accepts 1–10 URLs per call and is quota-limited per property per day. It is not a crawler — never point it at a whole site.

Choose deliberately:

- **User supplied a list** — use it, up to 10 at a time.
- **User asked about the whole site** — do not guess. Pull the pages that matter with `query_search_analytics`, `dimensions: ["page"]`, `sort_by: impressions`, `row_limit: 25`, then inspect the top 10. Read `truncated`, `scan_capped`, and `ordering` before treating that list as complete. Explain that you sampled and why.
- **User asked about new pages** — ask for the URLs. Pages with no impressions yet cannot be found through search analytics.

## Step 3 — Inspect

```
inspect_url
  site_url: <resolved>
  urls: [<up to 10>]
```

The response carries `quota_used_today` and, near the limit, `quota_warning`. Surface the warning to the user rather than silently continuing.

## Step 4 — Read the verdicts

For each URL the important fields are:

| Field | What it tells you |
|---|---|
| `verdict` | `PASS` / `NEUTRAL` / `FAIL` — the headline |
| `coverage_state` | The specific reason, e.g. "Indexed", "Crawled - currently not indexed", "Discovered - currently not indexed" |
| `indexing_state` | Whether indexing is blocked, e.g. by a `noindex` |
| `robots_txt_state` | Whether robots.txt allowed the crawl |
| `page_fetch_state` | Whether Google could fetch the page at all |
| `google_canonical` vs `user_canonical` | **When these differ, Google ignored your canonical** — a common and invisible cause of missing pages |
| `last_crawl_time` | Stale means Google has not looked recently |
| `sitemap` | Which sitemaps reference this URL; empty means it is not in any |

The optional blocks (`mobile_usability_verdict`, `rich_results_verdict`, `amp_verdict`) are omitted by Google when they do not apply. Their absence is not a failure.

## Step 5 — Report as an action list

Group by what the user has to do, not by URL:

```markdown
## Not indexed — needs action
- <url> — Crawled but not indexed. Google fetched it and chose not to index.
  Usually thin or duplicate content. Compare against <the canonical Google chose>.

## Canonical mismatch
- <url> — You declared <user_canonical>, Google chose <google_canonical>.
  Your declaration is being ignored; check for duplicate content or conflicting signals.

## Blocked
- <url> — robots.txt disallows it / page carries noindex.

## Not in any sitemap
- <url> — Add it so Google discovers it faster.

## Indexed and healthy
- <count> URLs, no action needed. (List them only if asked.)
```

## Accuracy rules

- Always read `truncated`, `scan_capped`, and `ordering` on any `query_search_analytics` sample. `truncated=true` means more pages exist; `scan_capped=true` means even the scan was incomplete.
- **This reports what Google has stored, not the live page.** Fix a page today and this still shows the old verdict until Google recrawls. Say so whenever the data looks stale.
- It is not a live test, does not request indexing, and does not enumerate every indexed URL. It cannot replace the Page Indexing report in the Search Console UI.
- Quotas are per property per day; see https://developers.google.com/webmaster-tools/limits. Do not state a specific number — Google changes them.
- "Discovered - currently not indexed" and "Crawled - currently not indexed" mean different things. The first means Google knows the URL but has not fetched it (often a crawl-budget or internal-linking issue); the second means it fetched and declined (usually a content issue). Do not give the same advice for both.
