# sophos-central skill - the MSP pain it closes

## The pain

Sophos Central is tenant-by-tenant underneath. Sophos's own
[partner getting-started guide](https://developer.sophos.com/getting-started/)
spells out the loop every MSP script has to run: call `whoami` for the partner
ID, page through `/partner/v1/tenants`, record each tenant's `apiHost`, then call
that regional host with an `X-Tenant-ID` header, one customer at a time. The
Partner Dashboard rolls up alerts, but the questions behind them (which machines
are unhealthy, whose tamper protection is off, whose Account Health Check score
slipped, who is over their licence) still mean drilling into one customer at a
time.

Community tooling usually stops at one tenant. The
[SophosCentral-Powershell](https://github.com/jd1100/SophosCentral-Powershell)
module says plainly "There is currently no support for multiple tenants", with
multi-tenant support on its to-do list, and Sophos's own
[tamper protection toolkit](https://community.sophos.com/sophos-endpoint/f/recommended-reads/143257/sophos-central-endpoint-sophos-central-api-tamper-protection-toolkit)
is written for a single tenant. So each MSP rewrites the same tenant loop, region
routing and key-based paging, and month-end billing reconciliation stays a
spreadsheet job.

## What this skill does about it

- `sophos-central-cli fleet refresh` - every customer tenant pulled into one local
  store in one run, with a per-tenant ok/forbidden/error ledger, so a missing
  customer is visible instead of silently absent.
- `sophos-central-cli fleet triage` - one ranked morning work queue across every
  customer: high and medium alerts, bad or suspicious machines, tamper
  protection off, isolated or stale devices.
- `sophos-central-cli fleet tamper` - every machine with tamper protection off,
  across customers, previewed first and switched back on only with `--yes`.
- `sophos-central-cli contain "WS-FIN-07" --scan --yes` - isolate a machine by hostname
  or IP mid-incident without first working out which tenant it lives in.
- `sophos-central-cli fleet revenue` - each customer's monthly Sophos billing next
  to the endpoints actually deployed, for the month-end reconciliation.

## Status

Beta. Validated against the Sophos Central API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
