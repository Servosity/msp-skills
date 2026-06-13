# Acronis Cyber Protect Cloud CLI

**The first real CLI for the Acronis Cyber Protect Cloud platform  -  every tenant, agent, and usage metric mirrored locally, with cross-tenant rollups no single API call returns.**

Manage your whole MSP estate from one Go binary: tenants, users, agents, offering items, usage, billing reports, tasks, and activities  -  all synced to a local SQLite store. Then answer questions the Acronis console can't: which customers' backups failed last night (health), which agents went silently offline (agents stale), and where you're billing for protection that isn't running (coverage --unprotected).

Learn more at [Acronis Cyber Protect Cloud](https://developer.acronis.com).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `acronis-cli` binary and the `pp-acronis` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install acronis
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install acronis --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install acronis --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install acronis --agent claude-code
npx -y @mvanhorn/printing-press-library install acronis --agent claude-code --agent codex
```

### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/acronis-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install acronis --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-acronis --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-acronis --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install acronis --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/acronis-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ACRONIS_BEARER_AUTH` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

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

## Authentication

Acronis uses OAuth2 client credentials. Register an API client in the Acronis console to get a client_id, client_secret, and your datacenter region, then run `acronis-cli auth login`  -  it exchanges them at /api/2/idp/token for a JWT (valid 2 hours) and stores it. Tokens last 2 hours; re-run `auth login` to refresh. The datacenter host is per-partner; set it with --datacenter or ACRONIS_DATACENTER (e.g. us-cloud, eu2-cloud). You can also skip login and provide a JWT directly via ACRONIS_CYBER_PROTECT_BEARER_AUTH or `auth set-token`.

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

These capabilities aren't available in any other tool for this API.

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


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
acronis-cli clients list

# JSON for scripting and agents
acronis-cli clients list --json

# Filter to specific fields
acronis-cli clients list --json --select id,name,status

# Dry run  -  show the request without sending
acronis-cli clients list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
acronis-cli clients list --agent
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

Config file: `~/.config/acronis-cyber-protect-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

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
