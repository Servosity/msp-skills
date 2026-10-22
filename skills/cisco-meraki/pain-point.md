# cisco-meraki skill - the MSP pain it closes

## The pain

An MSP running Meraki for twenty clients usually has twenty Dashboard organizations
under one login. Meraki's answer for that is the MSP Portal (now the Global Overview
page). Per Meraki's own documentation ("Monitoring and Managing Multiple
Organizations"), it lists each org with open support tickets, license status, one
co-terminated license expiration date, network count, and device counts with
online/alerting/offline indicators. What it documents is per-org counts: no cross-org
list of *which* devices are down, no cross-org change log, no admin management across
orgs. So the morning question "what is broken for every client right now?" still means clicking into
each red org, then into its networks.

Scripting around it runs into the API's own shape. Meraki's paginated list endpoints
return a bare JSON array and page only through an RFC 5988 `Link` header, ten items per
page by default on many of them, so a script that reads one response quietly stops at
page one. The Meraki Community Developers & APIs board has people working through
exactly that, in threads such as "Using Powershell how to paginate thru pages" and "Meraki API - Get
Organization Clients Search - Only return 3 networks". Add the ten-calls-per-second,
per-organization rate limit and a cross-org script becomes a project, not a query.

Renewals are worse: three licensing models (co-termination, per-device, subscription)
plus end-of-sale and end-of-support dates on hardware, so there is no single
date-sorted list of what is due across every client before the quotes go out.

## What this skill does about it

- `cisco-meraki-cli fleet down --agent` - every offline or alerting device and every
  failed MX uplink across all orgs, with org and network names and how long each has
  been down. The morning triage list in one call.
- `cisco-meraki-cli fleet renewals --within 90d --agent` - co-term, per-device and
  subscription expirations plus end-of-sale and end-of-support hardware, soonest first,
  for every client. The renewal pipeline without opening a license page.
- `cisco-meraki-cli fleet blame "Q2QN-9J8L-SLPD" --window 2h --agent` - each time a
  device or network went offline in the last week, the config changes made on that
  network just before, with the admin and old and new values. "Did a tech break it?"
  answered with evidence.
- `cisco-meraki-cli fleet admins --stale 90d --flagged --agent` - every Dashboard admin
  across orgs grouped by email, flagging two-factor off, API key holders and stale
  sign-ins; `--email` gives the offboarding view when a tech leaves.
- `cisco-meraki-cli fleet health --agent` - one worst-first row per org. An org whose
  API access is off or that fails to answer is listed as unreachable, never counted as
  zero devices.

Every fleet command follows every `Link`-header page and paces itself under the rate
limit, so a list is either complete or the org that failed is named.

## Status

Beta. Validated against the Meraki Dashboard API surface (the cross-org commands pass a
behavioral suite against a local fake of the Dashboard API, including multi-page
`Link` pagination, unreachable orgs and action-batch pushes); the closed-loop receipt
(a named MSP running it live in their production orgs at a Build Session) is tracked
separately and added here as `video.md` once it exists.

