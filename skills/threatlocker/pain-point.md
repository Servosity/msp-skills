# threatlocker skill - the MSP pain it closes

## The pain

ThreatLocker is default-deny application allowlisting: nothing runs unless it's
explicitly permitted, so every new or updated application a user touches becomes an
**approval request** an admin has to review. ThreatLocker's own MSP guide admits the
volume up front: *"While you are onboarding your first few clients, your devices are
learning and can be quite noisy"*
([ThreatLocker, "Allowlisting for MSPs"](https://www.threatlocker.com/blog/allowlisting-for-msps)).
Run that across a book of customer tenants and the queue never really goes quiet, and
the Portal makes you clear it **one tenant at a time** because the API and UI are scoped
to a single managed organization. The same blocked Chrome update can sit in twenty
different tenants' queues, each needing its own login and its own click.

The second cliff is evidence. ThreatLocker's Help Center is explicit: *"By default, the
Unified Audit retains data for 31 days. After 31 days, the information is permanently
deleted and cannot be recovered"*
([ThreatLocker, "Log Retention"](https://threatlocker.kb.help/log-retention/)).
Cyber-insurance questionnaires and compliance audits routinely ask for far longer than a
month, so the security evidence an MSP needs is exactly the evidence that just aged off,
unless someone remembered to export each tenant's log before the window closed.

## What this skill does about it

- **`approvals triage --all-tenants`** - one ranked queue of every pending approval across
  all your tenants, grouped by file hash so the same blocked file collapses into one row
  instead of twenty.
- **`approvals approve-batch --hash <sha256> --all-tenants --dry-run`** - permit a
  known-good file everywhere it's pending in a single command, with a preview plan first.
- **`audit retention-check`** - per tenant, how close your audit archive is to the 31-day
  cliff and how stale your last sync is, so evidence never ages off unnoticed.
- **`audit export --all-tenants --since 30d`** - persist every tenant's Unified Audit log
  to JSONL or CSV locally, keeping compliance evidence past ThreatLocker's retention window.
- **`devices health --all-tenants`** - classify every endpoint online / offline / stale /
  isolated across the whole book, so dark agents surface without a tenant-by-tenant sweep.

## Status

Beta. Validated against the ThreatLocker API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
