# levelio skill - the MSP pain it closes

## The pain

Level is a fast, modern RMM - MSPs reach for it for lightweight monitoring,
scripting, and automation. But like every RMM, the console answers questions one
device at a time, and the recurring complaint on
[r/msp](https://www.reddit.com/r/msp/) isn't monitoring - it's *reporting across
the whole fleet at once*: which machines quietly went dark, which clients are
furthest behind on patches, where the alert noise is actually systemic, and what
the per-client posture looks like when a QBR is due.

The practical result: there is no single screen that answers the questions an MSP
owner actually asks across every client at once. You assemble those answers by
hand, group tab by group tab, or you page the live API device by device. So the
numbers that matter at a QBR - patch exposure, dark-device counts, open
criticals per client - are the ones nobody has time to pull.

## What this skill does about it

`levelio-cli` syncs Level into a local SQLite mirror, then answers the
cross-entity questions the portal leaves on the table - instant, offline, and
rate-limit-free:

- **`levelio-cli at-risk --top 20`** - the worst endpoints across every axis at
  once (active alerts, pending patches, low security score, days dark) as one
  weighted, ranked "fix these first" list.
- **`levelio-cli stale --days 30`** - the machines that quietly stopped checking
  in, before the client calls to complain. A time-window the console makes you
  filter for by hand.
- **`levelio-cli patch-posture --category security`** - fleet-wide update
  exposure (available vs installed vs errored), so you can see and act on the gap
  at a glance.
- **`levelio-cli client-scorecard`** - one row per client: device count, online
  %, open criticals, average security score, stale count, and patch exposure -
  the QBR-ready rollup the portal never assembles.
- **`levelio-cli alert-triage --severity critical`** - unresolved alerts
  clustered by group and severity, so systemic fires surface above one-off noise.

## Status

Beta. Validated against the Level API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here once it exists.
