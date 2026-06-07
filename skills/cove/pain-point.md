# cove skill - the MSP pain it closes

## The pain

MSPs running Cove Data Protection (the N-able backup platform, formerly
SolarWinds Backup) keep hitting the same wall in the backup.management console:
it scopes to one customer at a time and keeps no history. Threads on r/msp and
the MSPGeek community ask the same questions over and over - "how do I get one
cross-customer list of every failed backup?" and "how do I trend storage growth
when the dashboard only shows today's number?" The answer in the console is
always the same: click into each customer, eyeball the dashboard, repeat
tomorrow. Month-end billing adds its own tax - exporting usage and decoding the
cryptic statistic column codes (SKU, used storage, M365 seat counts) by hand
against a legend in another tab.

The cost is real: a fleet-wide failure can hide behind a per-customer view, and
a silently stale device (one whose last status reads fine but hasn't actually
succeeded in days) is invisible until a restore fails.

## What this skill does about it

- `cove-cli devices failures --since 24h --agent` - one sweep across the whole
  partner tree returns every device whose last session failed, aborted, errored,
  or never started, with the F00 status codes decoded to names. The morning
  ticket queue in one command.
- `cove-cli devices stale --days 3 --json` - the silently stale devices the
  last-status view hides, ranked worst-first and grouped by customer.
- `cove-cli fleet health --by partner --json` - the single-pane rollup (healthy,
  failed, stale, never-run) with a per-customer breakdown, for standups and QBRs.
- `cove-cli storage growth --since 7d --agent` - which devices and customers are
  growing storage fastest, computed from timestamped local snapshots the console
  never keeps.
- `cove-cli billing usage --csv` - per-device SKU, used storage, and M365 seats
  with column codes decoded: the month-end invoice export in one line.

## Status

Beta. Validated against the Cove Data Protection API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
