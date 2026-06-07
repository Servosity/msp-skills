# rocketcyber skill - the MSP pain it closes

## The pain

Technicians on r/msp describe SOC work in one phrase: **alert fatigue**. RocketCyber
gives you a managed SOC, but answering a single question - *what actually broke
across my clients overnight?* - means logging into the console, switching to each
client account, and reading the incidents, events, and agents tabs one at a time.
The data is there; the cross-account, answer-first view is not. Stale suppression
rules quietly pile up and can hide real detections, and at QBR time you are
screenshotting secure-score charts and computing MTTR by hand because the console
shows the dashboard but won't hand you the trend or the number.

## What this skill does about it

- **`triage`** - one ranked board of open incidents, event verdict counts, and
  offline agents across every client account. "What broke overnight" in one call.
- **`agents stale`** - devices that stopped reporting beyond a window, grouped by
  client. A fleet-hygiene sweep instead of paging the live agents endpoint.
- **`incidents mttr`** - mean/median time-to-resolve plus open-incident aging
  buckets, straight from incident timestamps. QBR-ready, no spreadsheet.
- **`defender riskiest`** - devices-at-risk ranked by weighted malicious and
  suspicious detection counts. Tells you which machines to touch first.
- **`suppression audit`** - suppression rules classified by status and age,
  flagging stale rules that may be masking live detections.

## Status

Beta. Validated against the RocketCyber Customer API v3 surface; the closed-loop
receipt (a named MSP running it live in their production tenant at a Build Session)
is tracked separately and added here as `video.md` once it exists.
