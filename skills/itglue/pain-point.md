# itglue skill - the MSP pain it closes

## The pain

Ask any MSP where IT Glue hurts and you hear the same things: **nobody knows which
clients are actually documented**, **the credential-rotation audit is a manual
slog**, and **the API makes you ask one record at a time**.

IT Glue is the system of record for the whole client base - organizations,
contacts, passwords, configurations, documents - but it shows you one organization
at a time. There is no built-in scorecard for which clients are missing runbooks,
contacts, or documented configurations, so completeness gaps surface during an
incident or a QBR instead of before. Keeping IT Glue current is a recurring
discipline complaint on r/msp, and the portal gives owners no fleet-wide way to
measure it.

Credential hygiene is worse. SOC2 and cyber-insurance questionnaires ask when
privileged credentials were last rotated, but IT Glue has no single screen that
lists stale passwords across every client. Building that list by hand means
clicking through each org - and trying to do it live across the fleet runs straight
into IT Glue's documented **3000-requests / 5-minute** API rate ceiling.

Underneath, the REST API is a thin per-call JSON:API surface: recency is exposed one
resource at a time, there is no endpoint for "which contacts are duplicated" or
"which records point at an organization that no longer exists," and overlapping PSA
syncs routinely create the same person twice while offboarded clients leave orphaned
configurations and documents behind.

## What this skill does about it

It syncs IT Glue into a local SQLite mirror and answers the cross-client questions
as one offline query - no portal clicking, no per-question API round-trips:

- **`itglue-cli search "Fortinet"`** - find which client owns a device, contact, or
  serial number across every synced org at once, the answer a single ticket needs.
- **`itglue-cli coverage --below 1`** - rank clients by documentation completeness,
  surfacing the ones missing a whole category before a QBR or an incident does.
- **`itglue-cli passwords stale --days 365`** - every credential past its rotation
  threshold, grouped by client, metadata only - the SOC2 / cyber-insurance answer.
- **`itglue-cli contacts dupes`** - the duplicate contacts overlapping PSA syncs
  leave behind, found with a local self-join no IT Glue endpoint offers.
- **`itglue-cli orphans`** - configurations, contacts, passwords, and documents whose
  owning organization is gone, so dangling documentation stops misleading techs.

## Status

Beta. Validated against the IT Glue API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
