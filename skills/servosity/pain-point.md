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

- `attention` - one screen across every client: open issues and stale backups,
  ranked per company, snapshotted on every run so `drift` can compare days.
- `stale-backups` - every client with a backup that has not succeeded in N days -
  the Friday-email list; `email-draft --stale` writes the follow-up emails from it.
- `drift` - what got worse and what recovered since yesterday, so Monday starts
  with situational awareness instead of a blank slate.
- `triage` - batch-ignore, archive, reactivate, or comment on known-safe alert
  noise in one invocation (opt-in `--dry-run` preview), so the queue shows only what's new.
- `qbr` / `qbr-all` - the backup section of a client's Quarterly Business Review
  as Markdown, HTML, or PDF - or the whole book in one pass at quarter-end.
- `bill --reconcile` / `unprovisioned` / `storage-trend` - the revenue surface:
  bill-vs-invoice drift, installed-but-idle agents, and capacity forecasts per client.

## Status

Already used inside Servosity's own backup/DR operations; published here for MSP
partners. The public surface is in beta and being validated with MSPs in live Build Sessions.
