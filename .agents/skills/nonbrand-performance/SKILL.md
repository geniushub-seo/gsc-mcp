---
name: nonbrand-performance
description: Measure organic growth with branded queries excluded — the number that actually proves SEO is working. Use when asked about non-brand traffic, real organic growth, whether SEO is paying off, or how people find the site without already knowing the company name.
---

# Non-Brand Performance

Branded queries inflate every SEO report. People searching your company name were already coming — that traffic grows with your marketing, not your SEO. Strip it out and you see what search actually earned you.

**Most GSC tools cannot do this.** It requires regex filtering, which `gsc-mcp` supports through `excludingRegex`.

## Step 1 — Confirm the property

Call `list_sites`. Match the user's domain to a `site_url`. If several look plausible, ask which one.

## Step 2 — Establish the brand pattern

You need a regex covering every way people type the brand. Ask the user once, then reuse it for the rest of the conversation:

> What terms should count as branded? Include misspellings and any local-language spellings.

Build a RE2 alternation from their answer, case-insensitive by default in GSC:

```
acme|acmecorp|acme corp|akme|艾克米
```

If the user cannot answer, derive a first pass from the domain (`acme.com` → `acme`) and **say explicitly that you did so** — an incomplete brand pattern silently inflates the non-brand numbers, which is the exact error this analysis exists to avoid.

## Step 3 — Pull both halves

Same window for all calls. Default to the last 28 days ending 3 days ago (GSC data lags 2–4 days).

KPI totals (clicks, share, change) come from **aggregate** calls: **omit dimensions**. Do not sum a `dimensions: ["query"]` top-N for these numbers — `row_limit` would silently drop traffic.

**Non-brand totals:**

```
query_search_analytics
  site_url: <resolved>
  start_date / end_date: <window>
  dimensions: []
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"excludingRegex","expression":"<brand pattern>"}]
  }]
```

**Branded totals:**

```
query_search_analytics
  site_url: <resolved>
  start_date / end_date: <window>
  dimensions: []
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"includingRegex","expression":"<brand pattern>"}]
  }]
```

**Site totals** — same shape with no `dimension_filter_groups`.

Query-level detail is a separate call. Keep the non-brand filter. Never use that list to compute share:

```
query_search_analytics
  site_url: <resolved>
  start_date / end_date: <window>
  dimensions: ["query"]
  row_limit: 1000
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"excludingRegex","expression":"<brand pattern>"}]
  }]
```

## Step 4 — Compare against the previous period

KPI period change needs **two** aggregate `compare_periods` calls, both with **dimensions omitted**. One filter direction is not both halves of the table.

**Non-brand change (KPI):**

```
compare_periods
  ...same dates...
  dimensions: []
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"excludingRegex","expression":"<brand pattern>"}]
  }]
```

**Branded change (KPI):**

```
compare_periods
  ...same dates...
  dimensions: []
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"includingRegex","expression":"<brand pattern>"}]
  }]
```

Then, for query detail only, keep `excludingRegex` and call twice with `dimensions: ["query"]`, `sort_by: clicks_delta` / `sort_order: desc` for gains and `sort_order: asc` for losses — one default desc call is not both lists:

```
compare_periods
  ...same dates...
  dimensions: ["query"]
  sort_by: clicks_delta
  sort_order: desc
  row_limit: 10
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"excludingRegex","expression":"<brand pattern>"}]
  }]
```

After every response, read `truncated`, `scan_capped`, and `ordering`. `truncated=true` means more rows exist; `scan_capped=true` means the 25,000-row scan was incomplete.

## Step 5 — Report

Lead with the split, because that is the answer to "is SEO working":

| | Clicks | Share | vs. previous 28 days |
|---|---|---|---|
| Non-brand | 1,240 | 62% | +18% |
| Branded | 760 | 38% | +2% |

Then:

- **Top 10 non-brand queries by clicks**, with position and CTR
- **Biggest non-brand gains** — queries with the largest positive `clicks_delta`
- **New non-brand queries** — `only_in: "b"` rows, meaning search found the site for terms it never did before
- **Lost queries** — `only_in: "a"` rows, worth checking whether the page still exists

Close with one plain sentence a business owner understands, for example: *"Non-brand clicks grew 18% while branded stayed flat — new visitors are finding you through search rather than already knowing your name."*

## Accuracy rules

- **Never present branded and non-brand as if the split were exact.** It is only as good as the regex. Say which pattern you used.
- Average position is a weighted average across impressions, not a ranking for any single query.
- Always read `truncated`, `scan_capped`, and `ordering`. Do not treat a top-N as complete when either completeness flag is true.
- Never derive brand/non-brand **share or change** from a capped grouped query list. Use the aggregate (no-dimension) calls from Step 3. Anonymized `(other)` queries can still make the two halves miss the site total — say so.
- Rows returned are top rows only; totals will not match the Search Console UI exactly.
- Use `data_state: all` (the default) so figures line up with what the user sees in the Search Console dashboard.
