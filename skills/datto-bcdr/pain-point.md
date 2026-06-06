# datto-bcdr skill - the MSP pain it closes

## The pain

On r/msp, the recurring Datto BCDR complaint isn't that backups fail loudly - it's
that they fail *quietly*. A protected machine's screenshot/boot verification starts
failing, or an agent stops taking local snapshots, and nobody notices: the Partner
Portal shows backup health one appliance at a time, so a single bad agent buried on
one SIRIS among forty clients never bubbles up. The gap surfaces at the worst possible
moment - restore time, with the client already down.

The deeper pain is the missing fleet view. Owners describe walking every SIRIS and ALTO
by hand before a QBR, a renewal, or a cyber-insurance audit, because the one question
they actually need answered - "are all my backups, across all my clients, recoverable
right now?" - has no cross-client rollup in the portal. So "prove the backups are good"
becomes an afternoon of clicking instead of one number.

## What this skill does about it

It mirrors the whole estate into local SQLite and answers across it:

- `screenshots --failed --stale-days 7` - every silently-unbootable backup, fleet-wide,
  oldest failures first, grouped by client.
- `recoverability` - one defensible KPI: the percent of fleet agents whose latest
  recovery point is both fresh and screenshot-verified bootable.
- `stale-backups --local-days 1 --offsite-days 3` - agents that quietly stopped taking
  local snapshots or offsite syncs, before anyone needs them restored.
- `client-risk --top 10` - which clients to call first, ranked across screenshot
  failures, stale backups, open alerts, and storage pressure.
- `client-report "Acme Corp"` - one QBR-ready backup-health bundle for a single client.

## Status

Beta. Validated against the Datto BCDR API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
