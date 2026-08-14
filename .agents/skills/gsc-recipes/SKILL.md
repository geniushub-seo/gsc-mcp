---
name: gsc-recipes
description: Parameter recipes for Google Search Console questions — maps a plain question to the exact tool call. Consult this whenever a GSC question does not match a more specific skill, or when unsure which dimensions and filters to use.
---

# GSC Query Recipes

Six tools cover everything Search Console exposes. The difficulty is never *which tool* — for search data it is always `query_search_analytics` — it is **which parameters**. This is that lookup.

Other GSC servers ship a separate tool per preset (`get_performance_overview`, `get_search_by_page_query`, and so on). Same API call, different defaults. A table costs nothing and stays correct.

## Defaults that apply everywhere

- **Window**: last 28 days ending 3 days ago. Search Console data lags 2–4 days; ending today returns a partial tail that looks like a decline.
- **`data_state`**: leave unset. The default `all` matches what the user sees in the Search Console UI. Setting `final` will make your numbers disagree with their dashboard.
- **`site_url`**: pass whatever the user said — bare domain, full URL, or `sc-domain:`. It is normalised, and a 403 triggers automatic property discovery. Call `list_sites` first only if the user has several properties and you cannot tell which they mean.
- **`row_limit`**: 1000 is plenty for analysis. The 25,000 ceiling is for exports.
- **Completeness**: every analytics response includes `truncated`, `scan_capped`, and `ordering`. Read them. `truncated=true` means more rows exist beyond `row_limit`. `scan_capped=true` means the 25,000-row scan was incomplete, so a top-N by a non-clicks key can be missing candidates.

## Recipes

### "How is my site doing?" — totals only

```
query_search_analytics
  dimensions: []          ← omit entirely
```
One row: clicks, impressions, CTR, average position for the whole property.

### "What are people searching to find me?"

```
query_search_analytics
  dimensions: ["query"]
  row_limit: 25
```

### "Which pages get the most traffic?"

```
query_search_analytics
  dimensions: ["page"]
  row_limit: 25
```

### "What queries bring people to *this* page?"

```
query_search_analytics
  dimensions: ["query"]
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"page","operator":"equals","expression":"<full URL>"}]
  }]
```
Use `contains` instead of `equals` for a section (`/blog/`) rather than one page.

### "Show me the daily trend"

```
query_search_analytics
  dimensions: ["date"]
```
Add `["date","query"]` to see a single query's trend, filtered to that query.

### "Which countries / devices?"

```
query_search_analytics
  dimensions: ["country"]      ← ISO-3166-1 alpha-3, e.g. TWN, HKG, USA
  dimensions: ["device"]       ← DESKTOP / MOBILE / TABLET
```

### "Non-brand only" — the one other servers cannot do

```
query_search_analytics
  dimensions: ["query"]
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"excludingRegex","expression":"brand|brandname|品牌"}]
  }]
```
`includingRegex` for the branded half. RE2 syntax, case-insensitive. See the `nonbrand-performance` skill for the full analysis.

### "Compare to last month"

```
compare_periods
  period_a_*: the earlier window
  period_b_*: the recent window
  dimensions: ["query"]     or ["page"]
```
Returns both periods plus deltas. Keep both windows the same length — comparing 28 days against a calendar month mixes 4-weekend and 5-weekend periods and invents swings. Pass `dimension_filter_groups` (same shape as `query_search_analytics`) to compare a filtered slice such as non-brand. For biggest losses use `sort_by: clicks_delta` and `sort_order: asc`; a single default desc call is not both winners and losers.

### "Which images / videos / news?"

```
query_search_analytics
  search_type: "image" | "video" | "news" | "discover" | "googleNews"
```
Omit for web. Note `video` means Google Video *search* performance, not the Video Indexing report — it is not evidence that any video is indexed.

### "Hour by hour"

```
query_search_analytics
  dimensions: ["hour"]
  data_state: "hourly_all"    ← required, the call fails without it
```
Only the last 10 days are available.

### "Is this page indexed?"

```
inspect_url
  urls: [<1–10 URLs>]
```
Quota-limited per property per day. Never point it at a whole site.

### "What sitemaps do I have?"

```
manage_sitemaps
  action: "list"
```
`get` for one sitemap's detail. `submit` and `delete` need `GSC_ENABLE_WRITE`; `delete` needs `GSC_ALLOW_DESTRUCTIVE` as well.

### "What properties can I see?"

```
list_sites
```
No parameters. Returns `site_url` and `permission_level`. A `siteUnverifiedUser` property will usually return empty search data — that is a permission level, not a bug.

## Combining dimensions

Dimensions multiply into row keys, in the order given.

| Dimensions | One row per | Use for |
|---|---|---|
| `["query"]` | query | keyword performance |
| `["page"]` | URL | page performance |
| `["query","page"]` | query+URL pair | which page answers which query; also reveals cannibalisation when one query maps to several pages |
| `["date"]` | day | trend |
| `["query","device"]` | query per device | mobile vs desktop gaps |

More dimensions means more rows and thinner data per row. Two is usually the practical limit for reading.

## Constraints the server enforces

These are rejected before the request leaves, with an explanatory message — you do not need to pre-check them, but knowing them explains the errors:

- `row_limit` must be 1–25,000
- `hour` requires `data_state: "hourly_all"`
- filters cannot use `date` or `hour` — filter dimensions are `query`, `page`, `country`, `device`, `searchAppearance` only
- `aggregation_type: "byProperty"` is rejected when grouping or filtering by `page`
- `start_date` must not be after `end_date`

## Reading the numbers honestly

- **Average position is a weighted average** across all impressions for that row. It is not "this keyword ranks 4th". A query appearing at position 2 in one country and 20 in another averages to 11, a position it never actually held.
- **Always read `truncated`, `scan_capped`, and `ordering`.** Do not treat a top-N as complete when either flag is true.
- **Rows are top rows only.** Category totals will not sum to the site total. If you show both, say so.
- **Impressions moving while clicks hold still** usually means a position or SERP-layout change, not a content problem. Check position before recommending a rewrite.
- **CTR is `clicks / impressions`** for that row — comparing CTR across very different positions is meaningless without a position-matched baseline.
- Every response carries `queried_at`. In a long conversation, check it before treating an earlier result as current.
