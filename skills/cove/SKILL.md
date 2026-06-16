---
name: cove
description: "The first CLI and MCP server for Cove Data Protection  -  fleet-wide backup health, billing usage, and storage trends from a terminal, with the local history the vendor console doesn't keep. Trigger phrases: `which backups failed last night`, `check cove backup status`, `stale cove devices`, `cove storage growth`, `cove billing usage report`, `use cove`, `run cove-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Cove Data Protection"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - cove-cli
    install:
      - kind: go
        bins: [cove-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/cove/cmd/cove-cli
---

# Cove Data Protection  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `cove-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install cove --cli-only
   ```
2. Verify: `cove-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/cove/cmd/cove-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Cove's console scopes to one customer at a time and forgets yesterday. cove-cli speaks the whole JSON-RPC API: one command sweeps every partner for failed or stale backups with column codes decoded to human names, `snapshot` keeps timestamped SQLite history so `storage growth` and `devices changes` can answer trend questions nothing else can, and `call` reaches all 251 documented methods.

## When to Use This CLI

Use cove-cli when a task touches N-able Cove Data Protection fleet state: which backups failed or went stale, per-customer health rollups, storage growth, month-end SKU/seat billing, partner/device/user/audit enumeration, or any of the 251 JSON-RPC methods via `call`. It is the right tool for cross-customer questions the backup.management console can only answer one partner at a time, and for trend questions that need the local snapshot history.

## Anti-triggers

Do not use this CLI for:
- Restoring files or browsing backed-up data  -  restores run through the Backup Manager client or console, and per-device session detail lives on storage-node Reporting Service endpoints this CLI does not cover
- Installing or controlling the Backup Manager agent on endpoints  -  use deployment tooling (RMM) for that
- N-able N-central or RMM administration  -  this CLI only speaks the Cove (backup.management) API
- Accounts that only have interactive 2FA logins  -  the JSON-RPC API needs an API-enabled service account

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local history that compounds
- **`storage growth`**  -  See which devices and customers are growing their backup storage fastest, from timestamped local snapshots.

  _Reach for this when capacity planning or quota questions need a trend, not a point-in-time number._

  ```bash
  cove-cli storage growth --since 7d --agent
  ```
- **`devices changes`**  -  Devices whose backup status flipped between the two latest snapshots  -  regressions and recoveries without re-sweeping the fleet.

  _Answers 'what broke since last week' and 'did Friday's failures clear' in one command._

  ```bash
  cove-cli devices changes --since 7d --json
  ```

### Fleet triage
- **`devices failures`**  -  Every device across all customers whose last backup session failed, aborted, errored, or never started  -  decoded to status names.

  _The MSP morning ritual: run this first to build the day's ticket queue._

  ```bash
  cove-cli devices failures --since 24h --agent
  ```
- **`devices stale`**  -  Devices with no successful backup in N days, ranked by staleness and grouped by customer.

  _Catches silent gaps: a device can report a fine last status while not having succeeded for days._

  ```bash
  cove-cli devices stale --days 3 --json
  ```
- **`fleet health`**  -  One-screen rollup: total devices, healthy, failed, stale, never-run  -  with a per-customer breakdown.

  _The at-a-glance number for standups and QBR prep._

  ```bash
  cove-cli fleet health --by partner --json
  ```

### Billing
- **`billing usage`**  -  Per-device SKU, used storage, and M365 seat counts with column codes decoded  -  the month-end billing export in one command.

  _Turns Cove usage into invoice lines without the column-code legend open in another tab._

  ```bash
  cove-cli billing usage --csv
  ```
- **`billing changes`**  -  Devices whose plan changed since last month  -  current SKU vs Cove's built-in previous-month SKU column.

  _Run at month-end to catch upgrades, downgrades, and seat changes that affect invoices._

  ```bash
  cove-cli billing changes --json
  ```

## Command Reference

**audit**  -  Management console audit trail

- `cove-cli audit`  -  Enumerate audit actions in a time range (JSON-RPC EnumerateAuditActions)

**backup-jobs**  -  Backup/restore jobs

- `cove-cli backup-jobs`  -  Enumerate jobs for a partner (JSON-RPC EnumerateJobs)

**columns**  -  Statistics column metadata (pairs with the README column-code legend)

- `cove-cli columns`  -  Enumerate custom statistic columns visible to a partner (JSON-RPC EnumerateColumns)

**devices**  -  Backup devices (accounts) and their column-coded statistics

- `cove-cli devices get`  -  Get one device by id (JSON-RPC GetAccountInfoById)
- `cove-cli devices list`  -  Enumerate backup devices for a partner (JSON-RPC EnumerateAccounts)
- `cove-cli devices stats`  -  Query device statistics by column codes (JSON-RPC EnumerateAccountStatistics; see README column-code legend)

**labels**  -  Device labels

- `cove-cli labels`  -  Enumerate all device labels (JSON-RPC EnumerateAllLabels)

**locations**  -  Data-center locations

- `cove-cli locations`  -  Enumerate data-center locations (JSON-RPC EnumerateLocations)

**partners**  -  Customers and resellers in the partner hierarchy

- `cove-cli partners get`  -  Get one partner by id (JSON-RPC GetPartnerInfoById)
- `cove-cli partners list`  -  Enumerate partners under a parent (JSON-RPC EnumeratePartners)

**products**  -  Backup products available to a partner

- `cove-cli products`  -  Enumerate products (JSON-RPC EnumerateProducts)

**server**  -  Management service metadata

- `cove-cli server`  -  Get management server info (JSON-RPC GetServerInfo)

**storage**  -  Storage pools, statistics, and nodes

- `cove-cli storage list`  -  Enumerate storage pools for a partner (JSON-RPC EnumerateStorages)
- `cove-cli storage nodes`  -  Enumerate all storage nodes (JSON-RPC EnumerateAllStorageNodes)
- `cove-cli storage stats`  -  Storage usage statistics per location (JSON-RPC EnumerateStorageStatistics)

**users**  -  Console users

- `cove-cli users`  -  Enumerate users for partners (JSON-RPC EnumerateUsers)


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
cove-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning failure sweep for the ticket queue

```bash
cove-cli devices failures --since 24h --agent --select items.device_name,items.customer,items.status_name
```

Cross-customer failed/aborted/not-started list narrowed to the three fields a triage ticket needs.

### Find silently stale devices

```bash
cove-cli devices stale --days 3 --json
```

Devices with no successful session in 3 days, ranked worst-first  -  catches gaps the last-status view hides.

### Month-end billing export

```bash
cove-cli billing usage --csv
```

Per-device SKU, used storage, and M365 seats as CSV for the invoice model.

### What broke since the last snapshot

```bash
cove-cli devices changes --since 7d --json
```

After two `cove-cli snapshot` runs some time apart: regressions and recoveries between them.

### Call any of the 251 JSON-RPC methods

```bash
cove-cli call GetServerInfo --json
```

Generic escape hatch with automatic visa injection and typed vendor error mapping.

## Auth Setup

Cove's JSON-RPC API authenticates with a partner-scoped login, not a bearer token or API key. Create a dedicated **API User** in the Cove Management Console (**Users > API Users**); it issues a login name and an API token (shown only once). Set `COVE_USERNAME` to the API user's login name, `COVE_PASSWORD` to the API token, and `COVE_PARTNER` to the customer/partner the API user was created for - for an API User `COVE_PARTNER` is **required**, not optional, and an empty partner is the usual cause of a `2100 "Unknown partner/username"` error. The API token is the *password*; it is not itself a visa and is never sent as a header (passing it to `--visa` fails by design). N-able removed the older per-user "API access" checkbox, and API Users cannot sign in to the console. Run `cove-cli auth login` once: it calls the `Login` method with these three values, receives a session token (the "visa"), and caches it locally with auto-refresh on expiry. Hand-built commands (failures, stale, health, billing, snapshot, call) inject the visa automatically. Raw generated endpoint commands accept `--visa $(cove-cli auth token)` for ad-hoc use.

Run `cove-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  cove-cli audit --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
cove-cli feedback "the --since flag is inclusive but docs say exclusive"
cove-cli feedback --stdin < notes.txt
cove-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/cove-cli/feedback.jsonl`. They are never POSTed unless `COVE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `COVE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
cove-cli profile save briefing --json
cove-cli --profile briefing audit
cove-cli profile list --json
cove-cli profile show briefing
cove-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `cove-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/cove/cmd/cove-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add cove-mcp -- cove-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which cove-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   cove-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `cove-cli <command> --help`.
