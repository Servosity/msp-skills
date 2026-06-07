# afi skill - the MSP pain it closes

## The pain

SaaS backup has a quiet failure mode that MSP owners on r/msp raise again and
again: the backup you *think* is running isn't. A mailbox, site, or shared
drive gets created in Microsoft 365 or Google Workspace, the onboarding ticket
gets closed, and nobody ever confirms a backup policy was actually attached. Or
a policy *is* attached, but the nightly archive quietly stops landing - and
because the portal still shows the policy as "protected," the gap stays
invisible until the day a client asks for a restore and the data was never
there. The recurring r/msp refrain is blunt: *"how do you actually verify your
M365 backups are running across every client?"*

Afi gives you the data to answer that - but only one tenant at a time, through a
portal that forces a per-client walk, behind a public API the vendor explicitly
asks you not to poll. At MSP scale (dozens or hundreds of tenants) the
verification work that *should* be a thirty-second check becomes an afternoon of
tab-switching nobody does often enough.

## What this skill does about it

It syncs the whole Afi hierarchy into a local store once, then answers the
coverage questions offline - no portal walk, no rate-limit risk:

- **`afi-cli coverage-gaps --agent`** - every resource with no backup
  protection attached, across the fleet. The blind spots, named.
- **`afi-cli backup-stale --max-age 48h --agent`** - protected resources whose
  newest archive has gone stale: the silent-failure class no portal screen
  surfaces.
- **`afi-cli fleet-health --failed-only --agent`** - the Monday-morning "is the
  whole fleet green" sweep in one table instead of one tenant tab at a time.
- **`afi-cli reconcile-licenses --agent`** - seats purchased vs seats actually
  protected, so you stop paying for unused licenses and catch under-provisioning.
- **`afi-cli offboard <resource-id> --tenant <tenant-id> --reason "departure"`** -
  a guarded archive-then-release for departing employees that refuses to drop
  the protection until a fresh backup is verified.

## Status

Beta. Validated against the Afi API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
