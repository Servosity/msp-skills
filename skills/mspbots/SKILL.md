---
name: mspbots
description: "The first MSPbots tool anywhere  -  readable filters, alias-named resources, full exports, and the KPI history MSPbots itself doesn't keep. Trigger phrases: `pull the open tickets dataset from MSPbots`, `export MSPbots data to CSV`, `snapshot our MSPbots KPIs`, `is our ticket backlog up or down this week`, `what columns does this MSPbots dataset have`, `use mspbots`, `run mspbots-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "MSPbots"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - mspbots-cli
    install:
      - kind: go
        bins: [mspbots-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/mspbots/cmd/mspbots-cli
---

# MSPbots  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `mspbots-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install mspbots --cli-only
   ```
2. Verify: `mspbots-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/mspbots/cmd/mspbots-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

MSPbots' Public API shares your BI datasets and widgets but ships with 19-digit IDs, a comma-encoded filter DSL, manual pagination, and no history. This CLI turns a shared API key into a usable data faucet: register aliases once, filter with readable predicates, export whole tables in one command, and snapshot KPIs into local SQLite for diffs and trends no MSPbots surface can show.

## When to Use This CLI

Reach for this CLI whenever MSP business-intelligence data living in MSPbots needs to leave the dashboard: piping ticket/SLA/utilization rows into scripts, capturing scheduled KPI snapshots for week-over-week reporting, exporting full datasets to CSV for finance, or letting an AI agent answer service-desk questions from real data. It is the only programmatic MSPbots surface that exists.

## Anti-triggers

Do not use this CLI for:
- Creating, rotating, or binding MSPbots API keys  -  that is UI-only (Settings → Public API)
- Managing bots, dashboards, alerts, or any write operation inside MSPbots  -  the Public API is strictly read-only
- Pulling PSA/RMM data that is not bound to your key  -  bind it in MSPbots first, or use that tool's own API
- Reading widgets built with measure or calculate layers  -  the Public API does not support them

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`registry`**  -  Name your bound datasets and widgets once, then use readable aliases everywhere instead of 19-digit resource IDs.

  _Agents and scripts can address resources by stable, human-readable names instead of copy-pasted snowflake IDs._

  ```bash
  mspbots-cli registry add <alias> <resourceId> --type dataset
  ```
- **`snapshot`**  -  Capture point-in-time copies of any dataset or widget into local SQLite  -  the history MSPbots doesn't keep.

  _Run it on a schedule and every later question about "how has this changed" becomes answerable offline._

  ```bash
  mspbots-cli snapshot <alias>
  ```
- **`trend`**  -  Time-series and point-over-point deltas for any numeric column across stored snapshots.

  _Answers "is this KPI up or down since last week"  -  the question the live API structurally cannot answer._

  ```bash
  mspbots-cli trend <alias> --column "Open Count"
  ```
- **`diff`**  -  Row-level added/removed/changed comparison between two stored snapshots of the same resource.

  _Shows exactly which tickets entered or left a queue between two captures, not just the count._

  ```bash
  mspbots-cli diff <alias>
  ```

### Agent-native plumbing
- **`pull`**  -  Write filters like "Update Date >= 2026-05-29" and the CLI compiles them into MSPbots' comma-encoded operator DSL.

  _Reach for this instead of hand-building query strings; it handles operator encoding, spaces in column names, and URL-escaping._

  ```bash
  mspbots-cli pull <alias> --where "Update Date >= 2026-05-29" --where "Status = Open" --json
  ```
- **`export`**  -  Dump an entire dataset or widget to CSV or JSONL, walking every page automatically.

  _One command replaces a babysat pagination loop when feeding spreadsheets or downstream pipelines._

  ```bash
  mspbots-cli export <alias> --format csv
  ```
- **`describe`**  -  Sample live rows and infer the column names and types of a dataset or widget.

  _Run it before building --where filters so column names and types are known instead of guessed._

  ```bash
  mspbots-cli describe <alias>
  ```

## Command Reference

**dataset**  -  Fetch rows of datasets bound to your Public API key

- `mspbots-cli dataset <resourceId>`  -  Fetch one page of rows from a dataset bound to your API key

**widget**  -  Fetch data of widgets bound to your Public API key

- `mspbots-cli widget <resourceId>`  -  Fetch one page of data from a bound widget (widgets with measure or calculate layers are not supported by the Public


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
mspbots-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Register and pull a shared dataset

```bash
mspbots-cli registry add <alias> <resourceId> --type dataset
```

One-time alias setup; every later command addresses the resource by its alias.

### Filtered pull with readable predicates

```bash
mspbots-cli pull <alias> --where "Update Date >= 2026-05-01" --where "Status = Open" --json
```

The CLI compiles readable operators into MSPbots' comma-encoded query DSL and URL-encodes spaced column names.

### Agent-shaped KPI read

```bash
mspbots-cli pull <alias> --agent --select row_count,rows
```

Returns only the row count and rows fields in agent-envelope JSON  -  bounded context for LLM consumption.

### Full CSV export for finance

```bash
mspbots-cli export <alias> --format csv
```

Walks every page via current/size automatically and streams one clean CSV.

### Week-over-week KPI movement

```bash
mspbots-cli trend <alias> --column "Open Count"
```

Aggregates the column across stored snapshots; pair with a scheduled `snapshot` to keep the series growing.

## Auth Setup

MSPbots uses a raw API key sent as an `apikey` request header. An MSPbots admin creates the key in the MSPbots app under Settings → Public API (Add New Key, type Custom), flips the global Enable Public API toggle ON, and explicitly binds each dataset and widget the key may read. Set the key in your environment as `MSPBOTS_API_KEY`. Every endpoint is read-only; a key can never write anything. If a pull returns "resource unbound", the dataset or widget has not been bound to your key  -  that binding happens in the MSPbots UI, not in this CLI.

Run `mspbots-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  mspbots-cli dataset <id> --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Read-only**  -  do not use this CLI for create, update, delete, publish, comment, upvote, invite, order, send, or other mutating requests

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
mspbots-cli feedback "the --since flag is inclusive but docs say exclusive"
mspbots-cli feedback --stdin < notes.txt
mspbots-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/mspbots-cli/feedback.jsonl`. They are never POSTed unless `MSPBOTS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `MSPBOTS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
mspbots-cli profile save briefing --json
mspbots-cli --profile briefing dataset <id>
mspbots-cli profile list --json
mspbots-cli profile show briefing
mspbots-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `mspbots-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/mspbots/cmd/mspbots-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add mspbots-mcp -- mspbots-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which mspbots-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   mspbots-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `mspbots-cli <command> --help`.
