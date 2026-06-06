# ninjaone skill - the MSP pain it closes

## The pain

NinjaOne is one of the most-loved RMMs for real-time, per-device work - monitoring,
patching, remote control. The recurring complaint MSP owners raise is about the layer
above a single device: **reporting and the cross-client rollup.** Independent reviews
note that NinjaOne's built-in reports are "not consistently executive-ready" and that
client-facing reporting often takes effort or a third-party BI overlay, a gap still open
in 2026 (Flamingo, *NinjaOne Review*, 2026). The same reviewers call out the lack of a
single unified multi-tenant view (Flamingo, *NinjaOne vs Intune: The Multi-Tenancy
Dealbreaker*, 2026).

The practical result for an owner running dozens of client organizations: the questions
you ask every week - *which clients are behind on patches, which endpoints have no
backup, how far did that threat spread, which machines are running an end-of-life OS* -
are not one click. They are a per-org report run N times and re-totaled in a spreadsheet,
because the API returns per-device rows and nothing aggregates them across the fleet.

## What this skill does about it

It syncs your whole NinjaOne estate into a local SQLite mirror, then answers the
cross-fleet questions as one local join - offline, instant, and shaped so an AI agent
sees the answer instead of pages of JSON:

- **`ninjaone-cli patch-compliance --min-pct 95`** - one compliance row per organization
  (percent OS- and software-patched, failed counts, worst-offender device); filter to the
  clients below your bar.
- **`ninjaone-cli backup-coverage`** - every device with no backup usage, grouped by
  organization: the unprotected endpoints NinjaOne has no single screen to list.
- **`ninjaone-cli av-sweep --threat "Trojan.Generic"`** - turn one detection into a
  fleet-wide blast-radius map; or `--definition-stale-days 7` for endpoints with stale AV.
- **`ninjaone-cli fleet-health`** - a transparent 0-100 score per organization from patch,
  backup, AV, and stale-device signals, with the deductions itemized.
- **`ninjaone-cli drift --metric patch`** - the week-over-week answer no console keeps:
  which organizations got better or worse since the last snapshot.

## Status

Beta. Validated against the NinjaOne API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
