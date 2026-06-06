# huntress skill - the MSP pain it closes

## The pain

Huntress is a favorite on r/msp, but the recurring complaint from multi-tenant
shops is the same: visibility is scoped one organization at a time. There is no
native cross-client incident queue, no fleet-wide posture rollup, and no built-in
way to reconcile invoiced seats against deployed agents. At 20, 40, or 80 client
tenants, "which fire do I put out first?" becomes a tab-switching exercise across
the portal, and billing drift between what you're invoiced and what you've actually
deployed is caught only when someone reconciles by hand at month-end. The API can
answer one org and one entity per call - it never returns the cross-tenant view an
MSP owner actually needs at triage time or QBR time.

## What this skill does about it

- **`fleet-incidents --sort age`** - one age-sorted incident queue across every
  organization, so the oldest, most urgent incident anywhere is at the top.
- **`coverage-gaps`** - posture exposure rolled up worst-first: stale callbacks,
  disabled Defender, disabled firewall, per org.
- **`blast-radius --indicator <ioc>`** - correlate an IP, hash, or hostname across
  incidents, agents, and external ports in every client tenant at once.
- **`billing-reconcile`** - the delta between invoiced seats and agents actually
  deployed, surfaced before the customer notices.
- **`handoff --since 12h`** and **`org-scorecard --org <id>`** - shift-change and
  QBR rollups built from synced history the live API throws away.

## Status

Beta. Validated against the Huntress API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
