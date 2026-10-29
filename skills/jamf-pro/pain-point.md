# jamf-pro skill - the MSP pain it closes

## The pain

Jamf Pro gives each customer its own server, so an MSP works many consoles. On Jamf
Nation's ["Multi-tenant Jamf Pro?" thread](https://community.jamf.com/general-discussions-2/multi-tenant-jamf-pro-4059)
one MSP engineer wrote "I work for an MSP with about 100 instances", because Apple
requires each organization to have its own push certificate. A later reply on the same
thread says of multi-tenancy: "This is something Jamf really needs to work on."

So every question that crosses clients means one login per client. A ticket names a Mac
but not the client, and someone searches console after console for the serial. ADE and
VPP tokens expire per client on their own dates, and a lapsed one shows up as an
onboarding that stopped working. A policy changed in your template drifts out of step,
and nothing says which client lags.

## What this skill does about it

- `jamf-pro-cli fleet find <serial>` - find which client has a Mac, iPad or iPhone, by
  serial, name, username or email, in one reply.
- `jamf-pro-cli fleet posture --stale-days 14 --min-os 14.6` - per-client counts of stale
  check-ins, Macs below a macOS floor, FileVault off and firewall off, with Macs inventory
  cannot judge reported as incomplete.
- `jamf-pro-cli fleet health --within 30d` - ADE and VPP token expiry, failed enrollment
  syncs and Jamf Pro version spread, before onboarding stops.
- `jamf-pro-cli fleet policy-hygiene` - policies that are disabled, can never run, have no
  scope, or point at a missing script or package.
- `jamf-pro-cli fleet baseline --golden msp-template` - which clients are missing your
  template's policies, profiles and smart groups by name, or run a different copy of its
  scripts.

Each instance is read with its own API client, in parallel, and nothing is written.

## Status

Beta. Validated against mocks built from Jamf's public API reference; no live tenant has
run it yet. The closed-loop receipt (a named MSP running it live in their production
tenant at a Build Session) is tracked separately and added here as `video.md` once it
exists.
