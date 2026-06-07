# tactical-rmm skill - the MSP pain it closes

## The pain

Tactical RMM is the budget-friendly, self-hosted RMM that MSPs on r/msp and in the
MSPGeek community reach for when they want to own their data and stop paying per
endpoint. It is genuinely capable - agents, checks, automation, scripts, patch
management - and the trade-off owners describe again and again is the same: the
reporting and cross-fleet views are thin. The web UI is built around drilling into
one agent or one client at a time. There is no built-in dashboard that answers, in
one look, "which machines went dark this week," "where are patches and reboots
pending across every client," or "what changed overnight."

Everything you'd want is in the REST API, but turning that into a Monday stand-up
rollup or a QBR number means writing and maintaining scripts against it yourself.
Self-hosting also means there is no analytics tier or BI add-on to fall back on -
the data is right there on your own server, but the fleet-wide views are not. So the
questions that actually drive the week get assembled by hand, portal tab by portal
tab, or not at all.

(Sources: recurring Tactical RMM reporting and "fleet overview" threads on r/msp and
the MSPGeek community; the project's own GitHub Discussions where the API is the
documented path for any cross-client view.)

## What this skill does about it

It mirrors your Tactical RMM instance into a local SQLite store, then answers the
cross-fleet questions as one offline join - instant, and the AI sees the answer, not
the raw dump:

- **`fleet health`** - whole-fleet posture in one command: online/offline/overdue
  agents, failing checks, pending reboots, outstanding patches, active alerts.
- **`triage --limit 20`** - the agents that need attention first, ranked across
  offline state, failing checks, reboots, and patches.
- **`patch posture --by client`** - pending Windows updates and the reboots they'll
  need, rolled up per client or site, before patch night.
- **`agents stale --days 7`** - endpoints that quietly stopped checking in, with
  client and site, before they become a gap.
- **`since "2h"`** - what moved across the fleet since you last looked: new alerts,
  newly-offline agents - so a shift starts with only what changed.

## Status

Beta. Validated against the Tactical RMM API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
