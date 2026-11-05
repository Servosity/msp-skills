# exchange-online skill - the MSP pain it closes

## The pain

Microsoft began a phased shutdown of Exchange Web Services (EWS) in Exchange Online on
October 1, 2026, and retires it completely on April 1, 2027 (Microsoft 365 Message Center
MC1227454, "Exchange Web Services (EWS) retirement update"). Tenants that never set their
EWS options are switched off by default. Every MSP now has to answer, per client: which
mailboxes and apps still depend on EWS, IMAP, POP, ActiveSync or SMTP AUTH? The effective
answer per mailbox is the org default and the mailbox override together, which is exactly
what a Get-CASMailbox export does not show.

At the same time, business email compromise keeps leaving the same fingerprints: an inbox
rule that forwards, deletes or hides mail, or a forwarding address on the mailbox
(Practical365, "Good Use of Microsoft 365 Auditing Can Halt Business Email Compromise
Attempts"). Those rules live per mailbox. Checking one tenant means a PowerShell session,
Get-InboxRule for every mailbox, then the outbound spam policy and the remote domains; then
the same again for the next client. Offboarding, permission changes and quarantine release
repeat by hand across dozens of tenants, with no receipt that each step actually stuck.

## What this skill does about it

- `ews-posture <tenantId>`: every mailbox whose EWS, IMAP, POP, ActiveSync or SMTP AUTH is
  effectively on once org defaults apply, so you know who breaks before Microsoft flips it.
- `forwarding-audit <tenantId> --sweep`: every way mail can auto-forward out of the tenant,
  and whether the outbound spam policy or a remote domain blocks it.
- `bec triage <tenantId> <mailbox>`: one mailbox's compromise evidence in one call (rules,
  forwarding, restricted-sender status, new mobile devices, outbound volume, legacy protocols).
- `offboard <tenantId> <mailbox> --plan`: the Exchange side of a leaver, each step done or
  pending; run it without `--plan` to apply the pending steps with a per-step receipt.
- `quarantine queue --older-than 24h`: one oldest-first list of quarantine release requests
  across every tenant you snapshotted, instead of one admin portal tab per client.

## Status

Beta. Validated against the Microsoft Exchange Online API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
