# runzero skill - the MSP pain it closes

## The pain

"You can't secure what you can't see" is one of the most repeated lines on r/msp,
and it shows up every time someone asks how to get a real asset inventory: devices
nobody remembers standing up, contractor laptops, a forgotten lab VLAN, the
printer running a CVE-ridden embedded web server. runZero is built exactly for
this - it discovers the unmanaged and unknown assets the RMM never enrolled.

But discovery is only half the job. The questions that actually drive remediation
are never about a single asset; they are cross-entity:

- *Which* of my critical assets run *this* vulnerable service?
- *What* newly opened a port or became vulnerable since last week?
- *Which* machines does this morning's CVE actually land on?
- *Which* assets are stale, end-of-life, or have no owner?

runZero's HTTP API is organized by scope (`/org`, `/account`, `/export`), so a
single live call cannot join assets to their services, software, and
vulnerabilities. The answer ends up in a spreadsheet: export the assets, export
the services, export the vulnerabilities, and join them by hand - every time the
question comes up. And because the console only shows the surface *now*, drift
between two points in time is invisible until something breaks.

## What this skill does about it

It syncs the whole attack surface into a local SQLite copy once, then answers the
cross-entity question directly and offline:

- **`runzero-cli triage --agent`** - rank assets by criticality joined to their
  exposed services and high-severity vulnerabilities, in one pass the live API
  cannot return.
- **`runzero-cli affected "CVE-2024-3094" --agent`** - pivot from a CVE to every
  affected asset, with its services and criticality, in one command.
- **`runzero-cli diff --since 7d`** and **`runzero-cli exposure-delta --agent`** -
  show what changed and what newly became exposed between two syncs.
- **`runzero-cli certs-expiring --days 90 --weak`** - surface TLS certs expiring
  soon or using weak crypto, joined to the asset presenting them.
- **`runzero-cli stale --days 90 --json`** - bucket unseen, EOL, and unowned
  assets and emit IDs ready to pipe into the bulk cleanup commands.

Each of these is one local join over data you already synced, so re-asking costs
zero additional runZero API quota.

## Status

Beta. Validated against the runZero API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
