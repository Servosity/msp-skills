# bitdefender skill - the MSP pain it closes

## The pain

GravityZone is company-by-company underneath. The
[Public API](https://www.bitdefender.com/business/support/en/77209-125277-public-api.html)
is JSON-RPC, one method per call, and `getCompaniesList` answers one level of the
company tree at a time, so every MSP script has to walk sub-partners and company
folders itself, then page `getEndpointsList` for each company. A
[Bitdefender Expert Community thread](https://community.bitdefender.com/en/discussion/92970/powershell-api-get-list-of-computers)
("Powershell API - Get List of Computers") shows exactly that: a hand-built
PowerShell loop over `page` and `pagesCount` with a `parentID` found by descending
the hierarchy, which still returned the wrong group.

The questions an MSP asks every morning live on different Control Center pages:
endpoint status under Network, open incidents under Incidents, quarantined files
under Quarantine, seats in the monthly usage reports. Asking any of them across
every customer means filtering company by company or stitching exports. And at
month end, GravityZone rolls each child company's usage up into its parent
partner, so matching billed seats per customer to PSA agreements means working
out each partner's own share by hand.

## What this skill does about it

- `bitdefender-cli fleet health` - every child company ranked by unmanaged,
  outdated, unscanned, policy-not-applied and isolated endpoints, saved as a dated
  snapshot.
- `bitdefender-cli fleet changes --since 7d` - what moved between two snapshots:
  new and removed machines, agents that went unmanaged or out of date, isolation on
  or off.
- `bitdefender-cli fleet incident-queue --sla 24h` - one open EDR/XDR incident queue
  across every customer, worst and oldest first, with unassigned incidents flagged.
- `bitdefender-cli fleet threats --window 7d` - the same quarantined file at two or
  more customers, and whether each affected company already blocklists it.
- `bitdefender-cli usage billing --month 09/2026 --agreements psa-agreements.csv` -
  per-company usage with each partner's own share after the rollup, checked against
  your PSA agreement quantities.

## Status

Beta. Validated against the Bitdefender GravityZone API surface; the closed-loop
receipt (a named MSP running it live in their production tenant at a Build
Session) is tracked separately and added here as `video.md` once it exists.
