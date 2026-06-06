# superops skill - the MSP pain it closes

## The pain

SuperOps' pitch is one database for PSA and RMM - and that part is real. But the
console still answers one entity at a time, and the platform's own AI is, as
third-party reviewers put it, "more roadmap than reality" (Flamingo, "SuperOps
Review for MSPs," 2026). So the questions an MSP owner actually asks at month-end
or before a QBR are still cross-entity questions no single console screen composes:

**1. "Who's about to breach SLA, and on whose queue?"** SuperOps ships scheduled
SLA/uptime reports for clients, but a live "who is at risk right now, grouped by
the tech who owns it" view requires joining open tickets to their SLA targets,
status, and assignee - a join no single API call returns.

**2. "How much billable time is sitting unreconciled for this client?"** The list
API does not expose a per-entry "already billed" flag, so the number you
sanity-check before invoicing - billable worklog totaled per client - means
exporting worklogs and summing by hand.

**3. "Which endpoints are both unpatched and actively alerting?"** Patch status
lives on the asset; the alert lives on the alert feed; the console shows them on
different screens. Intersecting them is a manual cross-reference.

And the GraphQL API makes the do-it-yourself version painful: it rate-limits
reads, and its list payloads omit some of the very links these questions need
(asset-to-ticket, aggregated child-activity timestamps), so any script has to
fetch, cache, and join locally rather than ask the API directly.

## What this skill does about it

- `superops-cli sla-watch --by tech --window 4h` - every open ticket breaching or
  about to breach its resolution SLA, grouped by technician (or `--by client`).
  The dispatcher's morning triage.
- `superops-cli client-360 "Acme Corp"` - the client plus its sites, users,
  contracts, open tickets, assets, and open invoices in one bundle. Six console
  tabs in one command, before the QBR.
- `superops-cli at-risk-assets --client Acme` - endpoints whose patch status
  signals a missing or critical patch that also carry an unresolved alert.
  Remediation, prioritized.
- `superops-cli alert-coverage --client Acme` - alerts split into resolved vs
  unresolved per client, so the clients with alerts nobody is handling surface.
- `superops-cli unbilled --since 2026-05-01` - billable logged worklog totaled per
  client (the reconciliation target), so you see where billable time is
  concentrated before the invoice run.

The first run, a `sync` pulls your tenant into local SQLite; after that these
views run offline, instant, and free of rate limits and the API's missing-link
traps.

## Honest limits

These cross-entity views are computed from what the SuperOps list API actually
exposes, and the CLI is candid about the proxies it uses:

- `unbilled` surfaces *billable* logged time per client, not a strict
  worklog-minus-invoice diff - the API carries no per-entry billed flag.
- `at-risk-assets` and `alert-coverage` use an *unresolved alert* as the proxy
  for "currently causing pain," because the asset-to-ticket link is not on the
  list payloads.
- `context-ticket` bundles the locally synced ticket, its worklogs, client, and
  SLA; conversation and note threads are fetched live with `superops-cli tickets`.

## Sources

- Flamingo, "SuperOps Review for MSPs (2026): Pros, Cons & Pricing" -
  flamingo.run/blog/superops-review (notes the platform's AI is "more roadmap
  than reality" and that PSA depth still trails the legacy players).
- SuperOps product positioning, "AI-Native UEM & PSA-RMM for MSPs and IT Teams" -
  superops.com (one unified PSA-RMM database; scheduled SLA/uptime reports).
- The CLI's own documented API gaps - each command's `--help` "Note:" and the
  README "Known gaps" section: no per-entry billed flag, asset-to-ticket link
  absent from list payloads, child-activity timestamps not aggregated server-side.

## Status

Beta. Validated against the SuperOps API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
