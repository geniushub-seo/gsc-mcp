---
name: ctr-gaps
description: Find pages that rank well but are clicked less than their position peers, and separate the ones worth rewriting from the ones where the click is never coming. Use when asked which titles to rewrite, why a page ranks but gets no traffic, about low CTR, about quick wins from existing rankings, or about keyword cannibalization.
---

# CTR Gaps

A page at position 5 that nobody clicks is losing traffic it has already earned the
right to. Fixing the title is cheaper than fixing the ranking. This finds those pages
and — the part most tools skip — tells you which ones are actually fixable.

**The arithmetic runs server-side.** `find_ctr_gaps` scans up to 25,000 query+page
rows, computes a position-matched CTR baseline, and returns a ranked shortlist. Do
not try to reproduce this by pulling rows with `query_search_analytics` and comparing
them yourself: a mid-size property returns well over 100KB of rows for a single
30-day window, which does not fit in context. The tool exists because the analysis
cannot be done here.

## Step 1 — Confirm the property

Call `list_sites`. Match the user's domain to a `site_url`. If several look
plausible, ask which one.

## Step 2 — Pull the shortlist

Default to the last 28 days. The tool reads Google's own
`metadata.firstIncompleteDate` and reports it in `assumptions.first_incomplete_date`,
so you do not need to guess a lag.

```
find_ctr_gaps
  site_url: <resolved>
  start_date / end_date: <window>
```

For non-brand only — branded queries have inflated CTR and will crowd the list:

```
find_ctr_gaps
  site_url: <resolved>
  start_date / end_date: <window>
  dimension_filter_groups: [{
    "groupType": "and",
    "filters": [{"dimension":"query","operator":"excludingRegex","expression":"<brand pattern>"}]
  }]
```

Thresholds you may need to move, with their defaults:

| Parameter | Default | Move it when |
|---|---|---|
| `position_min` / `position_max` | 1 / 20 | Only want page-one rankings → `position_max: 10` |
| `min_impressions` | 100 | Low-traffic property returns nothing → lower to 30–50 |
| `min_samples` | 10 | Small property, baselines keep coming back `site_wide` → lower to 5 |
| `row_limit` | 50 | Rarely. 50 rows is already more than anyone will action |

## Step 3 — Read the completeness flags before reading the rows

| Field | What it means if you ignore it |
|---|---|
| `rows_matched` vs `row_count` | `truncated: true` means more rows qualify than were returned |
| `scan_capped` | `true` means the 25,000-row scan was incomplete — **both** the ranking and the baseline pool may be missing rows |
| `baseline_pool_rows` | The size of the pool every baseline was drawn from. Under ~50, treat all baselines as soft |
| `assumptions` | The thresholds actually applied. Quote these when you report numbers |

## Step 4 — The judgement that makes this skill worth anything

`opportunity_clicks = impressions × (baseline_ctr − ctr)`. It is arithmetic, and it
is correct. It is also **not** a promise, and the tool deliberately does not tell you
what to do about it. Three checks before you recommend a rewrite.

### Check 1 — Is the baseline itself healthy?

Look at the absolute value of `baseline_ctr`, not just the gap.

| `baseline_ctr` at positions 1–10 | Reading |
|---|---|
| 5%–15% | Normal. A gap here is a real title problem, and rewriting is the right call |
| 2%–5% | Soft. Competitive SERP or many ads above the fold. Rewriting helps less than the number suggests |
| **under 2%** | **The click is not there to win.** Featured snippets or AI Overviews are answering the query in the SERP. Say so instead of recommending a rewrite |

This check exists because `opportunity_clicks` scales with impressions. A
high-volume query with a 0.9% site baseline will top the list and look like the
biggest win available, when in fact the SERP is satisfying the searcher without a
click. Recommending a title rewrite there wastes the client's time and costs you
credibility when nothing moves.

**How to tell the two apart:** informational queries where the answer is a single
fact — a size, a date, a conversion, a definition — get answered in the SERP.
Queries with commercial or navigational intent still need the click. If the low-CTR
rows are all "what size is X" and the healthy-CTR rows are all "X company", the
pattern is the SERP, not your titles.

### Check 2 — Is the baseline position-matched or site-wide?

`baseline_scope: site_wide` means the position window had to widen across every
observed position to reach `min_samples`. The comparison is then property-wide, not
position-matched — a position-3 row is being compared against position-15 rows.
Report those gaps as directional only, and say why.

`baseline_window_positions` tells you how far the window widened. `0` means true
peers at the same rounded position; `3` or more means the neighbourhood is loose.

### Check 3 — Cannibalization

`cannibalization: true` and `pages_for_query: N` appear only when one query draws
impressions for more than one of your pages. **This is a different problem with a
different fix**, and a title rewrite will not touch it.

| `pages_for_query` | Reading | Action |
|---|---|---|
| 2, both with clicks | Mild overlap, often fine | Watch it, do not act |
| 2, one page taking every click | The other page is dead weight for this query | Retarget the weak page at its own query, or consolidate |
| 3–5 | Real dilution — no single page accumulates authority | Pick the canonical page, merge or internal-link the rest into it |
| 6+ | Structural. The site has no clear owner for this topic | Topic-level restructure, not a page fix |

A branded query legitimately returning your home page, contact page, and about page
is **not** cannibalization — that is normal sitelink behaviour. Exclude brand terms
in Step 2 and this stops appearing.

## Step 5 — Report

Lead with the split, because "which of these are actually fixable" is the real
question:

**Worth rewriting** — healthy baseline, position-matched, no cannibalization:

| Query | Page | Position | CTR | Peer CTR | Opportunity |
|---|---|---|---|---|---|
| example query | /example-page/ | 4.2 | 1.8% | 6.1% | +42 clicks |

**Cannibalization, not a title problem** — name the competing pages and how many:

| Query | Pages competing | Impressions | Clicks | Suggested canonical |
|---|---|---|---|---|

**Low ceiling** — the SERP is answering the query. State the baseline CTR and say
plainly that a rewrite is unlikely to move it.

Close with the single highest-confidence action, not the whole list. A shortlist of
three that someone actually does beats fifty that sit in a doc.

## Accuracy rules

- **Never present `opportunity_clicks` as a forecast.** It is "what this row would
  have earned at peer CTR over this window", past tense. Say it that way.
- Quote `baseline_ctr`, `baseline_samples`, and `baseline_scope` whenever you cite a
  gap. A gap without its baseline is unauditable.
- Average position is an impression-weighted average, not proof the page ranked
  there for any single search.
- `scan_capped: true` invalidates completeness claims. Do not say "the biggest
  opportunities" — say "the biggest in the rows scanned".
- The tool returns facts and arithmetic. Every recommendation in your report is
  yours, so it has to survive the question "why do you think that will work".
- Use `data_state: all` (the default) so figures line up with the Search Console UI.
