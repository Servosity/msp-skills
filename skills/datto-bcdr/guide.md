# Datto BCDR CLI

**Sync your whole Datto BCDR fleet into local SQLite and answer the questions the per-appliance Partner Portal can't: which backups failed screenshot verification, which are stale, and which clients are at risk.**

The Datto BCDR API is strictly per-device  -  to check backup health you query one appliance at a time. This CLI syncs every device, agent, share, and alert into a local store, then runs fleet-wide local joins: screenshots --failed surfaces every silently-unbootable backup, client-risk ranks your clients by composite risk, and storage-runway tells you which appliance fills up first. All read-only, all agent-native with --json and --select.

## Install

The recommended path installs both the `datto-bcdr-cli` binary and the `pp-datto-bcdr` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install datto-bcdr
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install datto-bcdr --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install datto-bcdr --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install datto-bcdr --agent claude-code
npx -y @mvanhorn/printing-press-library install datto-bcdr --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/datto-bcdr/cmd/datto-bcdr-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/datto-bcdr-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install datto-bcdr --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-datto-bcdr --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-datto-bcdr --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install datto-bcdr --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/datto-bcdr-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `DATTO_BCDR_PUBLIC_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/datto-bcdr/cmd/datto-bcdr-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "datto-bcdr": {
      "command": "datto-bcdr-mcp",
      "env": {
        "DATTO_BCDR_PUBLIC_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Datto BCDR uses HTTP Basic auth with a partner-generated key pair. Generate a public/secret key in the Datto Partner Portal under Admin > Integrations, then export DATTO_BCDR_PUBLIC_KEY and DATTO_BCDR_SECRET_KEY. The CLI base64-encodes public:secret into the Authorization header on every request.

## Quick Start

```bash
# confirm your key pair is set and the API is reachable
datto-bcdr-cli doctor

# hydrate the local store with your entire fleet
datto-bcdr-cli sync

# the daily question: which backups are not provably bootable
datto-bcdr-cli screenshots --failed

# which clients are most at risk right now
datto-bcdr-cli client-risk --top 10

```

## Unique Features

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

## Usage

Run `datto-bcdr-cli --help` for the full command reference and flag list.

## Commands

### agent

Protected agents (machines) across the fleet or on one device

- **`datto-bcdr-cli agent by-device`** - List protected agents on a specific device
- **`datto-bcdr-cli agent list`** - List every protected agent across all devices

### alert

Open alerts raised by a device

- **`datto-bcdr-cli alert <serialNumber>`** - List open alerts for a device

### asset

All assets (agents and shares) on a device, plus single-volume detail

- **`datto-bcdr-cli asset get`** - Get a single asset by its volume name
- **`datto-bcdr-cli asset list`** - List all assets (agents + shares) on a device

### device

Datto BCDR appliances (SIRIS / ALTO / NAS / Backup-for-Azure)

- **`datto-bcdr-cli device get`** - Get a single BCDR device by serial number
- **`datto-bcdr-cli device list`** - List all BCDR devices in the partner account

### shares

Network shares protected on a device

- **`datto-bcdr-cli shares <serialNumber>`** - List protected shares on a device

### vm-restore

Virtualization / VM restore sessions on a device

- **`datto-bcdr-cli vm-restore <serialNumber>`** - List VM restore sessions on a device


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
datto-bcdr-cli agent list

# JSON for scripting and agents
datto-bcdr-cli agent list --json

# Filter to specific fields
datto-bcdr-cli agent list --json --select id,name,status

# Dry run  -  show the request without sending
datto-bcdr-cli agent list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
datto-bcdr-cli agent list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `DATTO_BCDR_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `datto-bcdr-cli agent`
- `datto-bcdr-cli agent by-device`
- `datto-bcdr-cli agent list`
- `datto-bcdr-cli device`
- `datto-bcdr-cli device get`
- `datto-bcdr-cli device list`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
datto-bcdr-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/datto-bcdr-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `DATTO_BCDR_PUBLIC_KEY` | per_call | Yes | Set to your API credential. |
| `DATTO_BCDR_SECRET_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `datto-bcdr-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `datto-bcdr-cli doctor` to check credentials
- Verify the environment variable is set: `echo $DATTO_BCDR_PUBLIC_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Confirm DATTO_BCDR_PUBLIC_KEY and DATTO_BCDR_SECRET_KEY are exported; regenerate the key pair in the Partner Portal if it was revoked.
- **Fleet commands return nothing**  -  Run `datto-bcdr-cli sync` first  -  the transcendence commands read the local store, not the live API.
- **A device is missing from the list**  -  Pass --show-hidden 1 (and --show-child-reseller 1) to device list, then re-sync.
