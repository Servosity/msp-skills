# betterstack skill - the MSP pain it closes

## The pain

Monitoring sprawl and alert fatigue are a standing complaint in MSP communities
(r/msp and MSPGeek threads on "monitoring noise," "alert fatigue," and "who's
actually getting paged" recur constantly). The shape of it:

- **Silent monitors.** Somewhere across dozens of client accounts is a monitor
  with no escalation policy and no alert channel. It goes down at 2am and pages
  nobody. You find out when the client calls. Nobody can hand-check, monitor by
  monitor, which ones are actually wired to alert someone.
- **Alert fatigue from flapping.** A handful of noisy monitors wake the on-call
  tech night after night for nothing. After enough false 3am pages, the team
  starts ignoring alerts, and a real outage gets lost in the noise.
- **On-call gaps.** A rotation has nobody on call right now - a calendar hole
  during a holiday or a handoff - and the one night it matters, the page goes
  to no one.
- **Status-page drift.** Your public status page reads "all systems
  operational" while a backing monitor has an open incident. The client sees
  green, then notices the outage themselves, and your credibility takes the hit.

None of these are answerable in one click in the Better Stack portal, which
shows you one monitor, one incident, one status page at a time.

## What this skill does about it

- **`betterstack-cli coverage`** - finds every monitor that would page nobody
  if it failed (no escalation policy, no alert channel), so the silent ones
  surface before they bite.
- **`betterstack-cli flapping --days 7 --top 10`** - ranks the noisiest
  monitors over the last week so you can fix or mute the alert-fatigue sources.
- **`betterstack-cli oncall-gaps`** - flags any rotation with nobody currently
  on call.
- **`betterstack-cli statuspage-audit`** - flags status pages showing green
  while a backing monitor has an open incident.
- **`betterstack-cli mttr --days 30 --by-monitor`** - your real MTTA/MTTR for
  the QBR, computed from the local mirror, broken down by monitor.

## Status

Beta. Validated against the Better Stack API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
