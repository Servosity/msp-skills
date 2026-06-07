# skykick skill - the MSP pain it closes

## The pain

On r/msp, the steady refrain about Microsoft 365 backup is "trust but verify."
Operators repeatedly warn each other that backup is set-and-forget right up until
it silently isn't: a mailbox stops snapshotting, a newly onboarded employee never
enrolls because autodiscover was off, or a tenant's retention quietly drifts below
what the contract promised. The worst time to discover any of these is during a
restore request, when it's already too late.

SkyKick Cloud Backup makes this easy to miss by design. The partner portal shows
one customer at a time, and there is no single screen that answers the only
question that matters on a Monday morning: *across all 30-50 of my tenants, who is
not fully protected today?* So the verification check - the thing that catches the
silent failure before the customer does - is exactly the chore that falls off the
weekly routine.

The September 2025 move of SkyKick Cloud Backup onto ConnectWise's
`apis.cloudservices.connectwise.com` host added a second paper cut: tooling and
scripts pointed at the old `apis.skykick.com` endpoint simply stopped returning
data.

## What this skill does about it

It syncs every SkyKick subscription plus per-tenant settings, retention,
autodiscover state, snapshot stats, mailboxes, sites, and alerts into a local
SQLite store, then answers the fleet questions the per-tenant API cannot:

- **`fleet-health --flag-gaps`** - every tenant with at least one protection gap
  (Exchange/SharePoint off, autodiscover off, unprotected mailboxes or sites, stale
  backup), in one cross-tenant table.
- **`stale-snapshots --hours 48`** - every mailbox whose last snapshot is older than
  your threshold, fleet-wide, with never-snapshotted mailboxes listed first.
- **`coverage-gaps --type all`** - discovered-but-unprotected mailboxes and
  SharePoint sites, the post-onboarding and post-churn reconciliation gap.
- **`retention-audit --floor-days 365`** - tenants whose retention falls below your
  compliance floor, graded, with unknowns flagged rather than silently passed.
- **`drift`** - what protection state changed between your two most recent syncs, so
  a backup that got turned off shows up before a customer needs it.

## Status

Beta. Validated against the SkyKick (ConnectWise Cloud Services) Backup API
surface; the closed-loop receipt (a named MSP running it live in their production
tenant at a Build Session) is tracked separately and added here as `video.md` once
it exists.
