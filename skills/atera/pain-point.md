# atera skill - the MSP pain it closes

## The pain

Atera is praised for ease of use and per-technician pricing, but its reporting
is the consistent weak spot in user reviews. Across
[G2](https://www.g2.com/products/atera/reviews) and
[Capterra](https://www.capterra.com/p/144309/Atera/reviews/), MSPs report that
custom reports require workarounds, filtering is rigid, exports are clunky, and
the deeper cross-client analytics are gated behind higher-tier plans -
themes echoed in third-party alternative round-ups from
[Syncro](https://syncrosecure.com/blog/atera-alternatives/) and
[Action1](https://www.action1.com/blog/patch-management/atera-alternatives/).

The practical result: there is no single screen that answers the questions an
MSP owner actually asks across every client at once - which machines went dark,
which tickets are about to breach SLA, who is overloaded on the service desk,
which customers are under-contracted, what contracts expire next quarter. You
assemble those answers by hand, portal tab by portal tab, or you page thousands
of objects through the live API against its rate limit. So the numbers that
matter at a QBR are the ones nobody has time to pull.

## What this skill does about it

`atera-cli` syncs Atera into a local SQLite mirror, then answers the cross-entity
questions the portal leaves on the table - instant, offline, and rate-limit-free:

- **`atera-cli agents stale --days 30`** - the machines that quietly stopped
  reporting, before the client calls to complain. A time-window the live API
  never returns.
- **`atera-cli tickets sla`** - open tickets ranked by minutes-to-breach, soonest
  first, so the service desk works the right ticket next.
- **`atera-cli customers coverage`** - accounts you manage but don't bill:
  managed agents with no active contract. The margin leak no Atera screen shows.
- **`atera-cli contracts expiring --days 60`** - the renewal calendar, ranked by
  days-to-expiry, joined to the customer name.
- **`atera-cli agents patch-status`** - a fleet-wide missing-patch rollup built by
  fanning the per-device patch endpoint across the estate (the API has no single
  fleet patch view), paced under Atera's 700-requests-per-minute limit.

## Status

Beta. Validated against the Atera API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
