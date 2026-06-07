# action1 skill - the MSP pain it closes

## The pain

Patch-management discussions on r/msp keep surfacing the same gap with multi-tenant
RMM and patch tools: the console is organized one client organization at a time, but
the questions an MSP owner actually has to answer are fleet-wide. "Which endpoints
are most behind on patches right now?" "Which CVE is sitting on the most machines?"
"What posture number do I put in this client's QBR?" In Action1 each of those means
switching organizations one by one and reading numbers off each dashboard, or
exporting per-org and merging spreadsheets. The data is all there per client - it
just does not roll up across every organization in a single view, which is exactly
the view an MSP needs at patch-review and QBR time.

## What this skill does about it

The skill syncs every organization into a local mirror and turns the cross-org
questions into one command each:

- **`fleet patch-posture`** - every endpoint across all organizations ranked by how
  many updates it is missing, so "who is most behind?" is a single ranked list.
- **`fleet vuln-triage --kev-only`** - CVEs ranked by blast radius across the whole
  fleet, weighted by CVSS and the CISA Known-Exploited flag: what to remediate first.
- **`fleet org-scorecard`** - one posture row per client organization (endpoints,
  missing updates, open CVEs, KEV exposure, stale agents): the QBR number, in one line.
- **`fleet stale --days 14`** - endpoints that stopped checking in across every
  organization, so dark agents do not silently fall out of your patch coverage.
- **`fleet reboot-pending`** - every endpoint fleet-wide waiting on a reboot to finish
  an update: the action queue that actually closes out a patch cycle.

## Status

Beta. Validated against the Action1 API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
