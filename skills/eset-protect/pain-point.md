# eset-protect skill - the MSP pain it closes

## The pain

An MSP running ESET PROTECT for dozens of clients gets a [Managed Customers](https://help.eset.com/protect_cloud/en-US/managed_customers.html)
page with per-customer counts of devices and unresolved detections. The
questions an owner asks next are not on that page: which machines are muted or
stopped checking in, worst customer first; which customers carry the CVE that
was announced this morning; what the techs should work first across every
client; what goes in this quarter's QBR. Those still come from filtering the
console one view at a time and copying numbers out by hand.

An AI agent cannot answer them from the API alone either. ESET Connect returns
devices, detections and incidents by device group, and no field names the
customer: the customer is the nearest ancestor group linked to a company, which
only the whole group tree can tell you.

Automation does not fill the gap on its own. The ESET Connect API mints
one-hour tokens through an OAuth2 password grant, splits detections across v1
(ESET PROTECT and ESET Inspect) and v2 (ESET Cloud Office Security), keeps
incidents in another service, and answers slow requests with 202 and a
response id that has to be polled. Integrations trip over it: Elastic's ESET
PROTECT integration had to be fixed for the password grant
([elastic/integrations#15335](https://github.com/elastic/integrations/issues/15335)),
and its device vulnerability stream could stay stuck after a transient ESET
Connect outage ([elastic/integrations#20977](https://github.com/elastic/integrations/issues/20977)).

## What this skill does about it

- `eset-protect-cli fleet health` - one row per customer: devices that need
  attention, problem counts, muted, stale and unenrolled devices, worst
  customer first.
- `eset-protect-cli triage --since 24h --min-severity medium` - one ranked
  morning queue of endpoint detections and open incidents, with the device,
  the customer and a suggested next command on each row.
- `eset-protect-cli exposure --cve CVE-2024-3400` - which customers and devices
  carry a CVE, the highest risk score, and whether a patch is available.
- `eset-protect-cli report customer "Acme Dental" --since 90d` - the QBR
  one-pager for one customer: device health, detections, incidents with triage
  and response times, top CVEs, pending and failed security patches.
- `eset-protect-cli respond isolate "FRONTDESK-01"` - prints the exact network
  isolation task for a device named the way your techs name it, and sends it
  only with `--yes`.

The per-customer views read a local SQLite mirror filled by
`eset-protect-cli sync`, which also handles the token, the 202 polling and the
pagination.

## Status

Beta. Validated against the ESET API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
