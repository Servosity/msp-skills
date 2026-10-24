# Acronis Cyber Protect Cloud CLI

**The first real CLI for the Acronis Cyber Protect Cloud platform  -  every tenant, agent, and usage metric mirrored locally, with cross-tenant rollups no single API call returns.**

Manage your whole MSP estate from one Go binary: tenants, users, agents, offering items, usage, billing reports, tasks, and activities  -  all synced to a local SQLite store. Then answer questions the Acronis console can't: which customers' backups failed last night (health), which agents went silently offline (agents stale), and where you're billing for protection that isn't running (coverage --unprotected).

Learn more at [Acronis Cyber Protect Cloud](https://developer.acronis.com).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `acronis-cli` and `acronis-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.ps1 | iex
   ```
3. Verify: `acronis-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=acronis). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/acronis --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/acronis --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `acronis-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the acronis skill from https://github.com/Servosity/msp-skills/tree/main/skills/acronis. The skill defines how its required CLI (`acronis-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=acronis).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ACRONIS_BEARER_AUTH` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "acronis": {
      "command": "acronis-mcp",
      "env": {
        "ACRONIS_DATACENTER": "<datacenter>",
        "ACRONIS_BEARER_AUTH": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`acronis-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `ACRONIS_MCP_HTTP_TOKEN` is set, and every request must send `Authorization: Bearer <token>`. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`.

```bash
export ACRONIS_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
acronis-mcp --transport http
```

Set `ACRONIS_MCP_READ_ONLY=1` (exactly `1`) to remove every MCP tool that is not marked read-only. Without it, endpoint tools that delete or replace (DELETE, PUT) carry the MCP `destructiveHint` annotation, and those that create or trigger (POST, such as `agent-manager_force-agent-update` and `idp_revoke-token`) are marked not read-only but not destructive. The server asks for no confirmation on either: that step is your MCP host's.

## Authentication

Acronis uses OAuth2 client credentials. Register an API client in the Acronis console to get a client_id, client_secret, and your datacenter region, then run `acronis-cli auth login`  -  it exchanges them at /api/2/idp/token for a JWT (valid 2 hours) and stores it. Tokens last 2 hours; re-run `auth login` to refresh. The datacenter host is per-partner; set it with --datacenter or ACRONIS_DATACENTER (e.g. us-cloud, eu2-cloud). You can also skip login and provide a JWT directly via ACRONIS_BEARER_AUTH or `auth set-token`.

## Quick Start

```bash
# Exchange your API client credentials (flags or ACRONIS_CLIENT_ID/SECRET env) for a JWT and store it.
acronis-cli auth login --datacenter eu2-cloud

# Mirror tenants, agents, usages, and tasks into the local store.
acronis-cli sync

# See cross-tenant backup health  -  the headline rollup.
acronis-cli health --agent

# Find agents that have gone silently offline across every customer.
acronis-cli agents stale --older-than 7d

# Find tenants paying for protection that isn't actually running.
acronis-cli coverage --unprotected

```

## Unique Features

### Cross-tenant rollups
- **`health`**  -  See backup success / failure / stale across your entire book of customer tenants in one table.

  _Reach for this when an agent or tech needs the one-screen 'which customers' backups failed last night' answer the Acronis console can't give across tenants._

  ```bash
  acronis-cli health --agent
  ```
- **`agents stale`**  -  List backup agents that haven't checked in within a threshold, across every tenant, sorted by customer.

  _Use this to catch silently-offline agents before the customer calls  -  the most common MSP backup-failure root cause._

  ```bash
  acronis-cli agents stale --older-than 7d --agent
  ```
- **`alerts repeat`**  -  Rank resources and tenants by how many distinct days in a window had a failed or missed backup.

  _Use this to separate one-off backup blips from chronically-failing resources that need real remediation._

  ```bash
  acronis-cli alerts repeat --days 14 --agent
  ```
- **`failures`**  -  Flat list of every failed or missed backup task across all tenants in a recent window, newest first.

  _Reach for this when the question is 'show me each backup that failed last night' as individual actionable rows, not rollup counts._

  ```bash
  acronis-cli failures --since 24h --agent
  ```
- **`freshness`**  -  Time since the last successful backup per tenant, flagged against an SLA threshold  -  including tenants never backed up.

  _Reach for this when an agent needs SLA breach detection: which customers have gone too long without a good backup._

  ```bash
  acronis-cli freshness --sla 48h --breached --agent
  ```
- **`customer`**  -  One cross-resource snapshot of a single customer: tenant record, users, licenses, usage, agents, and 7-day backup outcomes joined.

  _Reach for this before a customer call: the full per-customer picture in one command instead of six console drill-downs._

  ```bash
  acronis-cli customer TENANT_ID --agent
  ```

### Billing & licensing
- **`reconcile usages`**  -  Flag usage with no matching offering item and offering items with zero usage, per tenant.

  _Reach for this at month-end to catch under-billing (usage with no SKU) and waste (paid SKUs with zero usage) before invoices go out._

  ```bash
  acronis-cli reconcile usages --tenant <tenant_id> --agent
  ```
- **`coverage`**  -  Surface tenants that pay for protection but have no online agent or no recent successful backup.

  _Use this to find the highest-liability customers  -  billed for backup, not actually protected._

  ```bash
  acronis-cli coverage --unprotected --agent
  ```
- **`usages drift`**  -  Compare per-tenant, per-metric usage between two stored snapshots to see what grew or shrank.

  _Reach for this to explain month-over-month invoice changes and spot runaway storage growth early._

  ```bash
  acronis-cli usages drift --from 2026-04-01 --to 2026-05-01 --agent
  ```
- **`tenants offering-items inventory`**  -  Estate-wide rollup of which offering items and editions are enabled, with per-SKU tenant counts.

  _Reach for this for license trueups and edition migrations: which SKUs are deployed where, in one table._

  ```bash
  acronis-cli tenants offering-items inventory --agent
  ```

### Fleet posture
- **`agents compliance`**  -  Show the distribution of agent versions across the estate and flag tenants behind the target version.

  _Use this after a release rollout to confirm every customer's agents updated, for security and support consistency._

  ```bash
  acronis-cli agents compliance --target 16.0 --agent
  ```
- **`tree`**  -  Render the Partner -> Customer -> Folder -> Unit hierarchy with per-node agent and user counts.

  _Reach for this to understand the shape of a partner's book of business at a glance._

  ```bash
  acronis-cli tree --depth 3
  ```
- **`tenants audit`**  -  Flag enabled customer tenants missing users, offering items, agents, or OAuth clients  -  onboarding drift in one table.

  _Reach for this after onboarding waves: catches half-provisioned tenants before they become missed-backup tickets._

  ```bash
  acronis-cli tenants audit --agent
  ```

## Recipes

### Monday morning fleet triage

```bash
acronis-cli sync && acronis-cli health --agent --select tenant_id,failed,stale
```

Sync the estate then narrow the health rollup to just the failing and stale columns per tenant.

### Offline-agent sweep into a ticket

```bash
acronis-cli agents stale --older-than 3d --json --select tenant_id,hostname,last_seen | jq '.[]'
```

List agents silent for 3+ days with only the fields a ticket needs.

### Month-end billing reconciliation

```bash
acronis-cli reconcile usages --tenant <tenant_id> --agent
```

Flag usage with no matching SKU and SKUs with zero usage before invoicing.

### Post-rollout version audit

```bash
acronis-cli agents compliance --target 16.0 --json
```

Confirm every tenant's agents reached the target version after a release.

### Estate shape at a glance

```bash
acronis-cli tree --depth 3
```

Render the partner/customer/folder/unit hierarchy with per-node agent and user counts.

## Usage

Run `acronis-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `ACRONIS_CONFIG_DIR`, `ACRONIS_DATA_DIR`, `ACRONIS_STATE_DIR`, or `ACRONIS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `ACRONIS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export ACRONIS_HOME=/srv/acronis
acronis-cli doctor
```

On Windows (PowerShell):

```powershell
$env:ACRONIS_HOME = "C:\ProgramData\acronis"
acronis-cli doctor
```

Under `ACRONIS_HOME=/srv/acronis`, the four dirs resolve to `/srv/acronis/config`, `/srv/acronis/data`, `/srv/acronis/state`, and `/srv/acronis/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:ACRONIS_HOME = "C:\srv\acronis"
acronis-cli doctor
```

Under `$env:ACRONIS_HOME = "C:\srv\acronis"`, the four dirs resolve to `C:\srv\acronis\config`, `C:\srv\acronis\data`, `C:\srv\acronis\state`, and `C:\srv\acronis\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "acronis": {
      "command": "acronis-mcp",
      "env": {
        "ACRONIS_HOME": "/srv/acronis"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `ACRONIS_DATA_DIR` overrides an explicit `--home` for that kind. Use `ACRONIS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `ACRONIS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `acronis-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode a successful read of a tenant, client, agent, task or activity is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory, in the table for the endpoint it came from. Other reads (hardware nodes, agent update settings) are not stored, and `--data-source local` refuses them. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `acronis-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `acronis-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `ACRONIS_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), no API response is written to the store, and `sync` and `workflow archive` refuse to run. The local learning commands (`teach` and its MCP tool) still record what you teach them. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `ACRONIS_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### agent-manager

Manage agent manager

- **`acronis-cli agent-manager delete-agent`** - Cancel registration of a specific Acronis agent.
- **`acronis-cli agent-manager delete-agents`** - Cancel registration and delete service accounts for multiple agents.
- **`acronis-cli agent-manager force-agent-update`** - Launch a forced agent update bypassing maintenance windows for specified agents.
- **`acronis-cli agent-manager get-agent`** - Retrieve details about a specific registered Acronis agent.
- **`acronis-cli agent-manager get-agent-update-settings`** - Fetch update configuration settings for agents or tenants.
- **`acronis-cli agent-manager get-hardware-node`** - Retrieve specific hardware node information including storage configuration.
- **`acronis-cli agent-manager list-agents`** - List all registered Acronis protection agents visible from a specified tenant.
- **`acronis-cli agent-manager list-hardware-nodes`** - List all hardware nodes visible from a specified tenant.
- **`acronis-cli agent-manager update-agent-update-settings`** - Store or update agent update configuration settings including maintenance windows.

### clients

OAuth2 client credential management

- **`acronis-cli clients create`** - Create a new OAuth2 client credential for API authentication.
- **`acronis-cli clients delete`** - Delete an OAuth2 client credential.
- **`acronis-cli clients get`** - Retrieve details about a specific OAuth2 client.
- **`acronis-cli clients list`** - List OAuth2 client credentials registered in the system.

### idp

Manage idp

- **`acronis-cli idp request-token`** - Request an OAuth2 access token using client credentials, authorization code, or other grant types.
- **`acronis-cli idp revoke-token`** - Revoke an OAuth2 access or refresh token.

### remote_search

Manage remote search

- **`acronis-cli remote-search`** - Search for tenants and users by name, email, or login across the accessible hierarchy.

### reports

Manage reports

- **`acronis-cli reports`** - Create a scheduled or on-demand usage report configuration.

### task-manager

Manage task manager

- **`acronis-cli task-manager get-activity`** - Retrieve details about a specific task activity by ID.
- **`acronis-cli task-manager get-task`** - Retrieve details about a specific backup or protection task by ID.
- **`acronis-cli task-manager list-activities`** - Fetch a list of task activities with filtering and pagination.
- **`acronis-cli task-manager list-tasks`** - Fetch a list of backup and protection tasks with filtering, ordering, and pagination support.

### tenants

Tenant hierarchy management and configuration

- **`acronis-cli tenants create`** - Create a new tenant as a child of an existing tenant.
- **`acronis-cli tenants delete`** - Delete a tenant. The tenant must have no children or active services.
- **`acronis-cli tenants get`** - Retrieve details about a specific tenant by ID.
- **`acronis-cli tenants list`** - List tenants in the hierarchy. Can filter by parent tenant UUID or retrieve by specific UUIDs.
- **`acronis-cli tenants update`** - Update tenant properties including name, contact, and enabled status.


### agents


- **`acronis-cli agents stale`** - List backup agents that haven't checked in within a threshold, across every tenant, sorted by customer.

- **`acronis-cli agents compliance`** - Show the distribution of agent versions across the estate and flag tenants behind the target version.


### alerts


- **`acronis-cli alerts repeat`** - Rank resources and tenants by how many distinct days in a window had a failed or missed backup.


### coverage


- **`acronis-cli coverage`** - Surface tenants that pay for protection but have no online agent or no recent successful backup.


### customer


- **`acronis-cli customer`** - One cross-resource snapshot of a single customer: tenant record, users, licenses, usage, agents, and 7-day backup outcomes joined.


### failures


- **`acronis-cli failures`** - Flat list of every failed or missed backup task across all tenants in a recent window, newest first.


### freshness


- **`acronis-cli freshness`** - Time since the last successful backup per tenant, flagged against an SLA threshold  -  including tenants never backed up.


### health


- **`acronis-cli health`** - See backup success / failure / stale across your entire book of customer tenants in one table.


### reconcile


- **`acronis-cli reconcile usages`** - Flag usage with no matching offering item and offering items with zero usage, per tenant.


### tenants


- **`acronis-cli tenants audit`** - Flag enabled customer tenants missing users, offering items, agents, or OAuth clients  -  onboarding drift in one table.

- **`acronis-cli tenants offering-items inventory`** - Estate-wide rollup of which offering items and editions are enabled, with per-SKU tenant counts.


### tree


- **`acronis-cli tree`** - Render the Partner -> Customer -> Folder -> Unit hierarchy with per-node agent and user counts.


### usages


- **`acronis-cli usages drift`** - Compare per-tenant, per-metric usage between two stored snapshots to see what grew or shrank.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`acronis-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`acronis-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`acronis-cli learnings list`** - Inspect taught rows
- **`acronis-cli learnings forget <query>`** - Undo a teach
- **`acronis-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`acronis-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`acronis-cli teach-pattern`** - Install a query/resource template up front
- **`acronis-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `ACRONIS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `acronis-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
acronis-cli clients list

# JSON for scripting and agents
acronis-cli clients list --json
# Filter to specific fields
acronis-cli clients list --json --select client_id,type,status

# Dry run  -  show the request without sending
acronis-cli clients list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
acronis-cli clients list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit), `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `ACRONIS_DATACENTER` resolves `{datacenter}`

Base URL: `https://{datacenter}.acronis.com`

## Health Check

```bash
acronis-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `acronis-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/acronis-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\acronis-cli\config.toml` on Windows; `--home`, `ACRONIS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `ACRONIS_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `ACRONIS_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ACRONIS_DATACENTER` | endpoint | Yes |  |
| `ACRONIS_BEARER_AUTH` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `acronis-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `acronis-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ACRONIS_BEARER_AUTH`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call right after login**  -  Wrong datacenter host. Confirm the region shown when you registered the API client and re-run `auth login --datacenter <region>` (e.g. us-cloud, eu2-cloud, au-cloud).
- **Calls start failing ~2 hours after login**  -  The JWT expired (tokens are valid for 2 hours). Re-run `acronis-cli auth login` to get a fresh token; there is no background auto-refresh.
- **`usages drift` returns nothing**  -  Drift needs at least two stored snapshots. Run `acronis-cli usages snapshot` on two different dates first (after a sync); drift compares the two snapshot dates.
- **`health` or `alerts repeat` looks empty**  -  Run `acronis-cli sync` to populate tasks/activities; these rollups read the local task store, not a live alerts endpoint.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**acronis-cyber-platform-python-examples**](https://github.com/acronis/acronis-cyber-platform-python-examples)  -  Python
- [**acronis-cyber-platform-powershell-examples**](https://github.com/acronis/acronis-cyber-platform-powershell-examples)  -  PowerShell
- [**acronis-cyber-platform-bash-examples**](https://github.com/acronis/acronis-cyber-platform-bash-examples)  -  Shell
- [**api-evangelist/acronis**](https://github.com/api-evangelist/acronis)  -  YAML

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
