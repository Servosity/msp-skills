# Nerdio Manager CLI

**The first non-PowerShell client for the Nerdio Manager for MSP API - cross-account AVD fleet audits, async-job plumbing, and offline search no other Nerdio tool has.**

Manage Azure Virtual Desktop fleets across every customer account from one terminal: audit autoscale posture fleet-wide with `fleet autoscale-audit`, sweep session-host power state with `fleet host-estate`, reconcile billing with `fleet billing-rollup`, and tame NMM's async job model with `job wait`. Includes a local SQLite store with full-text search over accounts, profiles, and scripted actions - the only offline Nerdio client in existence.

## Install

The recommended path installs both the `nerdio-cli` binary and the `pp-nerdio` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install nerdio
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install nerdio --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install nerdio --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install nerdio --agent claude-code
npx -y @mvanhorn/printing-press-library install nerdio --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/cloud/nerdio/cmd/nerdio-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/nerdio-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install nerdio --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-nerdio --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-nerdio --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install nerdio --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

The bundle reuses your local OAuth tokens  -  authenticate first if you haven't:

```bash
nerdio-cli auth login
```

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/nerdio-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `NERDIO_CLIENT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/cloud/nerdio/cmd/nerdio-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "nerdio": {
      "command": "nerdio-mcp",
      "env": {
        "NERDIO_CLIENT_ID": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

The NMM Partner API is per-instance: every MSP hosts their own Nerdio Manager installation, so there is no vendor-global endpoint. Create an API client in your NMM portal (Settings -> Integrations -> REST API), then set five environment variables: NERDIO_BASE_URL (your instance root, e.g. https://nmm.contoso.com), NERDIO_TOKEN_URL (https://login.microsoftonline.com/<TENANT_ID>/oauth2/v2.0/token), NERDIO_CLIENT_ID, NERDIO_CLIENT_SECRET, and NERDIO_OAUTH_SCOPE. The scope is the bare Application ID URI default - '<app-id>/.default' with NO api:// prefix. Adding api:// triggers AADSTS500011; Nerdio's own docs got this wrong for years.

## Quick Start

```bash
# Verify config, client credentials, and instance reachability before anything else
nerdio-cli doctor --dry-run

# List every customer account on your NMM instance - account IDs feed every other command
nerdio-cli accounts --agent

# Snapshot accounts into the local SQLite store so fleet commands and search work offline
nerdio-cli sync --resources accounts

# The Monday sweep: find every host pool fleet-wide with autoscale disabled or off-baseline
nerdio-cli fleet autoscale-audit --agent

# Block until an async mutation's job reaches Completed/Failed - the loop every NMM script otherwise rewrites
nerdio-cli job wait 4821 --interval 10s

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Async job plumbing
- **`job wait`**  -  Wait for any NMM async job to finish - polls until Completed/Failed/Cancelled and exits with a typed code reflecting the outcome.

  _Run this after any mutation (provisioning, scripted actions, backup) instead of hand-writing a polling loop._

  ```bash
  nerdio-cli job wait 4821 --interval 10s --timeout 30m --agent
  ```

### Cross-account fleet ops
- **`fleet autoscale-audit`**  -  See every host pool across all customer accounts whose autoscale is disabled or diverges from your baseline - the Monday cost-control sweep as one command.

  _Use this for cross-customer autoscale posture instead of scripting foreach loops over per-account API calls._

  ```bash
  nerdio-cli fleet autoscale-audit --accounts 101,102 --agent
  ```
- **`fleet host-estate`**  -  One table of every session host across all customer accounts with pool, account, and power state - the weekend power-sweep view.

  _Use this to answer 'what is running right now across all customers' in one call._

  ```bash
  nerdio-cli fleet host-estate --running-only --agent --select items.account,items.host,items.power_state
  ```
- **`scripted-actions fan-run`**  -  Execute one scripted action across many customer accounts, collect every returned job ID, and optionally wait for all of them to finish.

  _Use this to push one operation fleet-wide instead of hand-looping account IDs in PowerShell._

  ```bash
  nerdio-cli scripted-actions fan-run 42 --accounts 101,102,103 --wait
  ```

### Billing intelligence
- **`fleet billing-rollup`**  -  Per-account billed/paid/unpaid/usage rollup for a billing period, joined to account names - PSA-reconciliation-ready without Excel.

  _Use this for the weekly unpaid-invoice check and month-end reconciliation export._

  ```bash
  nerdio-cli fleet billing-rollup --period 2026-05-01:2026-05-31 --unpaid-only --agent
  ```
- **`usages drift`**  -  Flag customer accounts whose consumption grew or shrank beyond a threshold between two periods.

  _Use this before invoicing to catch consumption surprises early._

  ```bash
  nerdio-cli usages drift --from 2026-04-01:2026-04-30 --to 2026-05-01:2026-05-31 --min-pct 20 --agent
  ```

## Recipes


### Monday autoscale sweep

```bash
nerdio-cli fleet autoscale-audit --agent
```

Fan out across every customer account and flag host pools with autoscale disabled or diverging from baseline, with failed accounts reported separately.

### Weekend power check

```bash
nerdio-cli fleet host-estate --running-only --agent --select items.account,items.host,items.power_state
```

One narrow table of every session host still running across all customers - the deep response trimmed to three fields.

### Unpaid invoice check

```bash
nerdio-cli fleet billing-rollup --period 2026-05-01:2026-05-31 --unpaid-only
```

Per-account unpaid balance for the period, joined to account names from the local store.

### Fleet-wide scripted action

```bash
nerdio-cli scripted-actions fan-run 42 --accounts 101,102,103 --wait
```

Run one scripted action in three customer accounts and block until every returned job reaches a terminal state.

### Find an account fast

```bash
nerdio-cli search "contoso" --type accounts
```

Full-text search the synced local store instead of paging the live API.

## Usage

Run `nerdio-cli --help` for the full command reference and flag list.

## Commands

### accounts

MSP customer accounts managed by this NMM installation

- **`nerdio-cli accounts`** - List all customer accounts

### app-roles

NMM application roles and assignments

- **`nerdio-cli app-roles assignments`** - List app role assignments
- **`nerdio-cli app-roles roles`** - List available app roles

### autoscale-profiles

Reusable autoscale profiles

- **`nerdio-cli autoscale-profiles account-get`** - Get an account autoscale profile
- **`nerdio-cli autoscale-profiles account-list`** - List autoscale profiles for an account
- **`nerdio-cli autoscale-profiles get`** - Get an MSP-level autoscale profile
- **`nerdio-cli autoscale-profiles list`** - List MSP-level autoscale profiles

### backup

Azure Backup operations for a customer account

- **`nerdio-cli backup disable`** - Disable backup for a protected item
- **`nerdio-cli backup enable`** - Enable backup for a resource with a policy
- **`nerdio-cli backup protected-items`** - List protected items for an account
- **`nerdio-cli backup recovery-points`** - List recovery points for a protected item
- **`nerdio-cli backup restore`** - Restore a protected item from a recovery point
- **`nerdio-cli backup run`** - Trigger an on-demand backup of a protected item

### cost-estimator

Azure cost estimates built in NMM

- **`nerdio-cli cost-estimator get`** - Get a cost estimate by ID
- **`nerdio-cli cost-estimator list`** - List saved cost estimates

### desktop-images

Golden desktop images managed by NMM

- **`nerdio-cli desktop-images changelog`** - Get desktop image change log
- **`nerdio-cli desktop-images get`** - Get desktop image details
- **`nerdio-cli desktop-images list`** - List desktop images for an account
- **`nerdio-cli desktop-images schedules`** - Get desktop image schedule configurations
- **`nerdio-cli desktop-images start`** - Start (power on) a desktop image VM
- **`nerdio-cli desktop-images stop`** - Stop (power off) a desktop image VM

### devices

Intune-managed devices (v1-beta API)

- **`nerdio-cli devices app-failures`** - List app installation failures on a device
- **`nerdio-cli devices apps`** - List apps installed on a device
- **`nerdio-cli devices bitlocker-keys`** - Get BitLocker recovery keys for a device
- **`nerdio-cli devices compliance`** - Get compliance state for a device
- **`nerdio-cli devices get`** - Get an Intune device by ID
- **`nerdio-cli devices hardware`** - Get hardware inventory for a device
- **`nerdio-cli devices laps`** - Get local admin password (LAPS) for a device
- **`nerdio-cli devices list`** - List Intune devices for an account
- **`nerdio-cli devices sync`** - Trigger an Intune sync on a device

### directories

Active Directory configurations

- **`nerdio-cli directories account`** - List directory configurations for an account
- **`nerdio-cli directories list`** - List MSP-level directory configurations

### environment-variables

Environment variables for scripted actions

- **`nerdio-cli environment-variables account`** - List environment variables for an account
- **`nerdio-cli environment-variables list`** - List MSP-level environment variables

### fslogix

FSLogix profile storage configurations

- **`nerdio-cli fslogix <account_id>`** - List FSLogix configurations for an account

### groups

Entra ID groups within a customer account

- **`nerdio-cli groups <account_id> <group_id>`** - Get a group by ID

### host-pools

AVD host pools within a customer account

- **`nerdio-cli host-pools ad`** - Get host pool Active Directory settings
- **`nerdio-cli host-pools assigned-users`** - List users assigned to a host pool
- **`nerdio-cli host-pools autoscale`** - Get host pool autoscale configuration
- **`nerdio-cli host-pools avd`** - Get host pool AVD settings
- **`nerdio-cli host-pools create`** - Create a host pool in an account
- **`nerdio-cli host-pools delete`** - Delete a host pool
- **`nerdio-cli host-pools fslogix`** - Get host pool FSLogix configuration
- **`nerdio-cli host-pools list`** - List host pools for an account
- **`nerdio-cli host-pools rdp`** - Get host pool RDP settings
- **`nerdio-cli host-pools schedules`** - Get host pool schedule configurations
- **`nerdio-cli host-pools session-timeouts`** - Get host pool session timeout settings
- **`nerdio-cli host-pools sessions`** - List active user sessions on a host pool
- **`nerdio-cli host-pools set-autoscale`** - Update host pool autoscale configuration (pass full config JSON via --stdin)
- **`nerdio-cli host-pools tags`** - Get host pool Azure tags
- **`nerdio-cli host-pools vm-deployment`** - Get host pool VM deployment settings

### hosts

Session hosts within a host pool

- **`nerdio-cli hosts list`** - List session hosts in a host pool
- **`nerdio-cli hosts restart`** - Restart a session host VM
- **`nerdio-cli hosts schedules`** - Get schedule configurations for a session host
- **`nerdio-cli hosts start`** - Start a session host VM
- **`nerdio-cli hosts stop`** - Stop (deallocate) a session host VM

### invoices

MSP billing invoices

- **`nerdio-cli invoices get`** - Get an invoice by ID
- **`nerdio-cli invoices list`** - List invoices in a billing period

### job

Async jobs returned by NMM mutations

- **`nerdio-cli job get`** - Get an async job by ID
- **`nerdio-cli job retry`** - Restart a failed job
- **`nerdio-cli job tasks`** - List tasks of an async job

### networks

Azure virtual networks for a customer account

- **`nerdio-cli networks all`** - List all networks visible to an account
- **`nerdio-cli networks link`** - Link an existing network to an account
- **`nerdio-cli networks list`** - List networks managed by NMM for an account

### provisioning

Customer account provisioning operations

- **`nerdio-cli provisioning link-network`** - Link a network during account provisioning
- **`nerdio-cli provisioning link-tenant`** - Link an existing Entra tenant as a new NMM account (returns a job; poll jobs get)

### recovery-vaults

Azure Recovery Services vaults for a customer account

- **`nerdio-cli recovery-vaults all`** - List all recovery vaults visible to an account
- **`nerdio-cli recovery-vaults create`** - Create a recovery vault
- **`nerdio-cli recovery-vaults delete-policy`** - Delete a backup policy
- **`nerdio-cli recovery-vaults link`** - Link an existing recovery vault to an account
- **`nerdio-cli recovery-vaults linked`** - List recovery vaults linked to an account
- **`nerdio-cli recovery-vaults policies`** - List backup policies in a recovery vault
- **`nerdio-cli recovery-vaults policy`** - Get a backup policy by name
- **`nerdio-cli recovery-vaults region-policies`** - Get vault policy info for an Azure region
- **`nerdio-cli recovery-vaults unlink`** - Unlink a recovery vault from an account

### reservations

Azure VM reserved instances for a customer account

- **`nerdio-cli reservations create`** - Create a reservation
- **`nerdio-cli reservations delete`** - Delete a reservation
- **`nerdio-cli reservations get`** - Get a reservation by ID
- **`nerdio-cli reservations list`** - List reservations for an account
- **`nerdio-cli reservations resources`** - List resources attached to a reservation
- **`nerdio-cli reservations update`** - Update a reservation

### resource-groups

Azure resource groups linked to NMM

- **`nerdio-cli resource-groups account-link`** - Link a resource group to an account
- **`nerdio-cli resource-groups account-list`** - List resource groups linked to an account
- **`nerdio-cli resource-groups account-set-default`** - Set the default resource group for an account
- **`nerdio-cli resource-groups account-unlink`** - Unlink a resource group from an account
- **`nerdio-cli resource-groups link`** - Link a resource group at MSP level
- **`nerdio-cli resource-groups list`** - List MSP-level linked resource groups
- **`nerdio-cli resource-groups set-default`** - Set the default MSP-level resource group
- **`nerdio-cli resource-groups unlink`** - Unlink an MSP-level resource group

### schedules

Reusable schedules

- **`nerdio-cli schedules account-configurations`** - Get configurations for an account schedule
- **`nerdio-cli schedules account-get`** - Get an account schedule
- **`nerdio-cli schedules account-list`** - List schedules for an account
- **`nerdio-cli schedules configurations`** - Get configurations for an MSP-level schedule
- **`nerdio-cli schedules get`** - Get an MSP-level schedule
- **`nerdio-cli schedules list`** - List MSP-level schedules

### scripted-actions

Scripted actions (MSP-level and per-account)

- **`nerdio-cli scripted-actions account-list`** - List scripted actions for an account
- **`nerdio-cli scripted-actions list`** - List MSP-level scripted actions
- **`nerdio-cli scripted-actions run`** - Execute an MSP-level scripted action (returns a job)
- **`nerdio-cli scripted-actions run-account`** - Execute a scripted action in an account context (returns a job)
- **`nerdio-cli scripted-actions schedule`** - Get the schedule for an account scripted action
- **`nerdio-cli scripted-actions unschedule`** - Remove the schedule from an account scripted action

### secure-variables

Secure variables for scripted actions (values are secrets)

- **`nerdio-cli secure-variables account-create`** - Create a secure variable in an account
- **`nerdio-cli secure-variables account-delete`** - Delete a secure variable from an account
- **`nerdio-cli secure-variables account-list`** - List secure variables for an account (may expose stored secret values)
- **`nerdio-cli secure-variables account-update`** - Update a secure variable in an account
- **`nerdio-cli secure-variables create`** - Create an MSP-level secure variable
- **`nerdio-cli secure-variables delete`** - Delete an MSP-level secure variable
- **`nerdio-cli secure-variables list`** - List MSP-level secure variables (may expose stored secret values)
- **`nerdio-cli secure-variables update`** - Update an MSP-level secure variable

### usages

Consumption/usage data

- **`nerdio-cli usages account`** - Get usage for one customer account between dates
- **`nerdio-cli usages msp`** - Get MSP-level usage between dates

### users

Entra ID users within a customer account

- **`nerdio-cli users get`** - Get a user by ID
- **`nerdio-cli users mfa`** - Get MFA registration status for a user
- **`nerdio-cli users search`** - Search/list users in an account (paginated POST search)

### workspaces

AVD workspaces for a customer account

- **`nerdio-cli workspaces create`** - Create an AVD workspace
- **`nerdio-cli workspaces list`** - List AVD workspaces for an account
- **`nerdio-cli workspaces sessions`** - List sessions in a workspace


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
nerdio-cli accounts

# JSON for scripting and agents
nerdio-cli accounts --json

# Filter to specific fields
nerdio-cli accounts --json --select id,name,status

# Dry run  -  show the request without sending
nerdio-cli accounts --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
nerdio-cli accounts --agent
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

Set `NERDIO_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `nerdio-cli accounts`
- `nerdio-cli autoscale-profiles`
- `nerdio-cli autoscale-profiles get`
- `nerdio-cli autoscale-profiles list`
- `nerdio-cli cost-estimator`
- `nerdio-cli cost-estimator get`
- `nerdio-cli cost-estimator list`
- `nerdio-cli directories`
- `nerdio-cli directories list`
- `nerdio-cli environment-variables`
- `nerdio-cli environment-variables list`
- `nerdio-cli resource-groups`
- `nerdio-cli resource-groups list`
- `nerdio-cli schedules`
- `nerdio-cli schedules get`
- `nerdio-cli schedules list`
- `nerdio-cli scripted-actions`
- `nerdio-cli scripted-actions list`
- `nerdio-cli secure-variables`
- `nerdio-cli secure-variables list`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
nerdio-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/nerdio-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `NERDIO_CLIENT_ID` | per_call | Yes | Set to your API credential. |
| `NERDIO_CLIENT_SECRET` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `nerdio-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `nerdio-cli doctor` to check credentials
- Verify the environment variable is set: `echo $NERDIO_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **AADSTS500011: resource principal not found**  -  Remove the api:// prefix from NERDIO_OAUTH_SCOPE - the scope is the bare '<app-id>/.default' form
- **401 Unauthorized on every call**  -  Check NERDIO_TOKEN_URL contains YOUR tenant ID (https://login.microsoftonline.com/<TENANT_ID>/oauth2/v2.0/token) and the client secret has not expired
- **422 Unprocessable Entity**  -  The subscription ID positional must be the full GUID, not the subscription display name
- **connection refused or DNS error**  -  NERDIO_BASE_URL must be your own NMM instance root (per-MSP install) - there is no global nmm.nerdio.net API endpoint
- **mutation returned only a job ID and nothing happened**  -  NMM mutations are async - run 'nerdio-cli job wait <jobId>' to poll the job to a terminal state

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**NMM-PS**](https://github.com/Get-Nerdio/NMM-PS)  -  PowerShell (3 stars)
- [**NMMAPI**](https://github.com/AndyNolan/NMMAPI)  -  PowerShell (1 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
