# rewst skill - the MSP pain it closes

## The pain

Rewst is the automation layer a lot of MSPs now run their service delivery on - and
its only surface is a GraphQL gateway and a web app. There is no first-party CLI, no
terminal access, and no cross-tenant operational view. On r/msp and in the Rewst
community (the "RewstHQ" Discord and forums), the recurring theme is operational
blindness at scale: an automation that quietly stopped firing, a workflow that's been
failing for a week in one client and nobody noticed, a pack that got rolled out to 18
of 20 tenants. The data exists, but answering "is automation healthy for this client?"
means opening the web app and reading execution history org by org.

For an MSP, Rewst automations ARE the service. A failed workflow is a ticket that
didn't get created, an onboarding that didn't run, a report that didn't go out. A
cross-tenant blind spot on automation health is a blind spot on the work itself.

## What this skill does about it

It turns Rewst's GraphQL schema into typed commands and adds the cross-org rollups the
gateway has no single endpoint for:

- **`health --org <id>`** - one-call execution health for a client: succeeded / failed / running counts plus time saved, with a clear unhealthy verdict when failures are present.
- **`failures --org <id> --since 12h`** - the recent failed runs, newest first - the triage queue, not every execution.
- **`dormant --org <id> --days 30`** - workflows that stopped running - automation that quietly went idle after a trigger or integration broke.
- **`roi --org <id> --since 30d`** - Rewst's humanSecondsSaved aggregated into hours/days saved and the top time-savers, for a QBR.
- **`drift --org <id> --against <id>`** - what one tenant has that another is missing (variables, packs) when an automation works in one place but not another.

## Status

Beta. Validated against the Rewst GraphQL API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
