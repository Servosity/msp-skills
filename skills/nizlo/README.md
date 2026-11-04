# Nizlo RMM + AI for MSP and IT operations

> Vendor-contributed Nizlo integration, maintained by Nizlo. This contribution is
> hosted in the Servosity `msp-skills` community repository; Servosity is not
> affiliated with, endorsed by, or sponsored by Nizlo.

Use an AI agent to inspect a Nizlo workspace without giving it a mutation
surface. This skill ships a small standard-library Python helper for read-only
dashboard, customer, site, endpoint, and alert queries through Nizlo's
permission-limited personal access token API.

This Claude Code Skill is also readable by other agent-skill runtimes. It ships
no MCP server; the narrow local helper keeps the initial authority surface easy
to inspect.

## What it can answer

- How many endpoints are online, offline, servers, or awaiting reboot?
- Which active endpoints are offline or need attention?
- Which customers and sites match an operational search?
- Which critical or warning alerts are still active?
- What does the current fleet summary show before a maintenance window?

## Deliberately read-only

The helper permits only five GET routes. It cannot run scripts, create jobs,
change policies, acknowledge alerts, edit customers, or modify any other tenant
state. This keeps the first contribution useful for triage while making its
authority easy to review.

## Quick start

1. In Nizlo, open **Admin > API access** and create a short-lived personal token
   with the minimum permissions required.
2. Put the one-time value in `NIZLO_API_TOKEN` locally. Never paste it into chat
   or commit it.
3. Run the offline check, then a read-only query:

```bash
python skills/nizlo/scripts/nizlo_api.py --selfcheck
python skills/nizlo/scripts/nizlo_api.py dashboard
python skills/nizlo/scripts/nizlo_api.py devices --status offline --page-size 100
python skills/nizlo/scripts/nizlo_api.py alerts --status active --severity critical
```

Nizlo Cloud uses `https://app.nizlo.com/api/v1` by default. An authorized
self-hosted or alternate deployment can set `NIZLO_BASE_URL` to its API root.

## Authentication model

Personal access tokens are tenant-bound, time-limited, revocable, and scoped to
an explicit permission set that is intersected with the technician's current
role. The secret is shown once. Browser sessions, not bearer tokens, manage the
token lifecycle.

## Verification status

The helper includes an offline self-check for routing and redaction. This
contribution has been checked against the vendor-owned API contract and sandbox;
it is not presented as independent MSP live verification.

## License

Apache-2.0.
