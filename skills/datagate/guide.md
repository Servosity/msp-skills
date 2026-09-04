# DataGate CLI - full command reference

This connector is **read-only** (list, get, search) - DataGate's full CRUD surface
is documented but this build doesn't implement create/update/delete.

## Global flags

Every command accepts these (run `datagate-cli --help` for the full, current list):

| Flag | What it does |
| --- | --- |
| `--json` | Output as JSON |
| `--agent` | Agent-friendly defaults: `--json --compact --no-input --no-color` |
| `--compact` | Only key fields (id, name, status, timestamps) |
| `--csv` / `--plain` | CSV or tab-separated output |
| `--select <fields>` | Comma-separated fields to include, e.g. `--select id,name,email` |
| `--data-source auto\|live\|local` | Read from the live API, the local sync mirror, or auto-fallback |
| `--dry-run` | Show the request without sending it |
| `--timeout <duration>` | Request timeout (default 1m) |
| `--rate-limit <float>` | Cap requests/second (default: auto-pace to DataGate's rate-limit headers) |

## Resource commands (list / get)

Thirteen resource groups share the same `list` / `get <id>` shape:

```bash
datagate-cli customers list --json
datagate-cli customers get <customer-id>

datagate-cli customer-users list
datagate-cli agreements list --json
datagate-cli agreements get <agreement-id>
datagate-cli service-items list
datagate-cli assignments list
datagate-cli rate-cards list
datagate-cli sites list
datagate-cli product-templates list
datagate-cli kit-templates list
datagate-cli products list
datagate-cli delivery-methods list
datagate-cli product-transactions list
datagate-cli account-managers list
```

Run `datagate-cli <resource> --help` or `datagate-cli <resource> get --help` for
the exact filters each one accepts.

## `invoices` (special - not a plain list/get)

DataGate's own API has no documented `GET /invoices`. The real endpoint is
`POST /invoices/search` with a JSON filter body, and this CLI models that
directly as a single `invoices` command with flags instead of a `list`/`get` pair:

```bash
datagate-cli invoices \
  --period-start 2026-08-01T00:00:00Z \
  --period-end 2026-08-31T23:59:59Z \
  --json
```

Flags:

| Flag | Notes |
| --- | --- |
| `--period-start` | Full ISO-8601 datetime with `Z` suffix, e.g. `2026-08-01T00:00:00Z`. A bare date with no time component silently matches nothing - always include the time and `Z`. |
| `--period-end` | Same format. |
| `--status` | Invoice status filter. |
| `--customer-id` | Filter to a single customer GUID. |
| `--page` | 1-indexed page number; page size is fixed server-side at 50 and is not adjustable. |

## `sync` - mirror data locally

```bash
datagate-cli sync                                    # sync everything
datagate-cli sync --resources agreements,invoices     # only these resources
datagate-cli sync --full                              # ignore the checkpoint, full resync
datagate-cli sync --since 7d                          # incremental, last 7 days
datagate-cli sync --concurrency 8                     # parallel workers
```

Naming a parent resource also syncs its dependents. A resource DataGate denies
access to (403, or a 400 with an access-policy body) is reported as a warning,
not a hard failure, unless you pass `--strict`.

## `search` - query the local mirror (or the live API, if it has search)

```bash
datagate-cli search "acme"                              # auto: API search if available, else local
datagate-cli search "acme" --data-source local          # force local FTS5 search
datagate-cli search "acme" --type customers --json
```

## `doctor` - health check

```bash
datagate-cli doctor                # human-readable
datagate-cli doctor --json         # machine-readable
datagate-cli doctor --fail-on warn # non-zero exit on credential/path warnings too
```

Reports whether `DATAGATE_API_KEY` and `DATAGATE_CLIENT_ID` are set and the API
is reachable - run this first after installing or when something looks wrong.

## `auth` - credential management

```bash
datagate-cli auth status
```

DataGate has no OAuth flow to log in to - `auth` here just reports and manages
the environment-variable credentials described in [README.md](./README.md).

## Rate limits

DataGate enforces 60 requests/minute and 5,000/day, per account (shared across
everyone using the same Bearer token/ClientId pair). `--rate-limit` auto-paces to
the server's own rate-limit headers by default; the local mirror (`sync` +
`search`) exists specifically to avoid re-spending this budget on repeated
lookups.
