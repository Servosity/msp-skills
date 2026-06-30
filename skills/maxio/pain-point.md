# maxio skill - the MSP pain it closes

## The pain

Maxio (formerly SaaSOptics and Chargify) bills the recurring revenue, but
getting the *story* of that revenue out of it is the sore spot. On Maxio's own
[G2 reviews](https://www.g2.com/products/maxio-formerly-saasoptics-and-chargify/reviews),
operators repeatedly call the reporting weak for the metrics that actually run a
subscription business - MRR, churn, ARPA, retention - and describe falling back
to third-party applications on imported billing data to compute them. Reviewers
flag simple asks with no clean answer in the portal, like listing the customers
who signed up in a given window and the revenue they brought. On top of that, the
live API returns only point-in-time figures - there is no endpoint that
reconstructs the historic movement and retention curves a finance lead needs at
board time, so they only exist if something snapshots them over time.

For an MSP or SaaS operator, that means board-prep and QBR questions turn into a
manual export-and-spreadsheet loop every month instead of a one-line answer.

## What this skill does about it

It syncs Maxio into a local SQLite mirror, snapshots each sync into a time series,
and computes the revenue math locally - so the history survives even as the live
API retires its analytics endpoints:

- `maxio-cli mrr waterfall --since 2026-01-01 --group-by month` - the
  New/Expansion/Contraction/Churn/Reactivation movement, month by month.
- `maxio-cli retention --since 2026-01-01 --group-by month` - net and gross
  revenue retention, logo and revenue churn, and quick ratio over the window.
- `maxio-cli new-customers --since 3m` - the new logos in the window and the MRR
  they brought, the ask reviewers say the portal can't answer cleanly.
- `maxio-cli triage --limit 20` - a ranked list of the accounts that need
  attention right now (past-due, large upcoming renewals, concentration).
- `maxio-cli reconcile --since 2026-01-01` - where normalized MRR diverges from
  what was actually invoiced, with the mismatches flagged.

## Status

Beta. Validated against the Maxio API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
