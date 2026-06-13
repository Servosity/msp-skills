---
name: rocketcyber
description: "The first CLI and MCP server for RocketCyber Managed SOC, with triage and posture analytics no console page or API call computes. Trigger phrases: `check rocketcyber incidents`, `soc triage board`, `which rocketcyber agents are offline`, `rocketcyber secure score trend`, `audit rocketcyber suppression rules`, `use rocketcyber`, `run rocketcyber-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "RocketCyber"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - rocketcyber-cli
    install:
      - kind: go
        bins: [rocketcyber-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/rocketcyber/cmd/rocketcyber-cli
---

# RocketCyber  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `rocketcyber-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install rocketcyber --cli-only
   ```
2. Verify: `rocketcyber-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/rocketcyber/cmd/rocketcyber-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Every RocketCyber Customer API v3 endpoint as an agent-ready command, including the suppression rules and CSV report export that the only other tool (a PowerShell module) lacks. On top sit computed SOC analytics: a cross-account triage board, incident MTTR, stale-agent detection, Defender risk ranking, and secure-score trends, backed by an offline SQLite store with full-text search.

## When to Use This CLI

Use this CLI for read-only RocketCyber Managed SOC operations: morning triage across client accounts, incident reporting and MTTR math, agent fleet hygiene, Defender and Microsoft 365 posture checks, suppression-rule audits, and CSV exports. It is the right choice when an agent needs structured SOC telemetry without a browser session.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to acknowledge, resolve, or modify incidents - the Customer API is read-only; incident response actions happen in the RocketCyber console
- Do not use this CLI to install or manage RocketCyber agents on endpoints - that is the RMM or installer's job
- Do not use this CLI for real-time alert streaming or webhooks - it polls REST endpoints; use console notification integrations for push alerting
- Do not use this CLI to manage Kaseya VSA or other Kaseya module resources - it only speaks the RocketCyber Customer API

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### SOC triage that compounds
- **`triage`**  -  One ranked board of open incidents, event verdict counts, and offline agents across every client account.

  _Reach for this when asked for an overall SOC health snapshot or what broke overnight, instead of three separate list calls._

  ```bash
  rocketcyber-cli triage --since 24h --agent
  ```
- **`agents stale`**  -  Devices that stopped reporting beyond a time window, grouped by client account.

  _Use this for fleet-hygiene sweeps (which devices went dark this week) rather than paging the live agents endpoint._

  ```bash
  rocketcyber-cli agents stale --since 7d --json
  ```

### Posture analytics
- **`incidents mttr`**  -  Mean and median time-to-resolve plus open-incident aging buckets, computed from incident created/resolved timestamps.

  _Use this for SLA and QBR reporting questions like how fast incidents get resolved, instead of fetching raw incidents and computing by hand._

  ```bash
  rocketcyber-cli incidents mttr --since 90d --json
  ```
- **`defender riskiest`**  -  Devices-at-risk ranked by weighted malicious and suspicious detection counts.

  _Use this when asked which machines need attention first, instead of parsing raw Defender JSON._

  ```bash
  rocketcyber-cli defender riskiest --account-id 2 --top 10 --json
  ```
- **`office trend`**  -  First/last/delta/direction computed over the Microsoft 365 secure-score daily series.

  _Use this for is-our-365-posture-improving questions instead of dumping the raw daily series._

  ```bash
  rocketcyber-cli office trend --account-id 2 --json
  ```
- **`suppression audit`**  -  Alert-suppression rules classified by status and age, flagging stale rules that may hide real detections.

  _Use this for suppression-rule hygiene reviews instead of fetching rules one by one._

  ```bash
  rocketcyber-cli suppression audit --stale-after 90d --json
  ```

## Command Reference

**account**  -  Provider account information and client account hierarchy

- `rocketcyber-cli account`  -  Get account information, including child customer accounts

**agents**  -  RocketCyber agents (monitored devices) and their connectivity

- `rocketcyber-cli agents`  -  List agents (devices) with inventory and connectivity filters

**apps**  -  Detection apps catalog (threat detection modules)

- `rocketcyber-cli apps`  -  List detection apps and their status for an account

**defender**  -  Microsoft Defender health and devices-at-risk telemetry

- `rocketcyber-cli defender`  -  Get Defender detection summary, devices at risk, and device health

**events**  -  Verdict-classified detection events per detection app

- `rocketcyber-cli events list`  -  List detection events for an app, filtered by verdict and date window
- `rocketcyber-cli events summary`  -  Per-app event verdict counts for an account

**firewalls**  -  Firewall log sources feeding the SOC

- `rocketcyber-cli firewalls`  -  List firewall log sources, optionally with ingest counters

**incidents**  -  SOC-published incidents with status and lifecycle timestamps

- `rocketcyber-cli incidents`  -  List SOC incidents with status, title, and date filters

**office**  -  Microsoft 365 secure-score telemetry

- `rocketcyber-cli office`  -  Get the Microsoft 365 secure-score daily progress series

**reports**  -  CSV report export (reportApi)

- `rocketcyber-cli reports`  -  Export events or incidents as a CSV report

**suppression**  -  Alert-suppression rules that filter SOC noise

- `rocketcyber-cli suppression rule`  -  Get a single suppression rule by ID
- `rocketcyber-cli suppression rules`  -  List alert-suppression rules with status and ownership filters


## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `ROCKETCYBER_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

- `rocketcyber-cli agents`
- `rocketcyber-cli apps`
- `rocketcyber-cli firewalls`
- `rocketcyber-cli incidents`
- `rocketcyber-cli suppression`

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
rocketcyber-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Overnight SOC triage in one command

```bash
rocketcyber-cli triage --since 24h --json
```

Fans out to incidents, event summaries, and agents, then returns one ranked cross-account board with partial-failure accounting.

### Narrow Defender risk to the fields that matter

```bash
rocketcyber-cli defender --account-id 2 --agent --select devicesAtRisk.data.hostname,devicesAtRisk.data.detections.malicious
```

The defender payload is deeply nested - dotted --select paths keep agent context small.

### Quarterly MTTR evidence

```bash
rocketcyber-cli incidents mttr --since 90d --json
```

Mean/median resolution hours plus aging buckets, computed from synced incident timestamps.

### Full-text hunt across incident remediation text

```bash
rocketcyber-cli search "ransomware" --type incidents --limit 20
```

FTS5 over synced incident title, description, and remediation - no API endpoint can text-search these.

### Find devices that went dark this week

```bash
rocketcyber-cli agents stale --since 7d --json
```

Filters synced agents on lastConnected age, a dimension the live API cannot filter by.

## Auth Setup

RocketCyber uses a single bearer API token. In the RocketCyber console, go to Provider settings and copy your API token, then export it as ROCKETCYBER_API_TOKEN. US partners use the default base URL; EU and AP partners should set the regional API host (for example https://api-eu.rocketcyber.com/v3) as the base URL in the config file. Verify with rocketcyber-cli doctor.

Run `rocketcyber-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  rocketcyber-cli account --agent --select id,name,status
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
rocketcyber-cli feedback "the --since flag is inclusive but docs say exclusive"
rocketcyber-cli feedback --stdin < notes.txt
rocketcyber-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/rocketcyber-cli/feedback.jsonl`. They are never POSTed unless `ROCKETCYBER_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ROCKETCYBER_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
rocketcyber-cli profile save briefing --json
rocketcyber-cli --profile briefing account
rocketcyber-cli profile list --json
rocketcyber-cli profile show briefing
rocketcyber-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `rocketcyber-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/rocketcyber/cmd/rocketcyber-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add rocketcyber-mcp -- rocketcyber-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which rocketcyber-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   rocketcyber-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `rocketcyber-cli <command> --help`.
