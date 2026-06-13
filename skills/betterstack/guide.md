# Better Stack CLI

**Every Better Stack Uptime feature, plus an offline SQLite mirror and cross-resource fleet analytics  -  what's down and who's paged, coverage gaps, MTTA/MTTR, flapping, on-call gaps, and status-page drift  -  that the API alone can't answer.**

betterstack-cli mirrors your whole Better Stack Uptime account (monitors, heartbeats, incidents, on-call, escalation policies, status pages) into a local SQLite store, then answers operational questions no single API call can: `coverage` finds monitors that won't page anyone, `mttr` rolls up incident response times, `flapping` ranks your noisiest monitors, and `fleet` shows the whole account on one screen. Covers the official Terraform provider's resource surface (full create/update/delete on monitors and heartbeats; create/delete on groups, policies, and status pages), with agent-native output and typed exit codes throughout.

## Install

The recommended path installs both the `betterstack-cli` binary and the `pp-betterstack` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install betterstack
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install betterstack --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install betterstack --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install betterstack --agent claude-code
npx -y @mvanhorn/printing-press-library install betterstack --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/betterstack/cmd/betterstack-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/betterstack-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install betterstack --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-betterstack --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-betterstack --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install betterstack --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/betterstack-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `BETTERSTACK_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/betterstack/cmd/betterstack-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "betterstack": {
      "command": "betterstack-mcp",
      "env": {
        "BETTERSTACK_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Auth is a Better Stack Uptime API token (Authorization: Bearer). Create one under Better Stack → Settings → API tokens, then set BETTERSTACK_API_TOKEN. Run `betterstack-cli doctor` to confirm the token and reachability.

## Quick Start

```bash
# confirm the token works and the API is reachable
betterstack-cli doctor

# mirror the account into the local SQLite store
betterstack-cli sync

# see monitors, heartbeats, open incidents, and on-call at a glance
betterstack-cli fleet

# find monitors with no escalation policy or alert channel
betterstack-cli coverage

# page through monitors live, agent-friendly JSON
betterstack-cli monitors list --per-page 250 --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-resource analytics the API can't answer
- **`fleet`**  -  One-screen health of the whole Better Stack account: monitors up/down/paused, heartbeats, open incidents, and who's on call now.

  _Reach for this first when an agent needs the operational state of the entire account in one call instead of five._

  ```bash
  betterstack-cli fleet --agent
  ```
- **`coverage`**  -  Find monitors with no escalation policy or no alert channel  -  the ones that will go down silently and page no one.

  _Use before an incident to prove every monitor actually escalates to a human._

  ```bash
  betterstack-cli coverage --agent
  ```
- **`down`**  -  Focused triage list of monitors currently down or degraded, joined to their open incidents and whether anyone is actually paged for each.

  _Reach for this at the start of a shift or during an outage to see what is down and who is (or is not) being paged, in one call._

  ```bash
  betterstack-cli down --agent
  ```
- **`triage`**  -  Open incidents ranked by age and acknowledgement state  -  never-acknowledged first  -  joined to the affected monitor.

  _Use this to prioritize incident response: it surfaces the open incidents nobody has acknowledged yet, oldest first._

  ```bash
  betterstack-cli triage --agent
  ```
- **`statuspage-audit`**  -  Flags status pages showing operational while a backing monitor has an open incident, and status-page resources pointing at missing or paused monitors.

  _Use this before or during an incident to catch public status pages that are silently out of sync with reality._

  ```bash
  betterstack-cli statuspage-audit --agent
  ```
- **`group-health`**  -  Per-group health rollup: monitor and heartbeat up/down counts plus open incidents for every monitor group and heartbeat group.

  _For MSP-style accounts where one group is one client, this answers per-client health in a single call._

  ```bash
  betterstack-cli group-health --agent
  ```

### Incident intelligence
- **`mttr`**  -  Mean time to acknowledge and resolve, computed across incidents over a window and broken down by monitor.

  _Use for on-call retros and SLA reporting without exporting to a spreadsheet._

  ```bash
  betterstack-cli mttr --days 30 --agent
  ```
- **`flapping`**  -  Rank monitors by how many incidents they generated in a window to surface the noisy, flapping, or misconfigured ones.

  _Use to find alert fatigue sources before tuning thresholds._

  ```bash
  betterstack-cli flapping --days 7 --top 10 --agent
  ```

### On-call and resilience
- **`oncall-gaps`**  -  Detect on-call calendars with nobody currently on call.

  _Use to confirm someone is actually reachable on every rotation before relying on paging._

  ```bash
  betterstack-cli oncall-gaps --agent
  ```
- **`heartbeat-risk`**  -  Rank heartbeats by risk: tight period+grace windows, paused-but-expected, and non-up status.

  _Use to catch fragile cron/scheduled-task check-ins before they false-alarm or silently miss._

  ```bash
  betterstack-cli heartbeat-risk --agent
  ```

## Recipes


### Find unprotected monitors

```bash
betterstack-cli coverage --agent
```

Lists monitors with no escalation policy or alert channel so you can fix paging gaps before an outage.

### 30-day incident response report

```bash
betterstack-cli mttr --days 30 --agent
```

Computes MTTA and MTTR across the window from the local mirror  -  no spreadsheet export needed.

### Narrow a verbose monitor payload

```bash
betterstack-cli monitors list --per-page 250 --agent --select data.id,data.attributes.pronounceable_name,data.attributes.status
```

Uses --select dotted paths to pull only id, name, and status from the JSON:API envelope so agents don't burn context on full monitor objects.

### Acknowledge an incident safely

```bash
betterstack-cli incidents acknowledge 12345 --by oncall@example.com --dry-run
```

Shows the exact request without sending; drop --dry-run to actually acknowledge.

### Rank the noisiest monitors

```bash
betterstack-cli flapping --days 7 --top 10 --agent
```

Surfaces the monitors generating the most incidents so you can tune alert thresholds.

## Usage

Run `betterstack-cli --help` for the full command reference and flag list.

## Commands

### heartbeat-groups

Heartbeat groups

- **`betterstack-cli heartbeat-groups create`** - Create a heartbeat group
- **`betterstack-cli heartbeat-groups delete`** - Delete a heartbeat group
- **`betterstack-cli heartbeat-groups get`** - Get a heartbeat group by ID
- **`betterstack-cli heartbeat-groups list`** - List heartbeat groups

### heartbeats

Heartbeats  -  cron/scheduled-task check-ins

- **`betterstack-cli heartbeats create`** - Create a heartbeat
- **`betterstack-cli heartbeats delete`** - Delete a heartbeat
- **`betterstack-cli heartbeats get`** - Get a heartbeat by ID
- **`betterstack-cli heartbeats list`** - List heartbeats
- **`betterstack-cli heartbeats update`** - Update a heartbeat (sends PATCH)

### incidents

Incidents  -  outages and alerts across monitors and heartbeats

- **`betterstack-cli incidents acknowledge`** - Acknowledge an incident
- **`betterstack-cli incidents delete`** - Delete an incident
- **`betterstack-cli incidents get`** - Get an incident by ID
- **`betterstack-cli incidents list`** - List incidents
- **`betterstack-cli incidents resolve`** - Resolve an incident

### monitor-groups

Monitor groups

- **`betterstack-cli monitor-groups create`** - Create a monitor group
- **`betterstack-cli monitor-groups delete`** - Delete a monitor group
- **`betterstack-cli monitor-groups get`** - Get a monitor group by ID
- **`betterstack-cli monitor-groups list`** - List monitor groups

### monitors

Uptime monitors (HTTP, keyword, ping, TCP, heartbeat-backed)

- **`betterstack-cli monitors create`** - Create a monitor
- **`betterstack-cli monitors delete`** - Delete a monitor
- **`betterstack-cli monitors get`** - Get a monitor by ID
- **`betterstack-cli monitors list`** - List all monitors
- **`betterstack-cli monitors update`** - Update a monitor (sends PATCH)

### on-calls

On-call calendars and current on-call shifts

- **`betterstack-cli on-calls get`** - Get an on-call calendar by ID (includes current on-call users)
- **`betterstack-cli on-calls list`** - List on-call calendars

### policies

Escalation policies

- **`betterstack-cli policies create`** - Create an escalation policy
- **`betterstack-cli policies delete`** - Delete an escalation policy
- **`betterstack-cli policies get`** - Get an escalation policy by ID
- **`betterstack-cli policies list`** - List escalation policies

### status-page-resources

Resources (monitored components) shown on a status page

- **`betterstack-cli status-page-resources delete`** - Remove a resource from a status page
- **`betterstack-cli status-page-resources get`** - Get a status page resource
- **`betterstack-cli status-page-resources list`** - List resources on a status page

### status-page-sections

Sections within a status page

- **`betterstack-cli status-page-sections create`** - Create a status page section
- **`betterstack-cli status-page-sections delete`** - Delete a status page section
- **`betterstack-cli status-page-sections get`** - Get a status page section
- **`betterstack-cli status-page-sections list`** - List sections of a status page

### status-pages

Public status pages

- **`betterstack-cli status-pages create`** - Create a status page
- **`betterstack-cli status-pages delete`** - Delete a status page
- **`betterstack-cli status-pages get`** - Get a status page by ID
- **`betterstack-cli status-pages list`** - List status pages


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
betterstack-cli heartbeat-groups list

# JSON for scripting and agents
betterstack-cli heartbeat-groups list --json

# Filter to specific fields
betterstack-cli heartbeat-groups list --json --select id,name,status

# Dry run  -  show the request without sending
betterstack-cli heartbeat-groups list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
betterstack-cli heartbeat-groups list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `BETTERSTACK_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `betterstack-cli coverage`
- `betterstack-cli down`
- `betterstack-cli flapping`
- `betterstack-cli fleet`
- `betterstack-cli group-health`
- `betterstack-cli heartbeat-groups`
- `betterstack-cli heartbeat-groups get`
- `betterstack-cli heartbeat-groups list`
- `betterstack-cli heartbeat-risk`
- `betterstack-cli heartbeats`
- `betterstack-cli heartbeats get`
- `betterstack-cli heartbeats list`
- `betterstack-cli incidents`
- `betterstack-cli incidents get`
- `betterstack-cli incidents list`
- `betterstack-cli monitor-groups`
- `betterstack-cli monitor-groups get`
- `betterstack-cli monitor-groups list`
- `betterstack-cli monitors`
- `betterstack-cli monitors get`
- `betterstack-cli monitors list`
- `betterstack-cli mttr`
- `betterstack-cli on-calls`
- `betterstack-cli on-calls get`
- `betterstack-cli on-calls list`
- `betterstack-cli oncall-gaps`
- `betterstack-cli policies`
- `betterstack-cli policies get`
- `betterstack-cli policies list`
- `betterstack-cli status-pages`
- `betterstack-cli status-pages get`
- `betterstack-cli status-pages list`
- `betterstack-cli statuspage-audit`
- `betterstack-cli triage`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
betterstack-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/betterstack-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `BETTERSTACK_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `betterstack-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `betterstack-cli doctor` to check credentials
- Verify the environment variable is set: `echo $BETTERSTACK_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set BETTERSTACK_API_TOKEN to a valid Uptime API token (Settings → API tokens), then re-run `doctor`.
- **fleet/coverage/mttr return empty or 'run sync first'**  -  Run `betterstack-cli sync` to populate the local mirror before querying analytics.
- **List command only shows 50 rows**  -  Pass `--per-page 250` (the API max) or `--page N`; the `sync` command walks every page automatically.
