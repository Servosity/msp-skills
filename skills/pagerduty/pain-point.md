# pagerduty skill - the MSP pain it closes

## The pain

On-call is where good teams quietly bleed out. Catchpoint's 2025 SRE survey
found nearly **70% of responders say on-call stress drives burnout and
attrition**, and a 2025 Splunk study tied **73% of outages to alerts that got
ignored** - the predictable result when the average on-call engineer fields
about 50 pages a week and only a handful actually matter. The signal drowns in
the noise.

It gets worse at review time. The numbers leadership asks for - mean time to
acknowledge, mean time to resolve, who is carrying the on-call load, which
services are the noisiest - either sit behind PagerDuty's paid Analytics tier or
get hand-stitched from a CSV export the night before the QBR. And the gap in
next week's on-call schedule stays invisible until an incident finds the hole
first.

For an MSP running incident response across many clients, that is three open
loops at once: too much noise to triage, no cheap way to report on it, and no
early warning when coverage lapses.

## What this skill does about it

PagerDuty syncs into a local SQLite mirror, so the questions that used to be a
portal expedition become one offline command:

- **`pagerduty-cli pulse`** - what's open right now, bucketed by service and
  sorted by how long the oldest unacknowledged incident has waited. Triage in
  one screen.
- **`pagerduty-cli insights mttr --by service --since 30d`** - MTTA and MTTR per
  service, computed offline from synced incidents and log entries. QBR numbers
  without the paid Analytics add-on.
- **`pagerduty-cli insights responders --since 30d`** - per-responder page, ack
  and resolve counts plus the off-hours share - the on-call fairness and burnout
  signal.
- **`pagerduty-cli audit coverage --severity high`** - services whose escalation
  chain is broken: empty tiers, no policy, or a chain that resolves to a single
  person.
- **`pagerduty-cli audit schedule-gaps --days 14`** - future windows where a
  schedule has nobody on call, surfaced before the incident does.

## Status

Beta. Validated against the PagerDuty API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
