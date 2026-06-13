# veeam skill - the MSP pain it closes

## The pain

Backups that fail silently are the recurring nightmare on r/msp and the Veeam
community forums: the job stops succeeding, nobody notices, and the MSP finds out
the week the customer needs a restore. Veeam Service Provider Console (VSPC) gives
MSPs a real multi-tenant console, but it is organized per-tenant - the questions
that matter most across a book of business ("which customers have a failed backup
right now?", "who is past their RPO?", "which agents went stale this week?") mean
clicking into each company one at a time. The data is all in VSPC; getting a
fleet-wide answer out of it is the work, and it lands at the worst moments: the
morning standup, a restore request, the monthly license invoice.

For an owner-operator MSP, backup health is the service you actually sell. A
cross-tenant blind spot is not a reporting annoyance - it is the gap between "we
have backups" and "we have backups that work."

## What this skill does about it

It syncs every tenant's jobs, agents, alarms, protected workloads, and license usage
into a local SQLite mirror, then answers the cross-company questions directly:

- **`fleet-health`** - one pane: jobs by last status, agents online/offline, and active alarms per tenant.
- **`stale-backups --days 3`** - every job and agent whose last successful run is older than N days, across all tenants, sorted by staleness.
- **`at-risk --rpo 24h`** - protected workloads whose latest restore point is past the RPO threshold or missing - the data that would be lost on failure today.
- **`alarms-triage --severity Error`** - active alarms deduped and grouped by company and severity, so a noisy feed becomes the handful of problems worth acting on.
- **`license-usage`** - per-organization license consumption with the delta since the last run, for billing before overage.

## Status

Beta. Validated against the Veeam API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
