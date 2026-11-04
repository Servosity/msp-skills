---
name: nizlo
description: >
  Query a Nizlo RMM workspace through its permission-limited personal access
  token API. Use for Nizlo fleet summaries, endpoint inventory, stale or
  offline-device checks, customer or site lookup, and active alert triage. This
  first-party skill is intentionally read-only: it never creates jobs, changes
  policies, acknowledges alerts, or mutates a tenant.
allowed-tools: "Read Bash PowerShell"
author: "Nizlo"
license: "Apache-2.0"
vendor: "Nizlo"
metadata:
  markdown_only: true
---

# Nizlo RMM read-only operations

Use the bundled standard-library helper to answer operational questions from a
Nizlo workspace without placing a bearer token in a command or chat transcript.
The helper supports only HTTP GET requests to a fixed allowlist of Nizlo API
routes.

## Set up access

1. Sign in to Nizlo and open **Admin > API access**.
2. Create a short-lived personal access token with only the permissions needed
   for the intended query. Fleet, customer, site, and dashboard queries require
   `devices.view`; alert queries additionally require `alerts.view`.
3. Store the one-time token in the `NIZLO_API_TOKEN` environment variable. Do
   not paste the token into a prompt, repository, shell history, or config file.
4. Leave `NIZLO_BASE_URL` unset for Nizlo Cloud. Set it only when an authorized
   Nizlo deployment uses a different API origin.

Personal tokens remain tenant-bound and accountable to the active technician
who created them. They cannot manage their own token lifecycle.

## Run queries

The helper is `scripts/nizlo_api.py` relative to this file. On Windows use
`python`; on macOS or Linux use `python3`.

```bash
python scripts/nizlo_api.py dashboard
python scripts/nizlo_api.py devices --status offline --page-size 100
python scripts/nizlo_api.py devices --attention alerts --sort lastSeen
python scripts/nizlo_api.py stale-endpoints --days 7
python scripts/nizlo_api.py customers --search "Acme"
python scripts/nizlo_api.py sites --customer-id CUSTOMER_UUID
python scripts/nizlo_api.py alerts --status active --severity critical
```

All successful commands print the API JSON response. Paginated commands accept
`--page` and `--page-size`. Filters are sent only when explicitly provided.

### Find endpoints not seen for seven days

Use `stale-endpoints --days 7` for a complete, read-only stale-agent audit. The
helper walks every active endpoint page, applies the UTC cutoff locally, and
groups the result by customer. Each endpoint row includes its ID, display name
or hostname, operating system, and exact `lastSeenAt` value. A null value is
reported because it means the endpoint has never been seen. Increase `--days`
to use a different stop gate; the default is seven.

## Safety boundary

- Every permitted route is a GET under `/api/v1`.
- The helper rejects unknown routes and does not expose a generic request mode.
- The Authorization header and token value are never printed.
- HTTP errors include the status and a bounded response excerpt after token
  redaction.
- A `401` means the credential is missing, expired, revoked, or otherwise
  invalid. A `403` means the token or current technician role lacks the needed
  permission. Do not broaden permissions until the exact workflow requires it.

## Self-check

Run the offline self-check before using a live credential:

```bash
python scripts/nizlo_api.py --selfcheck
```

The self-check validates URL construction, route allowlisting, secret
redaction, and stale-report grouping. It performs no network request and reads
no token.
