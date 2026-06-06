# microsoft-graph skill - the MSP pain it closes

## The pain

On August 28, 2026 Microsoft retires the Microsoft Graph CLI (mgc). It has been
deprecated since September 2025 - no new features, only critical security fixes - and
Microsoft's recommended migration path is the Microsoft Graph PowerShell SDK
(Microsoft 365 Developer Blog, "Microsoft Graph CLI retirement"). For an MSP, that
trade is real: the lightweight, cross-platform binary that fed tenant-reporting scripts
is going away, and the replacement drags a .NET / PowerShell runtime onto every machine
and agent that has to run it.

The deeper pain is the one mgc never solved either. The questions an MSP actually asks
about a Microsoft 365 tenant - how much license spend can we reclaim, who can administer
this tenant right now, which devices are drifting out of compliance, where does this
tenant stand before the QBR - each span several Graph entities, and no single Graph
endpoint returns any of them. The M365 admin center, Entra, Defender, and Intune portals
answer one object at a time, so every one of those questions turns into a CSV export and
a spreadsheet join, or a click-path across modules - repeated per client, every month.
Graph also throttles bulk reads and paginates behind `@odata.nextLink`, so even a script
that wants "all users with their licenses" has to fetch, page, cache, and join by hand.

## What this skill does about it

One cross-platform Go binary - no .NET or PowerShell runtime - that `pull`s the
MSP-relevant Graph surface into a local SQLite mirror, then answers the cross-entity
questions offline:

- `microsoft-graph-cli licenses waste --agent` - ranks every SKU by prepaid-but-unused
  seats, so you walk into the renewal knowing exactly what to reclaim.
- `microsoft-graph-cli admins audit --agent` - lists every privileged-role holder with
  guest/disabled risk flags: the monthly "who can administer this tenant" review in one
  command.
- `microsoft-graph-cli security triage --since 24h --agent` - groups the open alerts that
  are new since yesterday by severity and source, without paging the Defender portal.
- `microsoft-graph-cli managed-devices drift --days 30 --agent` - the weekly compliance
  ticket queue: non-compliant, unencrypted, or stale Intune devices mapped to their user.
- `microsoft-graph-cli tenant snapshot --agent` - one posture summary across users,
  license waste, admins, alerts, and device drift: the "where does this tenant stand"
  answer no single Graph call returns.

## Status

Beta. Validated against the Microsoft Graph API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
