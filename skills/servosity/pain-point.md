# Servosity skill - the MSP pain it closes

## The pain

Backup and DR is where "silent failure" hurts most. The pains MSP owners name:

- **Silent backup failures discovered too late.** A backup that quietly stopped
  succeeding is invisible until a client needs a restore - the worst possible
  moment to find out.
- **No fleet-wide view.** Each client's backup state lives in its own portal
  view; there is no single screen that says "across my whole book, here is what
  is stale, failing, or in-flight right now."
- **Alert-queue noise buries the real failure.** Dozens of repeat and known-safe
  issues pile up per client; the one that matters hides in the pile.
- **Per-client questions mean portal archaeology.** Answering "is this client OK?"
  means clicking through metadata, three backup engines, contracts, and issues by hand.

## What this skill does about it

It turns the partner portal's per-client views into fleet-wide, offline-fast
intelligence:

- `attention` - one screen across every client: open issues, stale backups, and
  in-flight DR events, ranked per company.
- `stale-backups` - every client with a backup that has not succeeded in N days,
  sliced by engine - the list you email clients about on Friday.
- `drift` - what got worse and what recovered since yesterday, so Monday starts
  with situational awareness instead of a blank slate.
- `triage` / `clear` / `stale-issues` - batch-ignore, archive, or clear known-safe
  alert noise for a client in one planned command, so the queue shows only what's new.
- `company show` / `find` - one command assembles a client's full backup picture
  (metadata, contracts, three engines, open issues); `find` is one FTS5 query across the whole fleet.

## Status

Already used inside Servosity's own backup/DR operations; published here for MSP
partners. Beta for the public surface. The closed-loop receipt (an MSP partner
running it live against their fleet) is tracked separately and added as
`video.md` once it exists.
