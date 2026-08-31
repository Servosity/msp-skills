# Cove Data Protection CLI

**The first CLI and MCP server for Cove Data Protection  -  fleet-wide backup health, billing usage, and storage trends from a terminal, with the local history the vendor console doesn't keep.**

Cove's console scopes to one customer at a time and forgets yesterday. cove-cli speaks the whole JSON-RPC API: one command sweeps every partner for failed or stale backups with column codes decoded to human names, `snapshot` keeps timestamped SQLite history so `storage growth` and `devices changes` can answer trend questions nothing else can, and `call` reaches all 251 documented methods.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `cove-cli` and `cove-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.ps1 | iex
   ```
3. Verify: `cove-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cove). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/cove --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/cove --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cove-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cove skill from https://github.com/Servosity/msp-skills/tree/main/skills/cove. The skill defines how its required CLI (`cove-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cove).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

> **Interim note:** a `.mcpb` bundle built before the [#287](https://github.com/Servosity/msp-skills/issues/287) fix does not launch. Its `manifest.json` runs `${__dirname}/bin/cove-mcp`, but the archive stores the binaries under platform-suffixed names (`bin/cove-mcp-darwin-arm64` and four siblings), so Claude Desktop finds nothing at that path. Check the bundle you downloaded with `unzip -l <file>.mcpb | grep bin/`: if `bin/cove-mcp` is not listed, use the installer above and the manual JSON config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "cove": {
      "command": "cove-mcp"
    }
  }
}
```

</details>

## Authentication

Cove's JSON-RPC API authenticates with a partner-scoped login, not a bearer token or API key. Create a dedicated **API User** in the Cove Management Console (**Users > API Users**); it issues a login name and an API token (shown only once). Set `COVE_USERNAME` to the API user's login name, `COVE_PASSWORD` to the API token, and `COVE_PARTNER` to the customer/partner the API user was created for - for an API User `COVE_PARTNER` is **required**, not optional, and an empty partner is the usual cause of a `2100 "Unknown partner/username"` error. `COVE_PARTNER` must be the **full customer/partner string exactly as shown in the Cove console customer dropdown** - usually including the account email in parentheses, e.g. `Acme Corp (admin@acme.com)`, **not** just the company name; Cove returns the same `2100 "Unknown partner/username or bad password"` error for a wrong `COVE_PARTNER` format as for a bad password. The API token is the *password*; it is not itself a visa and is never sent as a header (passing it to `--visa` fails by design). N-able removed the older per-user "API access" checkbox, and API Users cannot sign in to the console. Run `cove-cli auth login` once: it calls the `Login` method with these three values, receives a session token (the "visa"), and caches it locally with auto-refresh on expiry. Hand-built commands (failures, stale, health, billing, snapshot, call) inject the visa automatically. The raw generated endpoint commands (partners/devices/storage `list`·`get`, `promoted *`) do **not** auto-inject it: pass `--visa "$(cove-cli auth token)"`, or prefer `cove-cli call <Method>`, which injects the visa for you.

## Quick Start

```bash
# Verify the CLI can reach api.backup.management before touching credentials
cove-cli doctor --dry-run

# Exchange COVE_USERNAME/COVE_PASSWORD for a cached session visa
cove-cli auth login

# The morning sweep: every failed/aborted/not-started backup across all customers
cove-cli devices failures --since 24h --json

# Capture a timestamped fleet snapshot into local SQLite  -  builds the history layer
cove-cli snapshot

# Once two snapshots exist: which devices are growing storage fastest
cove-cli storage growth --since 7d --json

```

## Unique Features

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

## Usage

Run `cove-cli --help` for the full command reference and flag list.

## Commands

### audit

Management console audit trail

- **`cove-cli audit`** - Enumerate audit actions in a time range (JSON-RPC EnumerateAuditActions)

### backup-jobs

Backup/restore jobs

- **`cove-cli backup-jobs`** - Enumerate jobs for a partner (JSON-RPC EnumerateJobs)

### columns

Statistics column metadata (pairs with the README column-code legend)

- **`cove-cli columns`** - Enumerate custom statistic columns visible to a partner (JSON-RPC EnumerateColumns)

### devices

Backup devices (accounts) and their column-coded statistics

- **`cove-cli devices get`** - Get one device by id (JSON-RPC GetAccountInfoById)
- **`cove-cli devices list`** - Enumerate backup devices for a partner (JSON-RPC EnumerateAccounts)
- **`cove-cli devices stats`** - Query device statistics by column codes (JSON-RPC EnumerateAccountStatistics; see README column-code legend)

### labels

Device labels

- **`cove-cli labels`** - Enumerate all device labels (JSON-RPC EnumerateAllLabels)

### locations

Data-center locations

- **`cove-cli locations`** - Enumerate data-center locations (JSON-RPC EnumerateLocations)

### partners

Customers and resellers in the partner hierarchy

- **`cove-cli partners get`** - Get one partner by id (JSON-RPC GetPartnerInfoById)
- **`cove-cli partners list`** - Enumerate partners under a parent (JSON-RPC EnumeratePartners)

### products

Backup products available to a partner

- **`cove-cli products`** - Enumerate products (JSON-RPC EnumerateProducts)

### server

Management service metadata

- **`cove-cli server`** - Get management server info (JSON-RPC GetServerInfo)

### storage

Storage pools, statistics, and nodes

- **`cove-cli storage list`** - Enumerate storage pools for a partner (JSON-RPC EnumerateStorages)
- **`cove-cli storage nodes`** - Enumerate all storage nodes (JSON-RPC EnumerateAllStorageNodes)
- **`cove-cli storage stats`** - Storage usage statistics per location (JSON-RPC EnumerateStorageStatistics)

### users

Console users

- **`cove-cli users`** - Enumerate users for partners (JSON-RPC EnumerateUsers)


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
cove-cli audit

# JSON for scripting and agents
cove-cli audit --json

# Filter to specific fields
cove-cli audit --json --select id,name,status

# Dry run  -  show the request without sending
cove-cli audit --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
cove-cli audit --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
cove-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Config file: `~/.config/cove-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **error 1701 "Visa is inconsistent/corrupted"**  -  Session expired or missing  -  run `cove-cli auth login` again; hand-built commands (failures, stale, health, billing, snapshot, call) re-login automatically when credentials are in the environment. The raw generated endpoint commands (partners/devices/storage `list`·`get`, `promoted *`) do not auto-inject the cached visa  -  pass `--visa "$(cove-cli auth token)"`, or prefer `cove-cli call <Method>`, which injects it for you
- **error 2100 "Unknown partner/username or bad password"**  -  Verify COVE_USERNAME is the API user's login name, COVE_PASSWORD is its API token, and COVE_PARTNER is the customer/partner the API user was created for. COVE_PARTNER is required for API Users and an empty partner is the most common cause. Set COVE_PARTNER to the **full customer/partner string exactly as shown in the Cove console customer dropdown** - usually including the account email in parentheses, e.g. `Acme Corp (admin@acme.com)`, not just the company name; Cove returns this same error for a wrong COVE_PARTNER format as for a bad password. Create the API User under Users > API Users in the Cove Management Console (the older per-user "API access" checkbox has been removed)
- **error 13501 "Operation failed because of security reasons"**  -  The visa was accepted but the method was refused  -  the API User's role may not permit it. Verify the API User's role in the Cove console, or run the method through `cove-cli call <Method>`, which manages the session visa automatically
- **storage growth / devices changes return empty or a single-snapshot note**  -  These commands diff local snapshots  -  run `cove-cli snapshot` at least twice, some time apart, before asking for trends
- **Generated endpoint commands (devices list, partners list) return an error envelope instead of data**  -  Raw endpoint commands need an explicit visa: pass --visa "$(cove-cli auth token)" or prefer the hand-built equivalents which authenticate automatically

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**BackupNerd/Backup-Scripts**](https://github.com/BackupNerd/Backup-Scripts)  -  PowerShell
- [**impelling/CoveBackupApi**](https://github.com/impelling/CoveBackupApi)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
