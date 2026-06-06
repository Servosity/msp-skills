# sentinelone skill - the MSP pain it closes

## The pain

SentinelOne's management console organizes customers as a hierarchy - Global, then
Accounts, Sites, and Groups - and the console view and the Management API both act on a
*selected* scope. For an MSP or MSSP running SentinelOne across a book of customer Sites,
that means every cross-client question is a scope-switch: to see who has the worst open
threats, whose agents went dark, who is stuck on an old agent build, or who quietly slipped
from Protect to detect-only, you flip the scope selector and re-read the same screens, Site
by Site. This single-pane gap is a recurring complaint among MSPs on
[r/msp](https://www.reddit.com/r/msp/) managing EDR across many tenants: there is no native
view that ranks every client's threats or fleet health *side by side*, so triage,
QBR prep, and "is everyone actually protected right now" all become manual, per-tenant
assembly.

The second cliff is silent erosion. Protection state drifts between syncs - an endpoint
falls behind on version, an agent stops checking in, a rollout stalls mid-wave, an
auto-mitigated threat is re-opened - and the console surfaces each of these only as a
separate per-Site filter, never as one fleet-wide "what changed since yesterday" answer.
Nobody has time to scope into every Site and read every filter every morning, so the gap
that matters is usually the one no one looked at.

## What this skill does about it

- **`sentinelone-cli threats triage`** - one ranked, cross-site worklist of every open
  threat, scored by confidence x severity x age, so the morning triage order needs zero
  console scope flips.
- **`sentinelone-cli threats blast-radius "<hash>"`** - trace one threat across the whole
  fleet: every endpoint it touched, which are mitigated vs still active, and the spread
  timeline - the containment view incident response actually needs.
- **`sentinelone-cli fleet-health stale`** - rank endpoints by a composite decay score
  (last check-in, last scan, out-of-date version, detect-only mode, infection) so the
  dark and under-protected agents surface worst-first instead of hiding per-Site.
- **`sentinelone-cli coverage gaps`** - list the specific endpoints in detect-only mode or
  with Ranger / firewall disabled - the "are we actually protecting everyone?" view.
- **`sentinelone-cli whatchanged --since 24h`** - diff the fleet against an earlier snapshot:
  new threats, agents gone offline, version changes, and Protect-to-detect flips, across
  every Site at once.
- **`sentinelone-cli posture`** - a one-row-per-Site scorecard (health %, coverage %,
  open threats, oldest unresolved, version compliance) for the morning review or a QBR.

## Status

Beta. Validated against the SentinelOne API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
