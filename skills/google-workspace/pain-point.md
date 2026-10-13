# google-workspace skill - the MSP pain it closes

## The pain

MSPs that support Google Workspace clients have no cross-client admin view. On the
r/msp thread ["Google Workspace Management"](https://www.reddit.com/r/msp/comments/1tu2nas/google_workspace_management/)
(June 2026) the original poster asked other MSPs which multi-tenant platform they use,
adding "I'd love a CIPP equivalent for the GW side." The replies are the state of the
art:

- An MSP managing "about 10 workspace tenants out of roughly 250 clients" called it
  "a manual process for the most part."
- The most common workaround was a server running multi-tenant GAM, where "you just
  GAM select company to switch companies."
- Another pointed out that Microsoft has GDAP and partner relationships to lean on,
  while Workspace leaves you with API access, per-client delegation, and admin rate
  limits.

So the questions an MSP owner asks across the whole book of business - which clients
still have admins without 2-Step Verification, where paid licenses sit on dead
accounts, which leavers were suspended but never cleaned up - turn into one login,
one export, and one spreadsheet join per client. The leaver question costs real money:
Google bills a suspended account the same as an active one (Google Workspace Admin
Help, "Preserve data for users who leave your organization"), so a seat stays on the
invoice until someone moves it to an Archived User license or releases it.

Offboarding has a second trap: order. A leaver's backup verification codes can only be
invalidated while the account is active (GAM's own deprovision documentation says "You
must unsuspend to process backupcodes"), so a tech who suspends first has to undo it to
finish the job.

## What this skill does about it

Register each client once as a tenant (a service account with domain-wide delegation,
least-privilege scopes), then:

- **`google-workspace-cli mfa-gaps --all-tenants`** - every account without 2SV across
  every client, ranked with unenrolled admins first.
- **`google-workspace-cli stale-licenses --all-tenants`** - Workspace edition licenses on
  suspended, never-signed-in, or idle accounts, per client, ready to reclaim
  (`--products` adds other license products).
- **`google-workspace-cli leavers --all-tenants`** - suspended accounts still holding a
  license, groups, an admin role, OAuth tokens, or an unfinished Drive transfer.
- **`google-workspace-cli offboard "jane@example.com" --successor "manager@example.com"`**
  - the offboarding runbook (revoke access, remove groups, transfer Drive and Calendar,
  suspend last so backup-code cleanup still works), previewed with live reads and
  changing nothing until you add `--yes`.
- **`google-workspace-cli posture --all-tenants`** - one row per client for the QBR:
  2SV coverage, admins without 2SV, stale licenses, and Drive and Calendar setting changes.

## Status

Beta. Validated against the Google Workspace Admin API surface; the closed-loop receipt
(a named MSP running it live against their own client tenants at a Build Session) is
tracked separately and added here as `video.md` once it exists.
