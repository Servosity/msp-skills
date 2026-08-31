# RocketCyber CLI

**The first CLI and MCP server for RocketCyber Managed SOC, with triage and posture analytics no console page or API call computes.**

Every RocketCyber Customer API v3 endpoint as an agent-ready command, including the suppression rules and CSV report export that the only other tool (a PowerShell module) lacks. On top sit computed SOC analytics: a cross-account triage board, incident MTTR, stale-agent detection, Defender risk ranking, and secure-score trends, backed by an offline SQLite store with full-text search.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `rocketcyber-cli` and `rocketcyber-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex
   ```
3. Verify: `rocketcyber-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=rocketcyber). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/rocketcyber --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/rocketcyber --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `rocketcyber-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the rocketcyber skill from https://github.com/Servosity/msp-skills/tree/main/skills/rocketcyber. The skill defines how its required CLI (`rocketcyber-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=rocketcyber).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ROCKETCYBER_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

> **Interim note:** a `.mcpb` bundle built before the [#287](https://github.com/Servosity/msp-skills/issues/287) fix does not launch. Its `manifest.json` runs `${__dirname}/bin/rocketcyber-mcp`, but the archive stores the binaries under platform-suffixed names (`bin/rocketcyber-mcp-darwin-arm64` and four siblings), so Claude Desktop finds nothing at that path. Check the bundle you downloaded with `unzip -l <file>.mcpb | grep bin/`: if `bin/rocketcyber-mcp` is not listed, use the installer above and the manual JSON config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "rocketcyber": {
      "command": "rocketcyber-mcp",
      "env": {
        "ROCKETCYBER_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

RocketCyber uses a single bearer API token. In the RocketCyber console, go to Provider settings and copy your API token, then export it as ROCKETCYBER_API_TOKEN. US partners use the default base URL; EU and AP partners should set the regional API host (for example https://api-eu.rocketcyber.com/v3) as the base URL in the config file. Verify with rocketcyber-cli doctor.

## Quick Start

```bash
# Health check: verifies config, token presence, and API reachability without side effects
rocketcyber-cli doctor --dry-run

# Confirm auth works and see your provider account plus client account IDs
rocketcyber-cli account --json

# Open SOC incidents across accounts - the core triage feed
rocketcyber-cli incidents --status open --json

# Snapshot agents, incidents, and detection apps into the local SQLite store
rocketcyber-cli sync --resources agents,incidents,apps --full

# One ranked board: open incidents + event verdicts + offline agents
rocketcyber-cli triage --since 24h --agent

```

## Unique Features

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

## Usage

Run `rocketcyber-cli --help` for the full command reference and flag list.

## Commands

### account

Provider account information and client account hierarchy

- **`rocketcyber-cli account`** - Get account information, including child customer accounts

### agents

RocketCyber agents (monitored devices) and their connectivity

- **`rocketcyber-cli agents`** - List agents (devices) with inventory and connectivity filters

### apps

Detection apps catalog (threat detection modules)

- **`rocketcyber-cli apps`** - List detection apps and their status for an account

### defender

Microsoft Defender health and devices-at-risk telemetry

- **`rocketcyber-cli defender`** - Get Defender detection summary, devices at risk, and device health

### events

Verdict-classified detection events per detection app

- **`rocketcyber-cli events list`** - List detection events for an app, filtered by verdict and date window
- **`rocketcyber-cli events summary`** - Per-app event verdict counts for an account

### firewalls

Firewall log sources feeding the SOC

- **`rocketcyber-cli firewalls`** - List firewall log sources, optionally with ingest counters

### incidents

SOC-published incidents with status and lifecycle timestamps

- **`rocketcyber-cli incidents`** - List SOC incidents with status, title, and date filters

### office

Microsoft 365 secure-score telemetry

- **`rocketcyber-cli office`** - Get the Microsoft 365 secure-score daily progress series

### reports

CSV report export (reportApi)

- **`rocketcyber-cli reports`** - Export events or incidents as a CSV report

### suppression

Alert-suppression rules that filter SOC noise

- **`rocketcyber-cli suppression rule`** - Get a single suppression rule by ID
- **`rocketcyber-cli suppression rules`** - List alert-suppression rules with status and ownership filters


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
rocketcyber-cli account

# JSON for scripting and agents
rocketcyber-cli account --json

# Filter to specific fields
rocketcyber-cli account --json --select id,name,status

# Dry run  -  show the request without sending
rocketcyber-cli account --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
rocketcyber-cli account --agent
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

Set `ROCKETCYBER_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `rocketcyber-cli agents`
- `rocketcyber-cli apps`
- `rocketcyber-cli firewalls`
- `rocketcyber-cli incidents`
- `rocketcyber-cli suppression`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
rocketcyber-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/rocketcyber-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ROCKETCYBER_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `rocketcyber-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `rocketcyber-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ROCKETCYBER_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Authentication Error on every call**  -  Regenerate the API token in the RocketCyber console (Provider settings) and re-export ROCKETCYBER_API_TOKEN
- **Valid token but all calls return 401 (EU/AP partner)**  -  Your tenant lives on a regional host - set base_url to your region (for example https://api-eu.rocketcyber.com/v3) in ~/.config/rocketcyber-cli/config.toml
- **events list returns an error about a missing app id**  -  The events endpoint requires --app-id - find IDs with rocketcyber-cli apps
- **agents stale or incidents mttr returns empty results**  -  These read the local store - run rocketcyber-cli sync --resources agents,incidents --full first

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Celerium.RocketCyber**](https://github.com/Celerium/Celerium.RocketCyber)  -  PowerShell
- [**RocketCyber-PowerShellWrapper**](https://github.com/Celerium/RocketCyber-PowerShellWrapper)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
