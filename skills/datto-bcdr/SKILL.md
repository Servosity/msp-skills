---
name: datto-bcdr
description: "Sync your whole Datto BCDR fleet into local SQLite and answer the questions the per-appliance Partner Portal can't: which backups failed screenshot verification, which are stale, and which clients are at risk. Trigger phrases: `which datto backups failed screenshot verification`, `datto bcdr fleet health`, `which clients are at risk in datto`, `find stale datto backups`, `datto storage runway`, `use datto-bcdr`, `run datto-bcdr-cli`, `datto recoverability score`, `client backup report for qbr`, `triage datto alerts fleet-wide`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Datto BCDR"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - datto-bcdr-cli
    install:
      - kind: go
        bins: [datto-bcdr-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/datto-bcdr/cmd/datto-bcdr-cli
---

# Datto BCDR  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `datto-bcdr-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install datto-bcdr --cli-only
   ```
2. Verify: `datto-bcdr-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/datto-bcdr/cmd/datto-bcdr-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The Datto BCDR API is strictly per-device  -  to check backup health you query one appliance at a time. This CLI syncs every device, agent, share, and alert into a local store, then runs fleet-wide local joins: screenshots --failed surfaces every silently-unbootable backup, client-risk ranks your clients by composite risk, and storage-runway tells you which appliance fills up first. All read-only, all agent-native with --json and --select.

## When to Use This CLI

Use this CLI when an agent needs the fleet-wide view of a partner's Datto BCDR estate: proving backups are recoverable, triaging which clients are at risk, finding stale or unprotected machines, and planning storage capacity. It is the right tool whenever the question spans more than one appliance, because the Datto API itself only answers per-device.

## When Not to Use This CLI

Do not activate this CLI for requests that require creating, updating, deleting, publishing, commenting, upvoting, inviting, ordering, sending messages, booking, purchasing, or changing remote state. This printed CLI exposes read-only commands for inspection, export, sync, and analysis.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Recovery Assurance
- **`screenshots`**  -  See every protected machine whose last backup-bootability screenshot failed, across your entire fleet, ranked by how long it's been failing and grouped by client.

  _Reach for this first every morning  -  it surfaces silently-unbootable backups before a client ever needs to restore._

  ```bash
  datto-bcdr-cli screenshots --failed --stale-days 7 --json
  ```
- **`stale-backups`**  -  Find every agent whose last local snapshot or last offsite sync is older than a threshold, across all devices and clients at once.

  _Use to check recovery-point freshness fleet-wide; catches a backup that quietly stopped taking points without firing an alert._

  ```bash
  datto-bcdr-cli stale-backups --local-days 1 --offsite-days 3 --json
  ```
- **`recoverability`**  -  One headline KPI: the percentage of fleet agents whose latest recovery point is both fresh and screenshot-verified bootable, with a breakdown of what drags it down.

  _Reach for this when leadership asks whether backups are actually recoverable  -  one defensible number instead of a device-by-device tour._

  ```bash
  datto-bcdr-cli recoverability --json
  ```

### Fleet Health
- **`client-risk`**  -  Per-client risk scorecard that rolls up screenshot failures, stale backups, open alerts, storage pressure, and warranty status into one ranked list of which clients are most at risk.

  _Reach for this when someone asks which clients are at risk  -  it answers the exact business question the per-device portal cannot._

  ```bash
  datto-bcdr-cli client-risk --top 10 --json
  ```
- **`alert-triage`**  -  Every open alert across the whole fleet in one ranked view, grouped by client and device, instead of pulling alerts one appliance at a time.

  _Use for morning alert triage  -  the whole fleet's open alerts ranked by client without walking the device list._

  ```bash
  datto-bcdr-cli alert-triage --group-by client --json
  ```
- **`storage-runway`**  -  Rank every appliance by remaining local and offsite storage and flag the devices and clients closest to running out of capacity.

  _Use when planning capacity or answering what's our storage runway  -  surfaces the appliance that fills up before anyone notices._

  ```bash
  datto-bcdr-cli storage-runway --threshold-pct 85 --json
  ```

### Coverage Gaps
- **`forgotten-assets`**  -  List agents that are paused or archived and devices that haven't checked in recently, across the whole fleet, so silently-unprotected machines and dead appliances get caught.

  _Reach for this to catch protection someone paused temporarily months ago, or an appliance that quietly went dark  -  gaps that never generate an alert._

  ```bash
  datto-bcdr-cli forgotten-assets --offline-days 2 --json
  ```
- **`agent-versions`**  -  Audit agent software versions across the entire fleet and flag every machine running an outdated agent, grouped by client and device.

  _Use during patch/maintenance planning or when an agent vulnerability is announced, to find every exposed install in one shot._

  ```bash
  datto-bcdr-cli agent-versions --outdated --json
  ```

### Client Reporting
- **`client-report`**  -  One QBR-ready health report for a single client: devices, agents, screenshot pass rate, stale backups, and open alerts in one bundled view.

  _Use when preparing a QBR or answering one client's are-we-protected question  -  the full single-client story in one command._

  ```bash
  datto-bcdr-cli client-report "Acme Corp" --json
  ```

## Command Reference

**agent**  -  Protected agents (machines) across the fleet or on one device

- `datto-bcdr-cli agent by-device`  -  List protected agents on a specific device
- `datto-bcdr-cli agent list`  -  List every protected agent across all devices

**alert**  -  Open alerts raised by a device

- `datto-bcdr-cli alert <serialNumber>`  -  List open alerts for a device

**asset**  -  All assets (agents and shares) on a device, plus single-volume detail

- `datto-bcdr-cli asset get`  -  Get a single asset by its volume name
- `datto-bcdr-cli asset list`  -  List all assets (agents + shares) on a device

**device**  -  Datto BCDR appliances (SIRIS / ALTO / NAS / Backup-for-Azure)

- `datto-bcdr-cli device get`  -  Get a single BCDR device by serial number
- `datto-bcdr-cli device list`  -  List all BCDR devices in the partner account

**shares**  -  Network shares protected on a device

- `datto-bcdr-cli shares <serialNumber>`  -  List protected shares on a device

**vm-restore**  -  Virtualization / VM restore sessions on a device

- `datto-bcdr-cli vm-restore <serialNumber>`  -  List VM restore sessions on a device


## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `DATTO_BCDR_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

- `datto-bcdr-cli agent`
- `datto-bcdr-cli agent by-device`
- `datto-bcdr-cli agent list`
- `datto-bcdr-cli device`
- `datto-bcdr-cli device get`
- `datto-bcdr-cli device list`

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
datto-bcdr-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning recovery-assurance sweep

```bash
datto-bcdr-cli screenshots --failed --stale-days 7 --agent
```

Lists every agent whose bootability screenshot has been failing 7+ days, in agent-native output.

### Narrowed fleet export for a report

```bash
datto-bcdr-cli agent list --agent --select agentName,os,lastScreenshotAttemptStatus,lastSnapshot
```

Pulls just the columns a recoverability report needs, skipping the verbose payload.

### Client-by-client risk briefing

```bash
datto-bcdr-cli client-risk --top 10 --json
```

Ranked per-client risk scorecard ready to pipe into a status update.

### Capacity planning

```bash
datto-bcdr-cli storage-runway --threshold-pct 85 --json
```

Flags every appliance over 85% on local or offsite storage.

### Fleet recoverability KPI

```bash
datto-bcdr-cli recoverability --json
```

One defensible number  -  the % of agents whose latest recovery point is fresh AND screenshot-verified bootable  -  for the morning report or leadership ask.

### Fleet-wide alert triage

```bash
datto-bcdr-cli alert-triage --group-by client --json
```

All open alerts across every appliance grouped by client, replacing a per-serial walk of the device list.

## Auth Setup

Datto BCDR uses HTTP Basic auth with a partner-generated key pair. Generate a public/secret key in the Datto Partner Portal under Admin > Integrations, then export DATTO_BCDR_PUBLIC_KEY and DATTO_BCDR_SECRET_KEY. The CLI base64-encodes public:secret into the Authorization header on every request.

Run `datto-bcdr-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  datto-bcdr-cli agent list --agent --select id,name,status
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
datto-bcdr-cli feedback "the --since flag is inclusive but docs say exclusive"
datto-bcdr-cli feedback --stdin < notes.txt
datto-bcdr-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/datto-bcdr-cli/feedback.jsonl`. They are never POSTed unless `DATTO_BCDR_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `DATTO_BCDR_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
datto-bcdr-cli profile save briefing --json
datto-bcdr-cli --profile briefing agent list
datto-bcdr-cli profile list --json
datto-bcdr-cli profile show briefing
datto-bcdr-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `datto-bcdr-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/datto-bcdr/cmd/datto-bcdr-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add datto-bcdr-mcp -- datto-bcdr-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which datto-bcdr-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   datto-bcdr-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `datto-bcdr-cli <command> --help`.
