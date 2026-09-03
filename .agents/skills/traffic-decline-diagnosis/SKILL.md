---
name: traffic-decline-diagnosis
description: Diagnose why a site or page lost search traffic by separating ranking loss from demand loss from CTR loss. Use when asked why traffic dropped, why clicks are down, what happened after an update, whether a decline is the site's fault, or which pages are declining.
---

# Traffic Decline Diagnosis

"Traffic is down" is not a diagnosis. Three completely different things produce the
same drop, and they need opposite responses:

| Cause | What actually happened | What to do |
|---|---|---|
| `position_drop` | You lost ranking | Look at who overtook you |
| `impressions_drop` | Fewer people searched, or the SERP changed | **Often not your fault.** Do not rewrite the page |
| `ctr_drop` | Same ranking, same searches, fewer clicks | This one points at the title |

Getting this wrong is expensive in both directions: rewriting titles when search
demand fell seasonally, or ignoring a competitor who took your position because
"traffic is just down".

**`find_declining_pages` does the attribution.** It is the only reason to use this
tool over a plain `compare_periods` call.

## Step 1 — Confirm the property

Call `list_sites`. Match the user's domain to a `site_url`.

## Step 2 — Choose the two periods

Period A is the baseline (earlier), period B is current (later). Every delta is
**B minus A**. The periods must be the same length, and A must end before B starts —
reversed or overlapping ranges return `invalid_input`.

Two ways to pick them, and they answer different questions:

**"What changed recently?"** — consecutive equal windows:
```
period_a: 2026-06-15 → 2026-07-12   (28 days)
period_b: 2026-07-13 → 2026-08-09   (28 days)
```

**"How does this compare to last year?"** — same window, previous year. Use this
when the business is seasonal, because a consecutive comparison will attribute
seasonality to your work.

Ask the user which question they are asking if it is not obvious. A drop measured
against the wrong baseline is worse than no measurement.

```
find_declining_pages
  site_url: <resolved>
  period_a_start / period_a_end: <baseline>
  period_b_start / period_b_end: <current>
```

## Step 3 — The incomplete-data trap this tool closes for you

Counting the last few days of the current period against a complete baseline is the
classic way to invent a decline that does not exist. Google's data is not final for
the most recent days.

This tool reads `metadata.firstIncompleteDate` and **trims the end of both periods
by the same number of days**, keeping them equal length. Read these fields and
report them:

| Field | Meaning |
|---|---|
| `assumptions.excluded_incomplete_days` | How many days were trimmed off each period |
| `assumptions` originally-requested ends | Present when trimming happened — the dates you asked for vs. the dates used |

If a user reports a drop that this tool does not find, incomplete-day trimming is
the first thing to check. Their number probably included unfinalized days.

## Step 4 — Read the attribution, then trust it selectively

| `attribution` | Confidence | What to do next |
|---|---|---|
| `position_drop` | High | Check the SERP for this query manually. Who is above you now? |
| `impressions_drop` | High | Check whether this is seasonal or a SERP layout change. Compare year-over-year before concluding anything |
| `ctr_drop` | High | Title and meta description. Also check whether an AI Overview or featured snippet appeared |
| `unclear` | **None — by design** | The page is missing a position on one side: it appeared or vanished. This means "we do not know why", not "no cause" |

`unclear` is the honest answer, not a gap in the tool. A page with no position in
period A is new; a page with none in period B may be deindexed, redirected, or
deleted. Those need `inspect_url` or a look at the site, not a guess.

**When several pages share one attribution, treat it as one event, not N problems.**
Twelve pages all showing `position_drop` in the same window is a site-level event —
an algorithm update, a technical regression, a migration — not twelve page problems.
Say that. Fixing twelve pages one at a time is the wrong response to one cause.

## Step 5 — Thresholds

| Parameter | Default | Move it when |
|---|---|---|
| `min_click_drop` | 10 (inclusive) | Small site → lower to 3–5, or nothing reports. Large site → raise to 50 to cut noise |
| `position_drop_threshold` | 1 | Rarely. Under 1 position of movement is noise |
| `impressions_drop_pct_threshold` | -10 | Volatile niche → -20 so normal fluctuation is not called a decline |

Rows are ranked by `clicks_delta` ascending — biggest absolute loss first. The
applied values come back in `assumptions`; quote them.

## Step 6 — Report

Lead with the split, because the split **is** the diagnosis:

**Total decline:** −340 clicks across 14 pages

| Cause | Pages | Clicks lost | Reading |
|---|---|---|---|
| `position_drop` | 8 | −210 | Competitors gained. Likely one event, see below |
| `impressions_drop` | 3 | −80 | Demand fell — not a site problem |
| `ctr_drop` | 2 | −40 | Title issue |
| `unclear` | 1 | −10 | Page appeared or vanished — needs a look |

Then the worst pages within each cause, with their actual numbers:

| Page | Clicks A → B | Position A → B | Attribution |
|---|---|---|---|
| /example-page/ | 120 → 40 | 4.2 → 9.8 | `position_drop` |

Close with one sentence naming the most likely single cause, and say explicitly which
part of the decline is **not** the site's fault. Clients rarely hear that, and it is
usually the most valuable thing in the report.

## Accuracy rules

- **Never report a decline without stating `excluded_incomplete_days`.** It is the
  difference between a real drop and an artefact.
- Never present `unclear` as "no reason found". It means the page appeared or
  vanished — that is a specific finding requiring a specific follow-up.
- Do not attribute `impressions_drop` to the site's own work without a
  year-over-year check. Seasonality is the most common explanation and the easiest
  to mistake for a failure.
- One attribution shared by many pages is one event. Report it as one.
- This tool makes three API calls (a freshness probe plus one page-level scan per
  period, each up to 25,000 rows). Read `scan_capped` before calling the list
  complete.
- The tool classifies but does not advise. Every recommendation is yours to defend.
- Use `data_state: all` (the default) to match the Search Console UI.
