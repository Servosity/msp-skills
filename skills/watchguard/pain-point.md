# watchguard skill - the MSP pain it closes

## The pain

An MSP running WatchGuard for 40 customers has one Monday question: which customer
needs me first? The answer is spread across products and accounts. A Firebox can be
offline, a laptop's Endpoint Security can be out of date or waiting on a reboot, a
ThreatSync incident can sit untouched, and a license can be about to lapse, each in
a different product view of a different customer account.

The API does not collapse that by itself. WatchGuard's API documentation
(**Authentication**) says a Service Provider reaches a managed account by looking up
that account's **audience** value and requesting an access token with it; the token
lasts an hour. Firebox Management, Endpoint Security, ThreatSync, NDR, AuthPoint and
license allocation are separate APIs with their own paths, paging and integer status
codes. So "every customer, every product" is a token per customer and a call per
product per customer before anyone sees an answer.

WatchGuard knows the one-account-at-a-time shape hurts. Its September 2022 blog post
on Service Provider-level firmware upgrades sold the feature on MSPs "no longer need
to go down the line on each subscriber account, one at a time". License inventory has
the same drift: a
[WatchGuard Community thread](https://community.watchguard.com/watchguard-community/discussion/3267/removing-retired-and-unallocated-firebox-from-watchguard-cloud)
opened in May 2023 and still asked about in September 2024 shows retired, unallocated
Fireboxes staying in the inventory and inflating the device count, with a support case
as the only way out. And every Firebox deployment is asynchronous: the API returns a
transaction ID, and you poll it to learn whether the change landed or failed.

## What this skill does about it

- `watchguard-cli accounts health --agent` - one ranked row per customer: offline
  Fireboxes, at-risk endpoints, open incidents by severity and expiring licenses.
- `watchguard-cli endpoint-devices posture --agent` - every unprotected, outdated,
  isolated, unlicensed or reboot-pending computer across every customer, with the
  status codes decoded.
- `watchguard-cli threatsync-incidents queue --status new --agent` - the incidents
  nobody has touched, ranked by severity, threat score and age.
- `watchguard-cli allocations reconcile --agent` - allocated serials with no reporting
  Firebox, reporting Fireboxes with no allocation, and what expires soon.
- `watchguard-cli firebox-transactions wait --device FB-12345` - exit 0 when the
  deployment completes, 8 when it fails, 9 when it is still running at `--max-wait`.

## Status

Beta. Built from WatchGuard's published OpenAPI specifications and tested against a
local mock of those APIs; no real WatchGuard Cloud account has run it yet. The
closed-loop receipt (a named MSP running it live in their production account at a
Build Session) is tracked separately and added here as `video.md` once it exists.
