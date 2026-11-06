# comet-backup skill - the MSP pain it closes

## The pain

MSPs who run Comet Backup (self-hosted or Comet-Hosted) keep every customer on
one Comet Server, one user account per tenant. The console and the emailed job
reports still work one tenant and one job at a time. "What failed last night
across every customer?" means clicking through user accounts or reading a pile
of report emails every morning, and "why did it fail?" means opening each job
log by hand.

The worse problem is the backup that never reports a failure because it never
ran: a laptop that has been offline for weeks, a schedule that stopped firing.
Magnus Box, a Comet-based backup provider, describes an MSP that lost a major
client because an Exchange backup had been failing silently for two months
([Magnus Box](https://www.magnusbox.com/post/are-you-making-these-common-msp-backup-mistakes-86-of-msps-miss-these-critical-steps)).
And because the console shows today's state only, "what changed since
yesterday?" and "which tenants are creeping toward quota?" have no history
behind them.

## What this skill does about it

- `comet-backup-cli health --state failed,overdue,never --agent` - every
  protected item across every tenant that failed, is overdue against its own
  schedule, or never backed up: the morning review in one list.
- `comet-backup-cli triage --window 24h --group-by error --agent` - the failed
  and stuck jobs with the first error lines from each job log, bucketed by
  cause so one bad Storage Vault stands out.
- `comet-backup-cli devices stale --older-than 7d --billable-only --agent` -
  offline devices that stopped backing up but still own protected items.
- `comet-backup-cli usage --over 80 --agent` - tenants at or above 80 percent
  of a storage, device or Microsoft 365 quota, before invoicing or a QBR.
- `comet-backup-cli snapshot` then `comet-backup-cli changes --only health --agent` -
  problems opened and resolved since the previous run, for a PSA ticket feed.

## Status

Beta. Validated against the Comet Backup API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
