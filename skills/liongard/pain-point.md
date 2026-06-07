# liongard skill - the MSP pain it closes

## The pain

Liongard is one of the most data-rich tools in the MSP stack - and one of the
hardest to operationalize. The recurring theme in r/msp threads about Liongard
(search "Liongard" on r/msp - "is it worth it", "how do you actually get value
out of it") is the same: the platform collects an enormous amount of
configuration and change data, but the value is locked behind clicking into one
environment at a time. Owners describe paying for deep visibility they rarely
see, because there is no fast way to ask a question across every client at once.

Two pains show up over and over:

1. **Silent decay.** A launchpoint goes stale or an agent drops offline and just
   stops collecting. Nobody notices until a QBR, a security review, or an audit
   needs the data - and by then the "documentation" has quietly rotted. There is
   no single command that says "show me every stale collector and offline agent
   across the whole estate, right now."

2. **Reporting is manual.** Pulling one metric (MFA-enabled count, local-admin
   count, patch age) across every system, or listing every failed inspection
   estate-wide, means clicking environment by environment or writing a one-off
   API script - every single time you need it.

## What this skill does about it

It syncs your whole Liongard estate into a local mirror once, then answers the
cross-client questions from there:

- **`liongard-cli drift --since 7d`** - every change across every client in one
  feed, joined to the owning environment and system.
- **`liongard-cli health --agent`** - one estate-wide scorecard: stale
  launchpoints, offline agents, failed inspections, and coverage gaps, with a
  typed exit code for cron.
- **`liongard-cli launchpoints stale --older-than 7d`** and
  **`liongard-cli agents offline`** - catch the silent decay before a QBR does.
- **`liongard-cli metrics pivot "MFA Enabled Count" --csv`** - one metric across
  every system, report-ready, in a single command.
- **`liongard-cli inspectors coverage`** - which environments are still missing
  an inspector, biggest gap first.

## Status

Beta. Validated against the Liongard API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
