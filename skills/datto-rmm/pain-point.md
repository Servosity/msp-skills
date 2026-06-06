# datto-rmm skill - the MSP pain it closes

## The pain

Datto RMM is built around a per-customer view. That's fine until you have to answer
a question that spans the whole fleet - and those are the questions that actually
matter to an MSP owner: *which endpoints stopped reporting?*, *where is antivirus
off?*, *who's behind on patches?*, *whose hardware warranty is about to lapse?*

On r/msp, the recurring Datto RMM complaints are exactly this shape: alert volume
that buries real failures ("monitor noise" / "alert fatigue"), and reporting that
makes you click through site by site because there's no clean cross-client rollup.
Owners describe pulling device lists into spreadsheets before a QBR and reconciling
warranty and patch data by hand. The console answers "how is *this* client?" well;
it does not answer "how are *all* my clients?" without a lot of manual clicking.

The result is that dead agents, unprotected endpoints, and expiring warranties are
discovered late - often when the customer calls - instead of in a 10-second sweep.

## What this skill does about it

The CLI syncs your entire multi-site Datto RMM fleet into a local SQLite mirror,
then answers the cross-client questions offline in one command:

- **`fleet stale`** - every device across all sites that stopped checking in, so you
  catch dead agents before the customer does.
- **`fleet av-gaps`** - every endpoint fleet-wide where antivirus is missing,
  disabled, or not running, so security gaps surface before an incident.
- **`fleet patch-gaps`** - devices ranked by missing-patch count across all sites,
  so you remediate the most-exposed endpoints first.
- **`fleet warranty`** - every device whose hardware warranty expires within a
  window, ready to drop into a QBR refresh plan.
- **`fleet scorecard`** - a one-page per-site health card (devices, alerts, patch and
  AV coverage, warranty exposure, agent drift) for the QBR conversation.

## Status

Beta. Validated against the Datto RMM API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
