# Level CLI

**Every Level RMM endpoint, plus a local SQLite fleet store and offline cross-entity rollups no Level tool has: at-risk ranking, patch posture, alert triage, and stale-device detection in one command.**

levelio-cli syncs your entire Level estate  -  devices, groups, tags, custom fields, alerts, and OS updates  -  into a local SQLite database, then answers portfolio-wide questions offline that the Level web UI shows one device at a time. Match every API operation with agent-native output (--json/--select/--csv), then transcend with weighted at-risk ranking, fleet-wide patch posture, per-client posture scorecards, group-clustered alert triage, reboot-debt tracking, and custom-field coverage audits.

## Install

The recommended path installs both the `levelio-cli` binary and the `pp-levelio` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install levelio
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install levelio --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install levelio --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install levelio --agent claude-code
npx -y @mvanhorn/printing-press-library install levelio --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/levelio/cmd/levelio-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/levelio-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install levelio --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-levelio --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-levelio --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install levelio --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/levelio-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `LEVEL_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/levelio/cmd/levelio-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "levelio": {
      "command": "levelio-mcp",
      "env": {
        "LEVEL_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Authenticate with a Level API key (Settings -> API keys; read-only is enough for every list/show/sync/analytics command). Export it as LEVEL_API_TOKEN; it is sent as 'Authorization: Bearer <token>'.

## Quick Start

```bash
# confirm the API key works and api.level.io is reachable
levelio-cli doctor

# pull the whole estate into the local SQLite store
levelio-cli sync

# one-screen inventory rollup by operating system
levelio-cli fleet --by os

# the 20 worst endpoints across alerts, patches, score, and staleness
levelio-cli at-risk --top 20

# fleet-wide security-update exposure
levelio-cli patch-posture --category security

# active critical fires clustered by group
levelio-cli alert-triage --severity critical --group-by group

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-entity fleet intelligence
- **`at-risk`**  -  Rank the worst endpoints across every axis at once  -  active alerts, pending patches, low security score, and how long they have been dark  -  as a single weighted risk score.

  _Reach for this when an agent needs the single prioritized 'fix these first' list across the whole fleet instead of one health axis at a time._

  ```bash
  levelio-cli at-risk --top 20 --agent
  ```
- **`patch-posture`**  -  Aggregate OS updates across the fleet  -  available vs installed, by category and device, including patches that errored  -  so you can see exposure at a glance.

  _Use when an agent must report fleet-wide patch exposure or pick which category of updates to push next._

  ```bash
  levelio-cli patch-posture --category security --agent
  ```
- **`fleet`**  -  One-screen inventory rollup, cross-tabbed any way you slice it  -  by OS, platform, group, or tag  -  with online and maintenance counts.

  _Reach for this for a portfolio-wide inventory answer instead of paging through device lists._

  ```bash
  levelio-cli fleet --by os --online --agent
  ```
- **`alert-triage`**  -  Cluster unresolved alerts by group and severity with device context, so systemic fires surface above one-off noise.

  _Reach for this to answer 'where are my fires and which are systemic' in one call._

  ```bash
  levelio-cli alert-triage --severity critical --group-by group --agent
  ```
- **`client-scorecard`**  -  One row per top-level group (client) with device count, online %, open critical alerts, average security score, stale count, and patch exposure  -  the QBR-ready per-client rollup.

  _Reach for this when an agent needs per-client fleet posture in one table  -  which client is worst, where to focus  -  instead of walking groups and devices one call at a time._

  ```bash
  levelio-cli client-scorecard --agent
  ```

### Health and drift
- **`stale`**  -  List devices that have gone dark  -  not seen in N days  -  with an option to exclude machines intentionally in maintenance mode.

  _Use to find agents that silently stopped checking in without chasing each device in the UI._

  ```bash
  levelio-cli stale --days 14 --exclude-maintenance --agent
  ```
- **`group-tree`**  -  Render the Level group hierarchy with each node showing its real rolled-up health  -  descendant device, alert, stale, and score counts.

  _Use to see the org structure annotated with where the problems actually concentrate._

  ```bash
  levelio-cli group-tree --with alerts,stale,score --agent
  ```
- **`since`**  -  Show what changed in a recent time window  -  new alerts, newly published updates, and devices last seen  -  so you can answer 'what happened since I last looked'.

  _Reach for this at the start of a shift to catch up on everything that moved overnight._

  ```bash
  levelio-cli since --hours 24 --agent
  ```
- **`alert-recurrence`**  -  Rank which alert names fire most often across the fleet and on how many distinct devices, so chronically noisy monitors surface above one-off fires.

  _Reach for this when the question is which monitors are chronically noisy and worth tuning, not which fires are burning right now (use alert-triage for that)._

  ```bash
  levelio-cli alert-recurrence --top 15 --agent
  ```
- **`reboot-due`**  -  List devices waiting on a reboot to finalize installed patches  -  and how long they have been waiting  -  joined with online and maintenance state.

  _Reach for this when patches are installed but not yet effective  -  the reboot backlog is the gap between patch posture on paper and in reality._

  ```bash
  levelio-cli reboot-due --days 3 --agent
  ```

### Governance and hygiene
- **`cf-coverage`**  -  Audit which devices, groups, or the org are missing a custom-field value  -  the absence the UI can't show  -  via an anti-join.

  _Reach for this when an agent must enforce data hygiene  -  e.g. every device must carry an asset-tag or warranty field._

  ```bash
  levelio-cli cf-coverage --missing --agent
  ```
- **`security-posture`**  -  Show the fleet security-score distribution and everyone under a threshold, optionally rolled up by group.

  _Use to report fleet security posture or target the weakest endpoints for remediation._

  ```bash
  levelio-cli security-posture --below 70 --by-group --agent
  ```
- **`tag-audit`**  -  Surface tag-data drift: devices with zero tags, orphan tags applied to nothing, and duplicate tag names that fragment the fleet.

  _Reach for this when fleet filters and automations misbehave because tag data drifted  -  untagged devices and orphan tags are invisible until audited._

  ```bash
  levelio-cli tag-audit --agent
  ```

## Recipes


### Morning fleet catch-up

```bash
levelio-cli since --hours 12 --agent
```

Everything that changed overnight  -  new alerts, new updates, devices that checked in  -  in one structured payload.

### Patch-day exposure report

```bash
levelio-cli patch-posture --category security --agent --select summary,by_category
```

Fleet-wide pending vs installed security updates, narrowed to just the rollup fields an agent needs.

### Find the worst endpoints fast

```bash
levelio-cli at-risk --top 10 --agent --select rank,hostname,risk_score,reasons
```

Top-10 risk ranking with only the decision fields, ready to pipe into a ticket.

### Dark-agent sweep

```bash
levelio-cli stale --days 7 --exclude-maintenance
```

Devices that have not checked in for a week and are not in maintenance  -  the silent-failure list.

### Data-hygiene audit

```bash
levelio-cli cf-coverage --missing --agent
```

Every assignee missing a custom-field value, so required asset/warranty data can be backfilled.

### Per-client QBR rollup

```bash
levelio-cli client-scorecard --agent
```

One row per client group  -  devices, online %, open criticals, average security score, stale count, and patch exposure  -  ready for a QBR slide.

## Usage

Run `levelio-cli --help` for the full command reference and flag list.

## Commands

### alerts

Alerts generated by monitoring devices.

- **`levelio-cli alerts list`** - Returns a list of your alerts.
- **`levelio-cli alerts show`** - Retrieves the details of an existing alert.

### automations

Operations on automations.

- **`levelio-cli automations <token>`** - Triggers an automation via a webhook.

### custom-field-values

Values assigned to custom fields for the organization, groups, and devices.

- **`levelio-cli custom-field-values delete`** - Deletes a custom field value for the organization, group, or device.
- **`levelio-cli custom-field-values list`** - Returns a list of custom field values for the organization, groups, or devices.
- **`levelio-cli custom-field-values update`** - Set a custom field value for the organization, group, or device.

### custom-fields

Custom fields that can have values assigned to them.

- **`levelio-cli custom-fields create`** - Creates a custom field.
- **`levelio-cli custom-fields delete`** - Deletes a custom field.
- **`levelio-cli custom-fields list`** - Returns a list of custom fields.
- **`levelio-cli custom-fields show`** - Retrieves the details of an existing custom field.
- **`levelio-cli custom-fields update`** - Updates a custom field.

### devices

Devices with the Level agent installed.

- **`levelio-cli devices delete`** - Deletes the specified device.
- **`levelio-cli devices list`** - Returns a list of your devices.
- **`levelio-cli devices show`** - Retrieves the details of an existing device.
- **`levelio-cli devices update`** - Updates the specified device.

### groups

The group hierachy that contains devices.

- **`levelio-cli groups create`** - Creates a new group.
- **`levelio-cli groups delete`** - Deletes an existing group.
- **`levelio-cli groups list`** - Returns a list of your groups.
- **`levelio-cli groups show`** - Retrieves the details of an existing group.
- **`levelio-cli groups update`** - Updates an existing group.

### tags

Tags that can be applied to devices.

- **`levelio-cli tags create`** - Creates a new tag.
- **`levelio-cli tags delete`** - Deletes a tag.
- **`levelio-cli tags list`** - Returns a list of tags.
- **`levelio-cli tags show`** - Retrieves a tag.
- **`levelio-cli tags update`** - Updates an existing tag.

### updates

Update operations and status for devices.

- **`levelio-cli updates list`** - Returns a list of your updates.
- **`levelio-cli updates show`** - Retrieves the details of an existing update.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
levelio-cli alerts list

# JSON for scripting and agents
levelio-cli alerts list --json

# Filter to specific fields
levelio-cli alerts list --json --select id,name,status

# Dry run  -  show the request without sending
levelio-cli alerts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
levelio-cli alerts list --agent
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

## Health Check

```bash
levelio-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/level-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `LEVEL_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `levelio-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `levelio-cli doctor` to check credentials
- Verify the environment variable is set: `echo $LEVEL_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Access denied**  -  Set LEVEL_API_TOKEN to a valid Level API key (Settings -> API keys). Read-only scope is sufficient for read commands.
- **Analytics commands return empty or stale data**  -  Run 'levelio-cli sync' first  -  the at-risk/fleet/patch-posture/group-tree commands read the local store, not the live API.
- **A list looks truncated**  -  Lists follow Level's cursor pagination automatically; use --limit to cap or let sync page the full estate.
