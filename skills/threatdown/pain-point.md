# threatdown skill - the MSP pain it closes

## The pain

ThreatDown OneView is the multi-site console MSPs use to run ThreatDown (Malwarebytes)
endpoint protection and EDR across their customers. It is built one site at a time.
The question every MSP asks on Monday morning - "which machines went dark since Friday,
across all my customers?" - has no single page.

ThreatDown's own answer is a script. The vendor support article **"API Example: Get
endpoints not seen for more than 24 hours"** (support.threatdown.com, article
35082574965011) ships a PowerShell example that lists every site, collects each site's
account ID, pages the endpoint search, and prints site name, endpoint name and hours
since last seen. The OneView dashboard's own **"Last sync 7+ days ago"** widget
(ThreatDown article 4413799480467, "Endpoints widget in OneView") only catches machines
a full week after they stop checking in. Community scripts such as
[nebula2nebula](https://github.com/B10sp4rt4n/nebula2nebula) carry their own
`list_endpoints_cli.py` for the same reason: the API answers per account, and someone
has to fan it out.

The same shape repeats everywhere an MSP needs a cross-customer answer:

- **Which customer needs me first?** Infected, suspicious, isolated, unprotected,
  reboot-pending and never-scanned machines are separate filters on separate pages.
- **Are my seats right before billing?** Allocations live on each site's subscription;
  deployed and recently seen machines live on the Endpoints page.
- **Did that scan or isolation actually run?** A job sent to an offline machine waits or
  quietly expires, and OneView reports job status as a bitmask nobody decodes by hand.
- **Isolating a machine** means looking up its site and endpoint ID and building a jobs
  request, under pressure, during an incident.

## What this skill does about it

- `threatdown-cli fleet stale --hours 24` - the vendor's PowerShell example as one
  command across every site, with exact hours since each machine was last seen.
- `threatdown-cli fleet health` - one ranked row per customer site, worst first, so you
  know where to start.
- `threatdown-cli seats reconcile` - seat allocations against deployed and active
  machines per site, with expiring subscriptions flagged, before the invoice goes out.
- `threatdown-cli respond pending --since 7d` - the scans, isolations and RMM-issued
  jobs that timed out, expired, failed or are still waiting.
- `threatdown-cli respond isolate --host LAPTOP-042` - finds the machine across every
  site, shows what would happen, and sends nothing until you add `--confirm`.

## Status

Beta. Built from ThreatDown's published OneView API specification and tested against a
local mock of that API, not yet a live OneView tenant. The closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.

