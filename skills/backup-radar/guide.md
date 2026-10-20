# Backup Radar CLI

**Every Backup Radar v3 and v2 endpoint plus a local store that finds the backups nobody is watching.**

Covers the ScalePad Backup Radar API v3 (client health, devices, ingest, retire) and the read-only v2 API (backups, results, inactive, retired, overview) in one binary. Sync both into SQLite, then run check for the morning triage, unwatched for blind spots, coverage diff against your backup vendor's device list, and scorecard for client QBRs.

Learn more at [Backup Radar](https://www.scalepad.com/backup-radar).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `backup-radar-cli` and `backup-radar-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.ps1 | iex
   ```
3. Verify: `backup-radar-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=backup-radar). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/backup-radar --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/backup-radar --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `backup-radar-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the backup-radar skill from https://github.com/Servosity/msp-skills/tree/main/skills/backup-radar. The skill defines how its required CLI (`backup-radar-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download `backup-radar-mcp.mcpb` (one bundle for macOS, Windows and Linux) from the [latest release](https://github.com/Servosity/msp-skills/releases?q=backup-radar).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `BACKUP_RADAR_API_KEY` (v3 commands) and/or `BACKUP_RADAR_V2_API_KEY` (v2 commands) when Claude Desktop prompts you; leave the one you do not use blank.

Requires Claude Desktop 1.0.0 or later. The one `.mcpb` bundle carries macOS (Intel and Apple Silicon), Windows x64 (Windows on ARM runs it under emulation) and Linux x64 binaries; for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "backup-radar": {
      "command": "backup-radar-mcp",
      "env": {
        "BACKUP_RADAR_API_KEY": "<your-scalepad-key>",
        "BACKUP_RADAR_V2_API_KEY": "<your-backup-radar-v2-key>"
      }
    }
  }
}
```

</details>

## Authentication

Backup Radar has two API generations with two different keys. The v3 commands (health, devices, backups ingest, backups retire, push) use a ScalePad API key from the ScalePad API tab: set BACKUP_RADAR_API_KEY (`export` on macOS / Linux, `$env:BACKUP_RADAR_API_KEY = "..."` on Windows) or store it with auth set-token. The v2 commands (backups list, get, results, filters, inactive, retired, overview, and timeline --live) use a separate Backup Radar API key that is read only from the environment: set BACKUP_RADAR_V2_API_KEY the same way. Either key alone works for its own commands, and the local reports use whatever data you synced. doctor probes each configured key against its API (GET /clients/health with the v3 key, GET /backups/overview with the v2 key) and reports valid, rejected or not verified per key. EU tenants set BACKUP_RADAR_BASE_URL=https://eu.api.scalepad.com/backup-radar/v3; that override applies to v3 commands only, because v2 always uses https://api.backupradar.com.

## Quick Start

```bash
# Probe each configured key against its API and report valid, rejected or not verified per key
backup-radar-cli doctor

# Pull devices, client health and backups into the local store
backup-radar-cli sync

# Today's failing, silent and at-risk backups, ranked
backup-radar-cli check --include-at-risk

# Backups nobody is watching: inactive, unmapped, ticketless
backup-radar-cli unwatched

# Rank clients by backup compliance for the quarter
backup-radar-cli scorecard --days 90

```

## Unique Features

### Daily triage and reporting
- **`check`**  -  One ranked list of today's failing, silent and at-risk backups, each marked new or ongoing with days in status and open ticket count; uses whichever keys you synced.

  _Reach for this first in a daily backup review instead of paging raw device or backup lists._

  ```bash
  backup-radar-cli check --include-at-risk --agent
  ```
- **`scorecard`**  -  In-compliance rate, weekly trend and worst devices for one client, or every client ranked, from up to a year of client health (v3 key); worst devices use synced device history (each sync fetches 90 days; stored days accumulate up to a year).

  _Use it for QBR backup slides and client health rankings._

  ```bash
  backup-radar-cli scorecard "Acme Dental" --days 90 --agent
  ```
- **`timeline`**  -  One device's day-by-day backup story merged from both API generations, with tickets, tags and notes; --live adds each day's v2 results (v2 key).

  _Use it to explain a single device's failures before a client call._

  ```bash
  backup-radar-cli timeline "DC01" --job "Nightly Image" --days 30 --agent
  ```

### Find what nobody is watching
- **`unwatched`**  -  Every backup Backup Radar receives but nobody acts on: inactive, unmapped to ScalePad, failing with no ticket, no PSA company, or warnings counted as success by policy. Unmapped needs the v3 key; the other reasons need the v2 key.

  _Use it to answer which backups have no human or ticket behind them before a client finds out._

  ```bash
  backup-radar-cli unwatched --reason inactive,ticketless --agent
  ```
- **`coverage diff`**  -  Compare the devices a backup vendor protects against what Backup Radar monitors and list the gaps in both directions.

  _Use it when a vendor console and Backup Radar disagree about what is protected and monitored._

  ```bash
  backup-radar-cli coverage diff --devices vendor-devices.json --name-field hostname --agent
  ```
- **`stale`**  -  Backups with no result in N days, grouped by client, with retire request bodies ready to preview; works from either key's synced data.

  _Use it to plan a cleanup before calling backups retire, never instead of the server-side dry run._

  ```bash
  backup-radar-cli stale --days 30 --agent
  ```

### Custom ingest
- **`push`**  -  Push flat CSV or JSONL backup results into Backup Radar (v3 key, live write) with local validation and automatic batching under the 10,000-result limit.

  _Use it to report results from backup tools Backup Radar cannot parse, from any script or scheduler._

  ```bash
  backup-radar-cli push --from results.csv --dry-run
  ```

## Recipes

### Morning triage for agents

```bash
backup-radar-cli check --include-at-risk --agent
```

Ranked failing, silent and at-risk backups with new or ongoing state, ready for an agent to open tickets.

### Narrow the device list

```bash
backup-radar-cli devices --all --agent --select br_device_name,client.br_company_name,br_job_name,last_result
```

Device rows carry long history arrays; select only the fields you need (the list wrapper is already removed).

### Find blind spots

```bash
backup-radar-cli unwatched --reason inactive,unmapped,ticketless --agent
```

One row per backup and reason, plus counts and any reasons skipped because their collection was never synced.

### Plan a cleanup

```bash
backup-radar-cli stale --days 45 --agent
```

Silent backups grouped by client. Add --bodies to print only the retire bodies, then preview one with: stale --days 45 --bodies | head -n 1 | backups retire --stdin (PowerShell: stale --days 45 --bodies | Select-Object -First 1 | backups retire --stdin). Every body has dry_run true.

### Vendor coverage gap

```bash
backup-radar-cli coverage diff --devices vendor-devices.json --name-field hostname --agent
```

Devices the vendor protects that Backup Radar does not monitor, and the reverse.

## Usage

Run `backup-radar-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `BACKUP_RADAR_CONFIG_DIR`, `BACKUP_RADAR_DATA_DIR`, `BACKUP_RADAR_STATE_DIR`, or `BACKUP_RADAR_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `BACKUP_RADAR_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export BACKUP_RADAR_HOME=/srv/backup-radar      # macOS / Linux
backup-radar-cli doctor
```

```powershell
$env:BACKUP_RADAR_HOME = "D:\backup-radar"      # Windows
backup-radar-cli doctor
```

On Windows (PowerShell):

```powershell
$env:BACKUP_RADAR_HOME = "C:\ProgramData\backup-radar"
backup-radar-cli doctor
```

Under `BACKUP_RADAR_HOME=/srv/backup-radar`, the four dirs resolve to `/srv/backup-radar/config`, `/srv/backup-radar/data`, `/srv/backup-radar/state`, and `/srv/backup-radar/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "backup-radar": {
      "command": "backup-radar-mcp",
      "env": {
        "BACKUP_RADAR_HOME": "/srv/backup-radar"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `BACKUP_RADAR_DATA_DIR` overrides an explicit `--home` for that kind. Use `BACKUP_RADAR_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `BACKUP_RADAR_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `backup-radar-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### backups

Ingest backup results and retire unused backups (v3 key); list, inspect, filter and count backups (v2 key).

- **`backup-radar-cli backups filters`** - Returns the available filters, such as tags, companies, statuses, and more (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).
- **`backup-radar-cli backups get`** - Returns the backup data for a specific day (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).
- **`backup-radar-cli backups inactive`** - Returns inactive backups that are ready to be activated (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).
- **`backup-radar-cli backups ingest`** - Accepts a root JSON array of backup entries (v3 key). Each entry identifies a device, job, and client, and may include result collections for backups, replications, verifications, boot verifications, and alerts.

The entire batch is rejected when any entry fails validation. The total number of results across all collections in a request must not exceed 10,000.

A successful request is acknowledged with an empty `202 Accepted` response. Accepted results are queued for asynchronous processing by the backup results saver pipeline. Uses the ScalePad API key (BACKUP_RADAR_API_KEY).
- **`backup-radar-cli backups list`** - Returns scheduled backups that match the specified filters and are paginated accordingly (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).
- **`backup-radar-cli backups overview`** - Returns the counts of policies, backups, servers, and workstations (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).
- **`backup-radar-cli backups results`** - Returns all backup results for the given date (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).
- **`backup-radar-cli backups retire`** - Retires backups that match the supplied filters (v3 key). Filters are exact (case-insensitive) and combined with AND.
`reason` is required. At least one filter is required.

Set `dry_run` to `true` to preview every match without retiring anything.

A live request retires every match and returns the full retired set.

Tenant-level API keys are allowed. A user-tied key must belong to a tenant administrator or a user with the
Retire Backup permission. Uses the ScalePad API key (BACKUP_RADAR_API_KEY).
- **`backup-radar-cli backups retired`** - Returns backups that have been retired (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).

### devices

Get the client's backup devices.

- **`backup-radar-cli devices`** - Returns a cursor-paginated list of backup devices whose names match the optional filters (v3 key). Each item includes the device's last result and per-day SLA history for the requested window, ending on the tenant's current local date.

Devices are returned whether or not their Backup Radar company is mapped to a ScalePad company. When a device is not mapped, the ScalePad-specific fields (`device_id`, `device_name`, and the `client` `name`/`client_id`) are null. Uses the ScalePad API key (BACKUP_RADAR_API_KEY).

### health

Get the client's backup health.

- **`backup-radar-cli health get`** - Returns backup health for one client (v3 key).

Identify the client with the **ScalePad client ID** (`client_id` in list responses). Uses the ScalePad API key (BACKUP_RADAR_API_KEY).
- **`backup-radar-cli health list`** - Returns a cursor-paginated list of clients whose backup health matches the optional filters (v3 key). Each item includes history for the last seven calendar days, ending on the tenant's current local date. Uses the ScalePad API key (BACKUP_RADAR_API_KEY).


### check


- **`backup-radar-cli check`** - One ranked list of today's failing, silent and at-risk backups, each marked new or ongoing with days in status and open ticket count; uses whichever keys you synced.


### coverage


- **`backup-radar-cli coverage diff`** - Compare the devices a backup vendor protects against what Backup Radar monitors and list the gaps in both directions.


### push


- **`backup-radar-cli push`** - Push flat CSV or JSONL backup results into Backup Radar (v3 key, live write) with local validation and automatic batching under the 10,000-result limit.


### scorecard


- **`backup-radar-cli scorecard`** - In-compliance rate, weekly trend and worst devices for one client, or every client ranked, from up to a year of client health (v3 key); worst devices use synced device history (each sync fetches 90 days; stored days accumulate up to a year).


### stale


- **`backup-radar-cli stale`** - Backups with no result in N days, grouped by client, with retire request bodies ready to preview; works from either key's synced data.


### timeline


- **`backup-radar-cli timeline`** - One device's day-by-day backup story merged from both API generations, with tickets, tags and notes; --live adds each day's v2 results (v2 key).


### unwatched


- **`backup-radar-cli unwatched`** - Every backup Backup Radar receives but nobody acts on: inactive, unmapped to ScalePad, failing with no ticket, no PSA company, or warnings counted as success by policy. Unmapped needs the v3 key; the other reasons need the v2 key.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`backup-radar-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`backup-radar-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`backup-radar-cli learnings list`** - Inspect taught rows
- **`backup-radar-cli learnings forget <query>`** - Undo a teach
- **`backup-radar-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`backup-radar-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`backup-radar-cli teach-pattern`** - Install a query/resource template up front
- **`backup-radar-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `BACKUP_RADAR_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `backup-radar-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
backup-radar-cli backups list

# JSON for scripting and agents
backup-radar-cli backups list --json
# Filter to specific fields
backup-radar-cli backups list --json --select backupId,backupType,companyName

# Dry run  -  show the request without sending
backup-radar-cli backups list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
backup-radar-cli backups list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending; `backups retire --dry-run` is the exception: it is the API's own preview flag, so the request is sent with `dry_run: true` (and with `--stdin` it sets `dry_run` in the body)
- **Live writes** - `backups ingest`, `backups retire` (without `--dry-run`) and `push` are live writes that run immediately and never prompt; preview with `push --dry-run` or `backups retire --dry-run` first
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` other error, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `BACKUP_RADAR_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `backup-radar-cli backups list`
- `backup-radar-cli backups get`
- `backup-radar-cli backups filters`
- `backup-radar-cli backups inactive`
- `backup-radar-cli backups retired`
- `backup-radar-cli devices`
- `backup-radar-cli health list`
- `backup-radar-cli health get`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
backup-radar-cli doctor
```

Checks configuration and paths, then probes each configured key against its API (`GET /clients/health` with the ScalePad v3 key, `GET /backups/overview` with the Backup Radar v2 key) and reports `valid`, `rejected` or `not verified` per key (`credentials` and `credentials_v2`). Either key alone is a configured install; with neither set it says so and names both variables.

## Configuration

Run `backup-radar-cli doctor` to see the resolved config, data, state, and cache directories. The local store is scoped to the credentials that filled it (`data-<hash>.db` next to `data.db`; the hash covers the v3 key and, when set, the v2 key), so run the analysis commands with the same `BACKUP_RADAR_API_KEY` (environment or `auth set-token`) and the same `BACKUP_RADAR_V2_API_KEY` setting that `sync` used; a shell without them looks for a different store file and reports `no local mirror at <path>` with the `sync --db` command to run (or `nothing synced yet` when the file exists but is empty). The platform-default config path is `~/.config/backup-radar-cli/config.toml` (`%USERPROFILE%\.config\backup-radar-cli\config.toml` on Windows); `--home`, `BACKUP_RADAR_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `BACKUP_RADAR_API_KEY` | per_call | One of the two keys | ScalePad API key for v3 commands (health, devices, backups ingest/retire, push). |
| `BACKUP_RADAR_V2_API_KEY` | per_call | One of the two keys | Backup Radar API v2 key for v2 commands (backups list/get/results/filters/inactive/retired/overview, timeline --live). Environment only. |
| `BACKUP_RADAR_BASE_URL` | config | No | v3 base URL override, e.g. https://eu.api.scalepad.com/backup-radar/v3 for EU tenants. Does not affect v2. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `backup-radar-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `backup-radar-cli doctor` to check credentials
- Verify the environment variable is set: `echo $BACKUP_RADAR_API_KEY` (macOS / Linux) or `$env:BACKUP_RADAR_API_KEY` (Windows PowerShell) (v3 commands)
- For v2 commands: `echo $BACKUP_RADAR_V2_API_KEY` (macOS / Linux) or `$env:BACKUP_RADAR_V2_API_KEY` (Windows PowerShell); this key is read only from the environment (`auth set-token` stores only the v3 key)
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 No API credentials on health, devices, backups ingest or backups retire**  -  set BACKUP_RADAR_API_KEY (`export BACKUP_RADAR_API_KEY=<key>` on macOS / Linux, `$env:BACKUP_RADAR_API_KEY = "<key>"` on Windows) to a ScalePad API key from the ScalePad API tab, or save it with `backup-radar-cli auth set-token`
- **"this command uses the Backup Radar API v2 key, BACKUP_RADAR_V2_API_KEY, which is not set" (exit 4) on backups list or another v2 command**  -  set BACKUP_RADAR_V2_API_KEY (`export BACKUP_RADAR_V2_API_KEY=<key>` on macOS / Linux, `$env:BACKUP_RADAR_V2_API_KEY = "<key>"` on Windows) to a Backup Radar API key; this key is read only from the environment
- **402 PRODUCT_SUBSCRIPTION_REQUIRED**  -  The ScalePad tenant has no active Backup Radar subscription; use a key from the tenant that owns Backup Radar
- **EU tenant gets 401 on v3 commands with a valid key**  -  set BACKUP_RADAR_BASE_URL to https://eu.api.scalepad.com/backup-radar/v3 (`export` on macOS / Linux, `$env:BACKUP_RADAR_BASE_URL = "..."` on Windows; v3 commands only, v2 always uses https://api.backupradar.com)
- **429 Too Many on v2 commands**  -  Lower the pace with --rate-limit 1 (v2 allows one request every 0.5 seconds)
- **unwatched lists a reason under skipped**  -  That collection was never synced; set the key it names and run backup-radar-cli sync

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**scalepad-mcp**](https://github.com/wyre-technology/scalepad-mcp)  -  TypeScript
- [**br-ps**](https://github.com/chrisdaysie/br-ps)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
