---
name: connectwise-automate
description: "Use when the user asks to check ConnectWise Automate fleet health, find stale or offline agents, report patch compliance by client, triage open alerts across clients, inventory end-of-life OSes, or see what changed overnight across an RMM fleet. Syncs your whole Automate server into a local SQLite mirror so it answers cross-client questions the per-server console can't. Trigger phrases: `connectwise automate fleet health`, `stale automate agents`, `automate patch compliance by client`, `triage automate alerts`, `use connectwise automate`, `run connectwise-automate-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ConnectWise"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - connectwise-automate-cli
---

# ConnectWise Automate Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `connectwise-automate-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-automate/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-automate/install.ps1 | iex
   ```
3. Verify: `connectwise-automate-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

ConnectWise Automate's value is locked behind a per-server console built for one-endpoint-at-a-time work. This CLI syncs your whole fleet  -  computers, clients, locations, alerts, and patch history  -  into local SQLite, then answers the questions MSPs actually ask across clients: where the offline agents are (stale-agents), who's behind on patches (patch-compliance), and what changed overnight (since). Everything is offline, scriptable, and built for AI agents.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-client roll-ups the per-server UI can't do
- **`fleet-health`**  -  See every agent across every client in one roll-up: online/offline, last-contact age, and open-alert count, grouped by client.

  _Reach for this when an agent needs the whole-fleet posture in one shot instead of paging the Computers endpoint client by client._

  ```bash
  connectwise-automate-cli fleet-health --agent
  ```
- **`stale-agents`**  -  List computers not seen in N days, grouped by client  -  the offline agents bleeding license and hiding risk.

  _Use before license true-ups or security reviews to find agents that stopped checking in._

  ```bash
  connectwise-automate-cli stale-agents --days 30 --agent
  ```
- **`patch-compliance`**  -  Per-client patch posture from synced patch history joined to computers  -  worst offenders first.

  _Pull this before a QBR or a security conversation to name the clients that are behind._

  ```bash
  connectwise-automate-cli patch-compliance --agent
  ```
- **`client-rollup`**  -  One-line-per-client snapshot: computers, locations, offline agents, and open alerts  -  built for the client review.

  _The fastest way to brief on a client's whole environment without clicking through the console._

  ```bash
  connectwise-automate-cli client-rollup --agent
  ```

### Triage and inventory
- **`alert-triage`**  -  Open alerts across every client, ranked by priority and joined to computer → location → client, with duplicates collapsed.

  _Start the morning here to see what actually needs a human across the whole book of business._

  ```bash
  connectwise-automate-cli alert-triage --min-priority 3 --agent
  ```
- **`os-inventory`**  -  Fleet-wide operating-system distribution with end-of-life OSes (Windows 7, Server 2008/2012) flagged for upgrade planning.

  _Use for security and hardware-refresh planning when you need the EOL exposure across every client at once._

  ```bash
  connectwise-automate-cli os-inventory --eol-only --agent
  ```
- **`since`**  -  Fleet activity in the last N hours from the records' own timestamps: alerts created, agents that checked in, and patches installed.

  _Run first thing to catch overnight activity across the fleet without paging each endpoint._

  ```bash
  connectwise-automate-cli since --hours 24 --agent
  ```

## Recipes

### Morning triage across all clients

```bash
connectwise-automate-cli alert-triage --min-priority 3 --agent --select client,computer,priority,message
```

Narrows the alert payload to the four fields that matter so an agent doesn't burn context on the full alert objects.

### Find offline agents for a license true-up

```bash
connectwise-automate-cli stale-agents --days 45 --agent
```

Lists computers not seen in 45 days, grouped by client, straight from the local store.

### Patch posture for a QBR

```bash
connectwise-automate-cli patch-compliance --agent
```

Per-client patch percentages, worst first, ready to paste into a client review.

### Filter live computers without syncing

```bash
connectwise-automate-cli computers list --condition "Status='Offline'" --order-by "LastContact asc" --agent
```

Uses Automate's own condition/orderby query against the live API when you don't want a full sync.

## Command Reference

**alerts**  -  Open monitor alerts across the fleet

- `connectwise-automate-cli alerts get`  -  Get a single alert by Id
- `connectwise-automate-cli alerts list`  -  List open alerts across all computers

**apitoken**  -  Mint and refresh API bearer tokens

- `connectwise-automate-cli apitoken mint`  -  Mint a bearer token from username + password (needs CONNECTWISE_AUTOMATE_SERVER + clientId header)
- `connectwise-automate-cli apitoken refresh`  -  Refresh an existing (still-valid) bearer token

**clients**  -  Clients (companies / customers) in Automate

- `connectwise-automate-cli clients get`  -  Get a single client by Id
- `connectwise-automate-cli clients list`  -  List all clients

**commands**  -  Available commands that can be executed on agents

- `connectwise-automate-cli commands get`  -  Get a single command by Id
- `connectwise-automate-cli commands list`  -  List all available commands

**computers**  -  Managed endpoints (agents)  -  the core RMM inventory

- `connectwise-automate-cli computers alerts`  -  Open alerts for one computer
- `connectwise-automate-cli computers command-execute`  -  Execute a command on one computer (WRITE  -  runs a real command on the agent)
- `connectwise-automate-cli computers command-history`  -  Recent command execution history for one computer
- `connectwise-automate-cli computers get`  -  Get a single computer by Id
- `connectwise-automate-cli computers list`  -  List computers across the fleet (paginated, filterable)
- `connectwise-automate-cli computers patching-stats`  -  Patch installation statistics for one computer
- `connectwise-automate-cli computers software`  -  Installed software inventory for one computer

**contacts**  -  Client contacts

- `connectwise-automate-cli contacts`  -  List all client contacts

**groups**  -  Computer groups (organizational + policy grouping)

- `connectwise-automate-cli groups get`  -  Get a single group by Id
- `connectwise-automate-cli groups list`  -  List all groups

**locations**  -  Locations (sites) belonging to clients

- `connectwise-automate-cli locations get`  -  Get a single location by Id
- `connectwise-automate-cli locations list`  -  List all locations

**monitors**  -  Monitors and their per-monitor statistics

- `connectwise-automate-cli monitors list`  -  List monitors with their alerting statistics
- `connectwise-automate-cli monitors sensor-checks`  -  List sensor checks

**network-devices**  -  Discovered network devices (non-agent)

- `connectwise-automate-cli network-devices`  -  List discovered network devices

**patching**  -  Patch history, compliance information, and patch policies

- `connectwise-automate-cli patching approval-policies`  -  Patch approval policies
- `connectwise-automate-cli patching deploy-approved`  -  Deploy all approved patches (WRITE  -  triggers fleet patch deployment)
- `connectwise-automate-cli patching deploy-security`  -  Deploy all security patches (WRITE  -  triggers fleet patch deployment)
- `connectwise-automate-cli patching information`  -  Global patch information / catalog status
- `connectwise-automate-cli patching list`  -  Fleet-wide patch installation history
- `connectwise-automate-cli patching microsoft-policies`  -  Microsoft update policies
- `connectwise-automate-cli patching reattempt-failed`  -  Reattempt failed patches (WRITE  -  retries failed patch installs)
- `connectwise-automate-cli patching thirdparty-policies`  -  Third-party update policies

**scripts**  -  Automation scripts and their run state

- `connectwise-automate-cli scripts list`  -  List all scripts
- `connectwise-automate-cli scripts running`  -  List scripts currently running across the fleet
- `connectwise-automate-cli scripts schedules`  -  List scheduled script runs

**server**  -  Automate server metadata (used by doctor / health)

- `connectwise-automate-cli server db-time`  -  Current database server time
- `connectwise-automate-cli server info`  -  Server version and metadata


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
connectwise-automate-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Auth Setup

Automate is per-server: set CONNECTWISE_AUTOMATE_SERVER to your host (e.g. company.hostedrmm.com) and CONNECTWISE_AUTOMATE_CLIENT_ID to your registered integration GUID (required for v2020.11+). Mint a bearer token with `apitoken mint --username <u> --password <p>`, then export CONNECTWISE_AUTOMATE_TOKEN with the returned AccessToken. Tokens are short-lived; refresh with `apitoken refresh`.

Run `connectwise-automate-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  connectwise-automate-cli alerts list --agent --select id,name,status
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
connectwise-automate-cli feedback "the --since flag is inclusive but docs say exclusive"
connectwise-automate-cli feedback --stdin < notes.txt
connectwise-automate-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/connectwise-automate-cli/feedback.jsonl`. They are never POSTed unless `CONNECTWISE_AUTOMATE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CONNECTWISE_AUTOMATE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration.

```
connectwise-automate-cli profile save briefing --json
connectwise-automate-cli --profile briefing alerts list
connectwise-automate-cli profile list --json
connectwise-automate-cli profile show briefing
connectwise-automate-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `connectwise-automate-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up), then register it:

```bash
claude mcp add connectwise-automate-mcp -- connectwise-automate-mcp
```

Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which connectwise-automate-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   connectwise-automate-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `connectwise-automate-cli <command> --help`.
