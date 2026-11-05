# dropsuite skill - the MSP pain it closes

## The pain

**A backup that silently stops is the one nobody notices.** An r/msp thread,
"Office365 backup alternative to Dropsuite" (December 2021), describes a client
whose backups had been disabled and went undiscovered for over three months. The
replies did not blame the product so much as the process: nobody had checked, let
alone tested, that client's backups. The Dropsuite partner portal shows one
organization at a time, so a real morning check across a whole book of clients
is a click-through that does not happen every day, and the gap shows up at
restore time.

**Every cross-client question starts with collecting tokens.** Dropsuite's API is
organized per organization: backup data, tenants, delegated users and retention
policies each take that organization's own token, and only the organization list
takes the reseller token. "Are all my users backed up?", "do my seat counts match
what I bill?" and "which client does jane@example.com belong to?" each mean
fetching every client's token first and then walking every workload list.

**Proof of protection is asked for after the fact.** A cyber-insurance form, an
audit or a QBR wants dated evidence of coverage, recent backups and restore
testing. The API answers only the current state, so last quarter's answer is gone
unless someone wrote it down.

## What this skill does about it

- `dropsuite-cli health --stale-after 48h --failing --agent` - every failed, stuck
  or stale mailbox, OneDrive, SharePoint site, Team and drive across your active
  clients in one run, and a saved snapshot for later comparison.
- `dropsuite-cli drift --since 7d --agent` - what changed between health runs:
  objects that left backup and backups that stopped advancing, the silent stop
  a single status check misses.
- `dropsuite-cli coverage --agent` - tenant mailboxes that are not added to
  backup, with coverage percent per client.
- `dropsuite-cli billing --invoice psa-seats.csv --flagged-only --agent` - seat
  counters next to what your PSA invoices, with over-allocation and suspended
  clients still holding seats flagged.
- `dropsuite-cli evidence --org 26561-14 --window 90d --markdown` - one client's
  dated proof of protection, including the restore tests you recorded with
  `restore-test record`.

All of it runs from one reseller credential; `--org` resolves each client's token.

## Status

Beta. Validated against the Dropsuite (NinjaOne) API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
