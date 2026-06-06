# MSPbots CLI

**The first MSPbots tool we could find anywhere (June 2026)  -  readable filters, alias-named resources, full exports, and the KPI history MSPbots itself doesn't keep.**

MSPbots' Public API shares your BI datasets and widgets but ships with 19-digit IDs, a comma-encoded filter DSL, manual pagination, and no history. This CLI turns a shared API key into a usable data faucet: register aliases once, filter with readable predicates, export whole tables in one command, and snapshot KPIs into local SQLite for diffs and trends no MSPbots surface can show.

For the short install path see [README.md](./README.md). This file is the command reference.

## Authentication

MSPbots uses a raw API key sent as an `apikey` request header. An MSPbots admin creates the key in the MSPbots app under Settings → Public API (Add New Key, type Custom), flips the global Enable Public API toggle ON, and explicitly binds each dataset and widget the key may read. Set the key in your environment as `MSPBOTS_API_KEY`. Every endpoint is read-only; a key can never write anything. If a pull returns "resource unbound", the dataset or widget has not been bound to your key  -  that binding happens in the MSPbots UI, not in this CLI.

## Quick Start

```bash
# Verify the binary, config, and connectivity checks before touching the API
mspbots-cli doctor --dry-run

# Name a dataset your admin bound to the key  -  aliases replace 19-digit IDs everywhere
mspbots-cli registry add open_tickets 1534956341424005122 --type dataset

# Fetch the first page of rows as clean JSON
mspbots-cli pull open_tickets --page-size 10 --json

# Store a timestamped local copy  -  this builds the history MSPbots doesn't keep
mspbots-cli snapshot open_tickets

# See week-over-week movement once two or more snapshots exist
mspbots-cli trend open_tickets --column "Open Count"

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`registry`**  -  Name your bound datasets and widgets once, then use readable aliases everywhere instead of 19-digit resource IDs.

  _Agents and scripts can address resources by stable, human-readable names instead of copy-pasted snowflake IDs._

  ```bash
  mspbots-cli registry add open_tickets 1534956341424005122 --type dataset
  ```
- **`snapshot`**  -  Capture point-in-time copies of any dataset or widget into local SQLite  -  the history MSPbots doesn't keep.

  _Run it on a schedule and every later question about "how has this changed" becomes answerable offline._

  ```bash
  mspbots-cli snapshot open_tickets
  ```
- **`trend`**  -  Time-series and point-over-point deltas for any numeric column across stored snapshots.

  _Answers "is this KPI up or down since last week"  -  the question the live API structurally cannot answer._

  ```bash
  mspbots-cli trend open_tickets --column "Open Count"
  ```
- **`diff`**  -  Row-level added/removed/changed comparison between two stored snapshots of the same resource.

  _Shows exactly which tickets entered or left a queue between two captures, not just the count._

  ```bash
  mspbots-cli diff open_tickets
  ```

### Agent-native plumbing
- **`pull`**  -  Write filters like "Update Date >= 2026-05-29" and the CLI compiles them into MSPbots' comma-encoded operator DSL.

  _Reach for this instead of hand-building query strings; it handles operator encoding, spaces in column names, and URL-escaping._

  ```bash
  mspbots-cli pull open_tickets --where "Update Date >= 2026-05-29" --where "Status = Open" --json
  ```
- **`export`**  -  Dump an entire dataset or widget to CSV or JSONL, walking every page automatically.

  _One command replaces a babysat pagination loop when feeding spreadsheets or downstream pipelines._

  ```bash
  mspbots-cli export open_tickets --format csv
  ```
- **`describe`**  -  Sample live rows and infer the column names and types of a dataset or widget.

  _Run it before building --where filters so column names and types are known instead of guessed._

  ```bash
  mspbots-cli describe open_tickets
  ```

## Recipes


### Register and pull a shared dataset

```bash
mspbots-cli registry add sla_queue 1534956341424005122 --type dataset
```

One-time alias setup; every later command addresses the resource as sla_queue.

### Filtered pull with readable predicates

```bash
mspbots-cli pull sla_queue --where "Update Date >= 2026-05-01" --where "Status = Open" --json
```

The CLI compiles readable operators into MSPbots' comma-encoded query DSL and URL-encodes spaced column names.

### Agent-shaped KPI read

```bash
mspbots-cli pull sla_queue --agent --select row_count,rows
```

Returns only the row count and rows fields in agent-envelope JSON  -  bounded context for LLM consumption.

### Full CSV export for finance

```bash
mspbots-cli export sla_queue --format csv
```

Walks every page via current/size automatically and streams one clean CSV.

### Week-over-week KPI movement

```bash
mspbots-cli trend sla_queue --column "Open Count"
```

Aggregates the column across stored snapshots; pair with a scheduled `snapshot` to keep the series growing.

## Usage

Run `mspbots-cli --help` for the full command reference and flag list.

## Commands

### dataset

Fetch rows of datasets bound to your Public API key

- **`mspbots-cli dataset <resourceId>`** - Fetch one page of rows from a dataset bound to your API key

### widget

Fetch data of widgets bound to your Public API key

- **`mspbots-cli widget <resourceId>`** - Fetch one page of data from a bound widget (widgets with measure or calculate layers are not supported by the Public API)


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
mspbots-cli dataset 1534956341424005122

# JSON for scripting and agents
mspbots-cli dataset 1534956341424005122 --json

# Filter to specific fields
mspbots-cli dataset 1534956341424005122 --json --select id,name,status

# Dry run  -  show the request without sending
mspbots-cli dataset 1534956341424005122 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
mspbots-cli dataset 1534956341424005122 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - `snapshot` captures rows into local SQLite; `trend` and `diff` answer history questions offline
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
mspbots-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/mspbots-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `MSPBOTS_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `mspbots-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `mspbots-cli doctor` to check credentials
- Verify the environment variable is set: `echo $MSPBOTS_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `mspbots-cli registry list` to see registered aliases, or `mspbots-cli describe <alias>` to inspect a resource

### API-specific
- **HTTP 401 {"message":"Invalid API key in request"}**  -  Check MSPBOTS_API_KEY; keys are created at Settings → Public API in the MSPbots app, and the global Enable Public API toggle must be ON.
- **Resource unbound error on a valid key**  -  The dataset/widget is not bound to your key  -  an MSPbots admin must add it under Settings → Public API → Datasets/Widget → Add.
- **HTTP 502 when pulling a widget**  -  Known intermittent gateway issue on heavy widgets  -  retry with a smaller --page-size; note widgets with measure or calculate layers are not supported by the Public API at all.
- **Filtered pull returns zero rows unexpectedly**  -  Column names are exact (including spaces and casing)  -  run `mspbots-cli describe <alias>` to see the inferred columns before writing --where filters.
- **Requests start failing after rapid pulls**  -  The API enforces rate limits (thresholds undocumented)  -  space out calls or reduce export page size.
