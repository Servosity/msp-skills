# backup-radar skill - the MSP pain it closes

## The pain

A backup that never reports sends no failure email. ScalePad's own partner stories
are the clearest public record of what that costs: the
[Gflex story](https://www.scalepad.com/partner-stories/gflex-reclaimed-hours-with-automated-backup-monitoring)
describes backups that never technically fail going unnoticed for weeks or months,
and a technician spending about four hours a day sorting a flood of backup email
and remembering which errors were one-offs.
[MicroAge Swift Current](https://www.scalepad.com/partner-stories/microage-swift-current-saves-hours-every-week-with-backup-radar)
put its manual review at two hours a day before automated monitoring.

Backup Radar closes the collection gap: every result lands in one place and a
missing result raises a no-result flag. What it does not close is the morning
review, which is still a person in a portal answering the same questions by hand:

- Which failures are new, and which have been failing for a week?
- Which of them already have a ticket, and which never will because the company
  has no PSA ticketing?
- Which backups were set up but never activated, and which devices were never
  mapped to a ScalePad client?
- Which policies count warnings as success, hiding a backup that has not fully
  succeeded in a month?
- Which decommissioned devices are still generating no-result noise, and can we
  retire them without retiring the wrong backup by name?

[River Run](https://www.scalepad.com/partner-stories/river-run-securing-data-and-creating-revenue)
only acts on failures that persist past a day, because a job that fails once
usually self-corrects by the next run. That new-versus-ongoing judgment is exactly
what the portal leaves to the technician each morning, and it is the first thing
an AI agent can take over once it can see days-in-status and ticket counts in one
row.

The vendor sources above are self-reported. The questions in the list are the
ones the Backup Radar portal answers one screen at a time, which is the gap this
skill was printed for.

## What this skill does about it

- `backup-radar-cli check --include-at-risk --agent` - today's failing, silent and
  at-risk backups ranked (without `--include-at-risk`, only failing and no-result),
  each marked new or ongoing with days in status and open ticket count.
  The agent opens tickets for the ongoing ones and ignores the one-day blips.
- `backup-radar-cli unwatched --agent` - every backup nobody acts on, one row per
  reason: inactive, unmapped, ticketless, no-psa, masked. Reasons whose collection
  was never synced are listed as skipped, never reported as zero.
- `backup-radar-cli stale --days 45 --agent` - silent backups grouped by client,
  each with a `dry_run: true` retire body (pinned by ScalePad device id for mapped
  devices, by exact company, device and job name for unmapped ones; v2-only rows
  get a `retire_note` instead), ready to preview with `backups retire --stdin`
  before a human runs the real retire.
- `backup-radar-cli scorecard --days 90 --agent` - every client ranked by
  in-compliance rate, or one client's weekly trend and five worst devices for the
  QBR slide.
- `backup-radar-cli coverage diff --devices vendor-devices.json --agent` - the
  devices your backup vendor protects that Backup Radar never sees, and the
  reverse, matched exactly or by short host name with ambiguities listed.

## Status

Beta. Validated against the Backup Radar API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
