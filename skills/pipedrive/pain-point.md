# pipedrive skill - the MSP pain it closes

## The pain

Reporting is the single most-cited complaint about Pipedrive across G2, Capterra,
and Trustpilot review roundups: the built-in Insights module handles the basics
(activity counts, deals by stage, revenue by rep), but anything cross-entity means
exporting to a spreadsheet and rebuilding the math by hand. The questions an owner
actually asks at a Monday pipeline review - *which deals are silently dying, what's
my weighted forecast, which reps are really contributing, which deals are stuck* -
all live in different screens or behind a CSV export.

Meanwhile the data decays. CRM contact data goes stale at roughly 30% a year, and a
deal nobody has logged an activity against just sits in the pipeline until it's
already cold. As one Capterra reviewer put it, Pipedrive is "great for managing
deals, not for managing customers" - prepping for a single call means stitching
together the person, their organization, their open deals, the last and next
activity, and recent notes from five separate screens.

Sources: [Top Pipedrive problems users complain about](https://nethunt.com/blog/top-pipedrive-problems-users-complain-about-and-how-to-solve-them/)
(NetHunt, aggregating G2/Capterra/Trustpilot reviews); recurring threads on r/sales
and the Pipedrive Community forum about reporting limits and pipeline hygiene.

## What this skill does about it

It keeps a local SQLite mirror of your pipeline (`sync`) and answers the
cross-entity questions Insights can't, in one command each:

- **`stale`** - open deals nobody has touched in N days, ranked by the dollar value
  at risk. The highest-leverage read in the CLI: catch deals before they're lost.
- **`forecast`** - weighted pipeline value (deal value times stage probability) by
  pipeline, plus what's expected to close this period - no spreadsheet rebuild.
- **`aging`** - the deals stuck in a stage longer than that stage's typical dwell
  time, so you find the bottleneck and the specific deals rotting in it.
- **`leaderboard`** - per-rep open/won/lost, weighted pipeline, won value, and
  activity count over a window - team reviews without touching a spreadsheet.
- **`who`** - one card for a contact: their org, open deals and value, last and next
  activity, and recent notes, joined from the local store before a call.

## Status

Beta. Validated against the Pipedrive API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
