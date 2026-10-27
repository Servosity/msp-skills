# Cove Data Protection CLI

**Fleet-wide Cove backup health, billing usage and storage trends, with local history.**

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

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

### Remote MCP over HTTP

`cove-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `COVE_MCP_HTTP_TOKEN` is set, and every request must send `Authorization: Bearer <token>`. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`.

```bash
export COVE_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
cove-mcp --transport http
```

Set `COVE_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Cove's JSON-RPC API authenticates with a partner-scoped login, not a bearer token or API key. Create a dedicated **API User** in the Cove Management Console (**Users > API Users**); it issues a login name and an API token (shown only once). Set `COVE_USERNAME` to the API user's login name, `COVE_PASSWORD` to the API token, and `COVE_PARTNER` to the customer/partner the API user was created for - for an API User `COVE_PARTNER` is **required**, not optional, and an empty partner is the usual cause of a `2100 "Unknown partner/username"` error. `COVE_PARTNER` must be the **full customer/partner string exactly as shown in the Cove console customer dropdown** - usually including the account email in parentheses, e.g. `Acme Corp (admin@example.com)`, **not** just the company name; Cove returns the same `2100 "Unknown partner/username or bad password"` error for a wrong `COVE_PARTNER` format as for a bad password. The API token is the *password*; it is not itself a visa and is never sent as a header (passing it to `--visa` fails by design). N-able removed the older per-user "API access" checkbox, and API Users cannot sign in to the console. Run `cove-cli auth login` once: it calls the `Login` method with these three values, receives a session token (the "visa"), and caches it locally with auto-refresh on expiry. Hand-built commands (failures, stale, health, sources, activity, billing, snapshot, call) inject the visa automatically. The raw generated endpoint commands (partners/devices/storage `list` and `get`, and the promoted single-endpoint commands) do **not** auto-inject it: pass `--visa "$(cove-cli auth token)"`, or prefer `cove-cli call <Method>`, which injects the visa for you (it sends only Get and Enumerate methods unless you pass `--yes`).

## Quick Start

```bash
# Verify the CLI can reach api.backup.management before touching credentials
cove-cli doctor --dry-run

# Exchange COVE_PARTNER, COVE_USERNAME and COVE_PASSWORD for a cached session visa
cove-cli auth login

# The morning sweep: every failed/aborted/not-started backup across all customers
cove-cli devices failures --since 24h --json

# Capture a timestamped fleet snapshot into local SQLite  -  builds the history layer
cove-cli snapshot

# Once two snapshots exist: which devices are growing storage fastest
cove-cli storage growth --since 7d --json

```

## Unique Features

### Local history that compounds
- **`storage growth`**  -  See which devices and customers are growing their backup storage fastest, from timestamped local snapshots.

  _Reach for this when capacity planning or quota questions need a trend, not a point-in-time number._

  ```bash
  cove-cli storage growth --since 7d --agent
  ```
- **`devices changes`**  -  Devices whose backup status flipped between the two latest snapshots of the same partner tree: regressions and recoveries without re-sweeping the fleet.

  _Answers 'what broke since last week' and 'did Friday's failures clear' in one command._

  ```bash
  cove-cli devices changes --since 7d --json
  ```

### Fleet triage
- **`devices failures`**  -  Every device across all customers whose last backup session failed, aborted, errored, had no selection, never started or reported an unrecognised status, decoded to status names.

  _The MSP morning ritual: run this first to build the day's ticket queue._

  ```bash
  cove-cli devices failures --since 24h --agent
  ```
- **`devices stale`**  -  Devices with no successful backup in N days, ranked by staleness and grouped by customer.

  _Catches silent gaps: a device can report a fine last status while not having succeeded for days._

  ```bash
  cove-cli devices stale --days 3 --json
  ```
- **`fleet health`**  -  One-screen rollup: total devices, healthy, failed, stale, never-run and unknown status, with a per-customer breakdown.

  _The at-a-glance number for standups and QBR prep._

  ```bash
  cove-cli fleet health --by partner --json
  ```
- **`devices sources`**  -  Which data sources each device protects (Files, System State, SQL, Hyper-V, Microsoft 365), decoded fleet-wide, with a filter for devices missing one.

  _Reach for this when a restore plan assumes a data source is protected and you need to prove it across every customer._

  ```bash
  cove-cli devices sources --without "System State" --os server --agent
  ```
- **`fleet activity`**  -  Who did what in the Cove console: audit-trail actions grouped by user and operation across every sub-partner, with a breakdown by result.

  _Reach for this for a weekly review of console activity or after an incident, to see which accounts changed what._

  ```bash
  cove-cli fleet activity --since 7d --agent
  ```

### Billing
- **`billing usage`**  -  Per-device SKU, used storage, and M365 seat counts with column codes decoded  -  the month-end billing export in one command.

  _Turns Cove usage into invoice lines without the column-code legend open in another tab._

  ```bash
  cove-cli billing usage --csv
  ```
- **`billing changes`**  -  Devices whose plan changed since last month  -  current SKU vs Cove's built-in previous-month SKU column.

  _Run at month-end to catch plan upgrades and downgrades that affect invoices._

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

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data such as `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `COVE_CONFIG_DIR`, `COVE_DATA_DIR`, `COVE_STATE_DIR`, or `COVE_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `COVE_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export COVE_HOME=/srv/cove
cove-cli doctor
```

On Windows (PowerShell):

```powershell
$env:COVE_HOME = "C:\ProgramData\cove"
cove-cli doctor
```

Under `COVE_HOME=/srv/cove`, the four dirs resolve to `/srv/cove/config`, `/srv/cove/data`, `/srv/cove/state`, and `/srv/cove/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:COVE_HOME = "C:\srv\cove"
cove-cli doctor
```

Under `$env:COVE_HOME = "C:\srv\cove"`, the four dirs resolve to `C:\srv\cove\config`, `C:\srv\cove\data`, `C:\srv\cove\state`, and `C:\srv\cove\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "cove": {
      "command": "cove-mcp",
      "env": {
        "COVE_HOME": "/srv/cove"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `COVE_DATA_DIR` overrides an explicit `--home` for that kind. Use `COVE_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `COVE_HOME` does not move files back to platform defaults, and `doctor` cannot find files left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. Run `cove-cli doctor --fail-on warn` to check path warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. The learn journal lives under `learn/` in the state directory. `cove-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `cove-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `COVE_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `COVE_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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


### billing


- **`cove-cli billing usage`** - Per-device SKU, used storage, and M365 seat counts with column codes decoded  -  the month-end billing export in one command.

- **`cove-cli billing changes`** - Devices whose plan changed since last month  -  current SKU vs Cove's built-in previous-month SKU column.


### devices


- **`cove-cli devices failures`** - Every device across all customers whose last backup session failed, aborted, errored, or never started  -  decoded to status names.

- **`cove-cli devices stale`** - Devices with no successful backup in N days, ranked by staleness and grouped by customer.

- **`cove-cli devices changes`** - Devices whose backup status flipped between the two latest snapshots  -  regressions and recoveries without re-sweeping the fleet.

- **`cove-cli devices sources`** - Which data sources each device actually protects (Files and Folders, System State, MS SQL, Hyper-V, Microsoft 365 and more), decoded from Cove's active-sources column, with a fleet tally and a filter for devices missing a given source.


### fleet


- **`cove-cli fleet health`** - One-screen rollup: total devices, healthy, failed, stale, never-run  -  with a per-customer breakdown.

- **`cove-cli fleet activity`** - Who did what in the Cove console over a window: audit-trail actions grouped by user and operation across every sub-partner, with non-successful results counted.


### storage


- **`cove-cli storage growth`** - See which devices and customers are growing their backup storage fastest, from timestamped local snapshots.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`cove-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`cove-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`cove-cli learnings list`** - Inspect taught rows
- **`cove-cli learnings forget <query>`** - Undo a teach
- **`cove-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`cove-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`cove-cli teach-pattern`** - Install a query/resource template up front
- **`cove-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `COVE_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `cove-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
cove-cli audit

# JSON for scripting and agents
cove-cli audit --json
# Filter to specific fields
cove-cli audit --json --select PartnerName,UserName,EntityName

# Dry run  -  show the request without sending
cove-cli audit --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
cove-cli audit --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited or blocked, `10` config error.

## Health Check

```bash
cove-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Run `cove-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/cove-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\cove-cli\config.toml` on Windows; `--home`, `COVE_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `COVE_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `COVE_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **error 1701 "Visa is inconsistent/corrupted"**  -  Session expired or missing  -  run `cove-cli auth login` again; hand-built commands re-login automatically when credentials are in the environment
- **error 2100 "Unknown partner/username or bad password"**  -  COVE_PARTNER is required for API Users and must be the full customer/partner string exactly as shown in the Cove console customer dropdown (usually with the account email in parentheses); an empty or company-name-only COVE_PARTNER gives the same 2100 as a bad password. COVE_USERNAME is the API user login name and COVE_PASSWORD is its API token (Users > API Users)
- **storage growth / devices changes return empty or a single-snapshot note**  -  These commands diff local snapshots  -  run `cove-cli snapshot` at least twice, some time apart, before asking for trends
- **Generated endpoint commands (devices list, partners list) return an error envelope instead of data**  -  Raw endpoint commands need an explicit visa: pass --visa "$(cove-cli auth token)" or prefer the hand-built equivalents which authenticate automatically
- **error 13501 "Operation refused for security reasons" on a raw endpoint command**  -  Raw generated endpoint commands do not log in for you: pass --visa "$(cove-cli auth token)", or use `cove-cli call <Method>` or a hand-built command, which inject a fresh visa automatically

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**BackupNerd/Backup-Scripts**](https://github.com/BackupNerd/Backup-Scripts)  -  PowerShell
- [**impelling/CoveBackupApi**](https://github.com/impelling/CoveBackupApi)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
