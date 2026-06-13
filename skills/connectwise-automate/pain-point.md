# connectwise-automate skill - the MSP pain it closes

## The pain

ConnectWise Automate (formerly LabTech) is built around a per-server console that
excels at drilling into a single endpoint and falls apart at "show me this across
every client." On MSPGeek - the long-running ConnectWise Automate community forum -
and on r/msp, the recurring complaints are the same shape: there's no fast cross-client
roll-up, offline and stale agents quietly inflate license counts and hide risk, and
pulling patch-compliance for a quarterly business review means assembling a report per
client by hand. The data is all in Automate. Getting a fleet-wide answer out of it is
the work.

For an owner-operator MSP, that work lands at the worst times: before a license
true-up, before a security review, the morning of a QBR. The questions are simple -
"who's offline?", "who's behind on patches?", "what needs a tech today?" - but the
console makes you answer them one server, one client, one endpoint at a time.

## What this skill does about it

It syncs your whole Automate fleet into a local SQLite mirror, then answers the
cross-client questions directly:

- **`stale-agents --days 30`** - every computer not seen in N days, grouped by client - the offline agents bleeding license and hiding risk, ready for a true-up.
- **`patch-compliance`** - per-client patch posture, worst offenders first, ready to paste into a QBR.
- **`alert-triage --min-priority 3`** - open alerts across every client, ranked and joined to computer, location, and client, so the morning starts with what actually needs a human.
- **`fleet-health`** - every agent across every client in one roll-up: online/offline, last-contact age, and open-alert count.
- **`os-inventory --eol-only`** - end-of-life operating systems across the fleet, flagged for the hardware-refresh and security conversation.

## Status

Beta. Validated against the ConnectWise API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
