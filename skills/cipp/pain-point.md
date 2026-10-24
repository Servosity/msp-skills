# cipp skill - the MSP pain it closes

## The pain

CIPP (the CyberDrain Improved Partner Portal) is how a lot of MSPs run Microsoft
365 across every client tenant from one place, and it already answers many
estate-wide questions itself: its All Tenants views cover the MFA report,
Standards alignment and the licence report, and its API returns one estate-wide
response for `tenantFilter=AllTenants`.

Those views answer questions about the current state. An AI agent working from live
answers alone starts over on every question, and comparing this quarter with last
means keeping dated copies yourself. Joining what CIPP returns (which tenant lacks
a Conditional Access policy most of the fleet has, what changed in admin roles
since last month) is the kind of join an agent can do in seconds from a local
copy.

Scripting it has its own costs. Per the [CIPP API documentation](https://docs.cipp.app/api-documentation/setup-and-authentication),
each API client is limited to 100 requests per 10 seconds. And some CIPP write
endpoints answer HTTP 200 with the failure written inside the response's
`Results` (CIPP's `Invoke-EditUser`, for example), so a batch has to read every
response to know what really happened.

## What this skill does about it

It is local-first: it keeps a copy of what CIPP returns where an AI agent can
search, join and compare it offline. You already have this data through CIPP.

- **`cipp-cli fanout --endpoint /ListMFAUsers --all-tenants --save`** - runs one
  read for each tenant and saves the rows in a local SQLite store, with HTTP 429
  Retry-After handling and `--resume` after a halt. Each save is a dated snapshot.
- **`cipp-cli posture --dimension mfa`** (also `ca`, `standards`, `dlp`,
  `retention`, `labels`) - one tenant-by-metric table from the saved rows.
- **`cipp-cli ca diff`** - the Conditional Access policy one tenant lacks that
  most of the fleet has.
- **`cipp-cli changes --resource roles --since 7d`** - rows added, removed or
  changed between two of your own snapshots.
- **`cipp-cli bulk --from offboards.csv --execute`** - a CSV-driven batch of user
  and mailbox actions: plan-only without `--execute`, a non-zero exit when CIPP
  reports a failure inside an HTTP 200, and per-row `--resume`.

The local copy is not encrypted. `cipp-cli doctor` prints where it lives,
`cipp-cli local-data purge --yes` deletes the store, cache and learn journal,
`cipp-cli auth logout` removes stored credentials, and `CIPP_NO_LOCAL_DATA=1`
keeps no local copy of CIPP data (both from cipp 0.1.13; check `cipp-cli --version`).

## Status

Live-verified: Abhi Saini, Bearium Networks (MSP), ran cross-tenant `fanout` against a live CIPP tenant on 2026-08-13 ([receipt](https://compoundingteams.com/)).
