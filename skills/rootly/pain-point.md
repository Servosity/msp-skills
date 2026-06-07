# rootly skill - the MSP pain it closes

## The pain

On-call is where good teams quietly bleed out. The recurring on-call-burnout and
alert-fatigue threads on r/sysadmin and r/sre - and the industry's annual
Catchpoint SRE Report - tell the same story year after year: responders field a
flood of pages, only a handful matter, and the real incident hides in the noise.
The signal drowns, and the people carrying the rotation burn out.

It gets worse at review time. The numbers leadership asks for - mean time to
acknowledge, mean time to resolve, who is carrying the on-call load, which
services keep breaking - either live in the vendor's analytics surface or get
hand-stitched from exports the night before the review. And the gap in next
week's on-call schedule stays invisible until an incident finds the hole first.

For an MSP running incident response across many client services, that is three
open loops at once: too much noise to triage, no cheap way to report on it, and
no early warning when coverage lapses.

## What this skill does about it

Rootly syncs into a local SQLite mirror, so the questions that used to be a portal
expedition become one offline command:

- **`rootly-cli oncall-now`** - who is on call right now across every schedule and
  service, escalation tier included. No Slack archaeology mid-incident.
- **`rootly-cli mttr --by service --since 90d`** - MTTA and MTTR per service,
  computed offline from synced incidents. The review numbers without the portal
  expedition.
- **`rootly-cli coverage-gaps --days 14`** - future windows where a schedule has
  nobody on call, surfaced before a page is missed.
- **`rootly-cli related <incident-id>`** - the past incidents most similar to this
  one, ranked, so you can see how this class of problem played out before.
- **`rootly-cli fixed-last-time <service>`** - mine the resolutions and action
  items from a service's past incidents to surface what actually resolved it.

## Status

Beta. Validated against the Rootly API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
