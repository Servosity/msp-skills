# connectwise-rmm skill - the MSP pain it closes

## The pain

ConnectWise RMM (the cloud RMM on the ConnectWise Asio platform, formerly Command
and Continuum) is built around a per-client, per-device view. That works until the
question spans the whole book of business - and those are the questions an MSP
owner actually asks: *which agents stopped checking in?*, *which clients are behind
on patches?*, *which machines still need a reboot this patch week?*

The partner API does not make those questions cheaper. ConnectWise's own API
documentation (ConnectWise University, "Agent Last Seen API" and "Rate Limits")
caps availability checks at 400 endpoints per call and 40 calls per 5 minutes,
and patch compliance comes back as per-endpoint rows keyed by IDs, with no
per-client roll-up. On r/msp the recurring RMM
complaint is the same shape across vendors: no clean cross-client roll-up, so
owners export device lists into spreadsheets before a client review and find dead
agents when the customer calls.

Scripts have the same problem in reverse. After a fleet script push every
endpoint's result carries its own raw error text, so working out that 30 machines
failed for the same "Access is denied" - and re-running just those - is manual.

## What this skill does about it

The CLI syncs companies, sites, endpoints and tickets into a local SQLite mirror,
batches the live availability checks, caches patch summaries, and answers in one
command:

- **`fleet stale --older-than 24h`** - every offline or silent agent across all
  clients, grouped by company, so dead agents surface before the customer calls.
- **`fleet patch-posture --min-compliance 90`** - clients ranked by patch risk
  (below the bar, missing patches, pending reboots, out of support, stale
  assessments), ready for the client review.
- **`fleet reboot-due`** - the patch-week reboot queue, split into reboot now,
  coordinate with a logged-on user, and offline.
- **`fleet whois "jsmith"`** - which machine a caller is logged on to, so the
  ticket starts on the right endpoint.
- **`fleet task-failures "<task-id>"`** - failed endpoints from a script run grouped
  by identical error, with IDs ready to pass back to `run --endpoint`.

## Status

Beta. Built against the ConnectWise RMM partner API (every route confirmed on the live
gateway without credentials; behavior tested against a mock gateway); the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.

