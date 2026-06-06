# acronis skill - the MSP pain it closes

## The pain

Backup verification is the one job an MSP can never let slide, and Acronis Cyber
Protect Cloud makes it a per-tenant chore. Acronis's own Management Portal
Administrator Guide is explicit that partner-level dashboards and reports work
only inside one partner tenant - so the question every backup admin asks each
morning, *whose backups failed last night*, has no single cross-customer view.
You open the console, switch into each customer tenant, read its protection
status, switch back, and repeat - or you export per-tenant reports and stitch
them in Excel. Across thirty or fifty customers that is a daily slog, and it is
the slog MSPs describe on r/msp whenever backup monitoring comes up.

Worse, the failure mode that hurts most is the quiet one. A backup agent that
silently stops checking in is the most common way protection lapses without
anyone noticing - there is no failed-job alert because no job ran at all. The
gap surfaces when a customer asks for a restore that isn't there. Finding those
agents means sweeping every tenant for who has gone quiet, exactly the
cross-tenant view the portal doesn't compose.

Acronis even documents the workaround: its developer monitoring guide tells MSPs
that custom multi-tenant monitoring requires fetching each customer tenant's data
via the API at regular intervals and storing it locally. That is real
engineering most MSPs never staff, so usage-to-billing reconciliation and
backup-SLA reporting fall back to month-end spreadsheets.

## What this skill does about it

It is that local mirror Acronis points you at - already built. After one `sync`,
every tenant, agent, and usage metric lives in a local SQLite store, and the
cross-tenant questions become one offline query:

- **`acronis-cli health`** - backup success / failure / stale across your entire
  book of customers in one table, the morning answer the portal can't compose.
- **`acronis-cli agents stale --older-than 7d`** - every silently offline agent
  across all tenants, sorted by customer, before the restore request comes in.
- **`acronis-cli freshness --sla 48h --breached`** - the customers who have gone
  too long without a good backup, flagged against your SLA.
- **`acronis-cli coverage --unprotected`** - tenants billed for protection that
  has no online agent or no recent successful backup: your highest-liability
  accounts.
- **`acronis-cli reconcile usages`** - usage with no matching SKU and paid SKUs
  with zero usage, per tenant, so month-end invoices match reality.

## Status

Beta. Validated against the Acronis API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
