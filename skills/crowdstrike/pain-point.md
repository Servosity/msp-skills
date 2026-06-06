# crowdstrike skill - the MSP pain it closes

## The pain

The Falcon console scopes to one CID at a time. For an MSSP running CrowdStrike
across a book of client tenants, Flight Control gives you parent-level child
management - but every cross-client question still means switching CID and
re-reading the same screens tenant by tenant. On r/msp and in the CrowdStrike
community, MSPs raise the same single-pane gap: there is no one view that ranks
every CID's open detections, critical vulnerabilities, or sensor health side by
side. So the morning triage, the post-Patch-Tuesday vuln sweep, and the QBR
posture deck all turn into a manual loop of "open Flight Control, pick a child,
read the filter, write it down, pick the next child."

Posture also erodes quietly between logins. A sensor stops checking in, a host
falls out of a prevention policy, a tenant's critical-vuln count climbs - and
unless someone scopes into that exact CID and reads that exact filter, it goes
unnoticed until it becomes an incident. Spotlight and the host list surface these
as per-tenant views, never as one fleet-wide "what got worse since last week" or
"which sensors are silent right now" answer.

## What this skill does about it

It syncs every child CID into one local SQLite store keyed by CID, then answers
the book-wide questions directly - instantly and offline:

- `crowdstrike-cli fleet alerts --status new` - one severity-sorted detection queue across every tenant, not CID-by-CID triage.
- `crowdstrike-cli fleet vulns --severity critical` - rank Spotlight criticals across the whole fleet after Patch Tuesday in one command.
- `crowdstrike-cli fleet stale --days 14` - catch every silent sensor across all tenants in a single sweep.
- `crowdstrike-cli fleet scorecard` - a per-CID posture board (hosts, coverage, open criticals, vulns, policy) for the QBR deck.
- `crowdstrike-cli fleet policy-drift` - surface the tenants that fall short of your prevention-policy baseline before they bite you.

## Status

Beta. Validated against the CrowdStrike API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
