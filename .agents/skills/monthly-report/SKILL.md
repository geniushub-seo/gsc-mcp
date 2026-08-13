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

Run `compare_periods` twice more with `dimensions: ["query"]`, once with `excludingRegex` and once with `includingRegex` on the brand pattern. Ask the user for the brand terms if this is the first report of the conversation.

This split is what separates a real report from a vanity one. Branded traffic tracks brand awareness; non-brand tracks search performance.

## Step 4 — Winners and losers

From the `["query"]` comparison, take:

- Top 5 by positive `clicks_delta`
- Top 5 by negative `clicks_delta`
- Queries with `only_in: "b"` — newly appearing
- Queries with `only_in: "a"` — disappeared

Repeat with `dimensions: ["page"]` to get the same view by URL. Pages that lost clicks are usually more actionable than queries, because a page is something you can go and fix.

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
- API rows are top rows only, so category totals will not sum to the site total. Say so if you show both.
- Impressions moving without clicks moving usually means a position or SERP-feature change, not a content problem — check position before recommending a rewrite.
