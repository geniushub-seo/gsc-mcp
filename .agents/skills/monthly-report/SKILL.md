---
name: monthly-report
description: Produce a client-ready monthly search performance report with brand and non-brand split, winners, losers, and recommended actions. Use when asked for a monthly report, a site summary, how the site performed, or a client update.
---

# Monthly Search Report

A report someone can forward to a client without editing. Numbers, what changed, and what to do about it.

## Step 1 — Property and window

Call `list_sites` to resolve the exact `site_url`.

Default window: the last 28 days ending 3 days ago, compared against the 28 days immediately before. Twenty-eight days rather than a calendar month keeps weekday counts equal — a calendar month comparison mixes a 4-weekend month with a 5-weekend one and produces fake swings.

State the exact dates in the report.

## Step 2 — Overall movement

```
compare_periods
  site_url: <resolved>
  period_a_start / period_a_end: <previous 28 days>
  period_b_start / period_b_end: <recent 28 days>
  dimensions: []          # totals only
```

Gives clicks, impressions, CTR and position for both periods plus deltas. Note the field semantics:

- `ctr_delta_pp` is in **percentage points** (2.5 means 2.5pp, not 0.025)
- `position_improved` is a boolean — a *smaller* position number is better, and this field has already worked that out
- `only_in` marks rows present in one period only; those rows omit position deltas because a comparison would be meaningless

## Step 3 — Brand vs non-brand split

KPI totals (clicks, share, change) come from **aggregate** calls: **omit dimensions**. Do not sum a `dimensions: ["query"]` top-N for these numbers — `row_limit` would silently drop traffic and `truncated` cannot recover the missing total.

Ask the user for the brand terms if this is the first report of the conversation.

**Non-brand totals:**

```
compare_periods
  ...same dates...
  dimensions: []
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"excludingRegex","expression":"<brand pattern>"}]
  }]
```

**Branded totals:**

```
compare_periods
  ...same dates...
  dimensions: []
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"includingRegex","expression":"<brand pattern>"}]
  }]
```

This split is what separates a real report from a vanity one. Branded traffic tracks brand awareness; non-brand tracks search performance. Query-level detail stays in Step 4.

## Step 4 — Winners and losers

Do **not** take winners and losers from one `clicks_delta desc` call. That ranking only returns the biggest gains; the biggest drops sit at the other end of the list and are cut by `row_limit`.

Call `compare_periods` twice per dimension:

**Winners (query):**

```
compare_periods
  ...same dates...
  dimensions: ["query"]
  sort_by: clicks_delta
  sort_order: desc
  row_limit: 5
```

**Losers (query):**

```
compare_periods
  ...same dates...
  dimensions: ["query"]
  sort_by: clicks_delta
  sort_order: asc
  row_limit: 5
```

Repeat both calls with `dimensions: ["page"]`. Pages that lost clicks are usually more actionable than queries, because a page is something you can go and fix.

After every call, read `truncated`, `scan_capped`, and `ordering`:

- `truncated=true` means more rows exist beyond `row_limit`.
- `scan_capped=true` means even the 25,000-row scan was incomplete, so the top-N may be missing candidates.
- Confirm `ordering` matches what you asked for before writing the table.

Also collect:

- Queries/pages with `only_in: "b"` — newly appearing
- Queries/pages with `only_in: "a"` — disappeared

## Step 5 — Write the report

```markdown
# Search Performance — <site>
<period B dates> vs <period A dates>

## Summary
One paragraph in plain language. Lead with non-brand, since that is the SEO result.

## Numbers
| Metric | Previous | Current | Change |
| Clicks / Impressions / CTR / Avg position |

## Brand vs non-brand
Table with clicks, share, and change for each.

## What grew
Up to 5 rows, each with a one-line reason if the data suggests one
(position improved, new query appeared, CTR rose at the same position).

## What declined
Up to 5 rows. For each, say whether position fell, impressions fell,
or CTR fell — the three have different causes and different fixes.

## Recommended actions
Three at most, ordered by expected impact. Each names a specific page or query.
```

## Accuracy rules

- Give exact date ranges. "Last month" is ambiguous and clients will compare against their own dashboard.
- Round rates sensibly: CTR to one decimal as a percentage, position to one decimal.
- Do not attribute causes the data cannot support. A ranking drop is visible; *why* it dropped is not in Search Console.
- Always read `truncated`, `scan_capped`, and `ordering` on every `compare_periods` / `query_search_analytics` response. Do not treat a top-N as complete when either flag is true.
- Never derive brand/non-brand **share or change** from a capped grouped query list. Use the aggregate (no-dimension) calls from Step 3. Anonymized `(other)` queries can still make the two halves miss the site total — say so.
- API rows are top rows only, so category totals will not sum to the site total. Say so if you show both.
- Impressions moving without clicks moving usually means a position or SERP-feature change, not a content problem — check position before recommending a rewrite.
