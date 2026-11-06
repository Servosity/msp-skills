# Nerdio Manager CLI

**The first non-PowerShell client for the Nerdio Manager for MSP API - cross-account AVD fleet audits, async-job plumbing, and offline search no other Nerdio tool has.**

Manage Azure Virtual Desktop fleets across every customer account from one terminal: audit autoscale posture fleet-wide with `fleet autoscale-audit`, sweep session-host power state with `fleet host-estate`, reconcile billing with `fleet billing-rollup`, and tame NMM's async job model with `job wait`. Includes a local SQLite store with full-text search over accounts, profiles, and scripted actions - the only offline Nerdio client in existence.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `nerdio-cli` and `nerdio-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.ps1 | iex
   ```
3. Verify: `nerdio-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=nerdio). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/nerdio --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/nerdio --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `nerdio-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the nerdio skill from https://github.com/Servosity/msp-skills/tree/main/skills/nerdio. The skill defines how its required CLI (`nerdio-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

The bundle reuses your local OAuth tokens  -  authenticate first if you haven't:

```bash
nerdio-cli auth login
```

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=nerdio).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `NERDIO_BASE_URL`, `NERDIO_TOKEN_URL`, `NERDIO_CLIENT_ID`, `NERDIO_CLIENT_SECRET` and `NERDIO_OAUTH_SCOPE` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "nerdio": {
      "command": "nerdio-mcp",
      "env": {
        "NERDIO_BASE_URL": "https://nmm.contoso.com",
        "NERDIO_TOKEN_URL": "https://login.microsoftonline.com/<TENANT_ID>/oauth2/v2.0/token",
        "NERDIO_CLIENT_ID": "<client-id>",
        "NERDIO_CLIENT_SECRET": "<client-secret>",
        "NERDIO_OAUTH_SCOPE": "<app-id>/.default"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`nerdio-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `NERDIO_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `NERDIO_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export NERDIO_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
nerdio-mcp --transport http
```

Set `NERDIO_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

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
  nerdio-cli fleet host-estate --running-only --agent --select items.account,items.power_state
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
nerdio-cli fleet autoscale-audit --agent --max-accounts 50
```

Fan out across every customer account and flag host pools with autoscale disabled or diverging from baseline, with failed accounts reported separately.

### Weekend power check

```bash
nerdio-cli fleet host-estate --running-only --agent --select items.account,items.power_state
```

One narrow table of every session host still running across all customers - the deep response trimmed to account and power state.

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

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `NERDIO_CONFIG_DIR`, `NERDIO_DATA_DIR`, `NERDIO_STATE_DIR`, or `NERDIO_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `NERDIO_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export NERDIO_HOME=/srv/nerdio
nerdio-cli doctor
```

On Windows (PowerShell):

```powershell
$env:NERDIO_HOME = "C:\ProgramData\nerdio"
nerdio-cli doctor
```

Under `NERDIO_HOME=/srv/nerdio`, the four dirs resolve to `/srv/nerdio/config`, `/srv/nerdio/data`, `/srv/nerdio/state`, and `/srv/nerdio/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:NERDIO_HOME = "C:\srv\nerdio"
nerdio-cli doctor
```

Under `$env:NERDIO_HOME = "C:\srv\nerdio"`, the four dirs resolve to `C:\srv\nerdio\config`, `C:\srv\nerdio\data`, `C:\srv\nerdio\state`, and `C:\srv\nerdio\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "nerdio": {
      "command": "nerdio-mcp",
      "env": {
        "NERDIO_HOME": "/srv/nerdio"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `NERDIO_DATA_DIR` overrides an explicit `--home` for that kind. Use `NERDIO_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `NERDIO_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `nerdio-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `nerdio-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `nerdio-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `NERDIO_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `NERDIO_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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


### fleet


- **`nerdio-cli fleet autoscale-audit`** - See every host pool across all customer accounts whose autoscale is disabled or diverges from your baseline - the Monday cost-control sweep as one command.

- **`nerdio-cli fleet host-estate`** - One table of every session host across all customer accounts with pool, account, and power state - the weekend power-sweep view.

- **`nerdio-cli fleet billing-rollup`** - Per-account billed/paid/unpaid/usage rollup for a billing period, joined to account names - PSA-reconciliation-ready without Excel.


### job


- **`nerdio-cli job wait`** - Wait for any NMM async job to finish - polls until Completed/Failed/Cancelled and exits with a typed code reflecting the outcome.


### scripted-actions


- **`nerdio-cli scripted-actions fan-run`** - Execute one scripted action across many customer accounts, collect every returned job ID, and optionally wait for all of them to finish.


### usages


- **`nerdio-cli usages drift`** - Flag customer accounts whose consumption grew or shrank beyond a threshold between two periods.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`nerdio-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`nerdio-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`nerdio-cli learnings list`** - Inspect taught rows
- **`nerdio-cli learnings forget <query>`** - Undo a teach
- **`nerdio-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`nerdio-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`nerdio-cli teach-pattern`** - Install a query/resource template up front
- **`nerdio-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `NERDIO_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `nerdio-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
nerdio-cli accounts

# JSON for scripting and agents
nerdio-cli accounts --json
# Filter to specific fields
nerdio-cli accounts --json --select id,name

# Dry run  -  show the request without sending
nerdio-cli accounts --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
nerdio-cli accounts --agent
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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

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
- `nerdio-cli invoices`
- `nerdio-cli invoices get`
- `nerdio-cli invoices list`
- `nerdio-cli resource-groups`
- `nerdio-cli resource-groups list`
- `nerdio-cli schedules`
- `nerdio-cli schedules get`
- `nerdio-cli schedules list`
- `nerdio-cli scripted-actions`
- `nerdio-cli scripted-actions list`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
nerdio-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `nerdio-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/nerdio-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\nerdio-cli\config.toml` on Windows; `--home`, `NERDIO_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `NERDIO_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `NERDIO_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `NERDIO_CLIENT_ID` | per_call | Yes | Set to your API credential. |
| `NERDIO_CLIENT_SECRET` | per_call | Yes | Set to your API credential. |
| `NERDIO_BASE_URL` | per_call | Yes | Your own NMM instance root (e.g. `https://nmm.contoso.com`); there is no vendor-global endpoint. |
| `NERDIO_TOKEN_URL` | per_call | Yes | Your Entra tenant's token endpoint: `https://login.microsoftonline.com/<TENANT_ID>/oauth2/v2.0/token`. |
| `NERDIO_OAUTH_SCOPE` | per_call | Yes | The NMM application's scope, `<app-id>/.default` with NO api:// prefix (api:// triggers AADSTS500011). |
| `NERDIO_NO_CONFIG_WRITE` | per_call | No | Set to `1` to keep every credential off disk (the token is minted per run). |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `nerdio-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- Every API request and the bearer token go only to the scheme, host and port of `NERDIO_BASE_URL`, your own NMM instance; the shipped placeholder (`nmm.example.com`) is refused, never dialled with a credential. The client ID and secret go only to `NERDIO_TOKEN_URL`, your Entra tenant's token endpoint, as the token request itself, and only when that endpoint is on Entra ID's sign-in hosts (`login.microsoftonline.com`, `login.microsoftonline.us`, `login.partner.microsoftonline.cn`, `login.chinacloudapi.cn`) or on your NMM instance's own origin; a token URL anywhere else is refused. With no token URL (or the `YOUR_TENANT_ID` placeholder) or no `NERDIO_OAUTH_SCOPE`, nothing is sent and the error names the variable. A redirect to any other host is refused, so a credential never reaches it.
- Any output, file, MCP result or outgoing request that would repeat `NERDIO_CLIENT_ID`, `NERDIO_CLIENT_SECRET`, a minted or stored token, or a value piped in with `--stdin` is withheld whole: it prints "[credkit: withheld from <where>: it carried a credential this program holds]" in its place. A response field named as a token (`accessToken`, `sessionToken` and the like) prints as "[withheld]" and the rest of the record prints as usual. Responses are never cached on disk.
- Customer secrets you ask for by name (`devices laps`, `devices bitlocker-keys`, `secure-variables list`) are printed to you and written nowhere: they are never synced, cached or stored.
- Only `credentials.toml` (mode 0600) holds a credential. A client ID and secret supplied by the environment are never written to disk, and nor is the token minted for them: the process keeps it in memory (the MCP server reuses it until it expires). A token is cached in `credentials.toml` only beside the client pair stored there (not at all under `NERDIO_NO_CONFIG_WRITE=1`), and a token is only ever sent for the client and token URL it was minted for.
- `--dry-run` sends nothing and never mints a token. Over MCP, a call that changes Nerdio returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

### Keeping credentials off disk (optional)

By default the connector caches the token it mints in the config file so the next
command does not have to mint another one. Set `NERDIO_NO_CONFIG_WRITE=1` to turn that
off: nothing is written, and each run mints a fresh token instead. Use it when
your secrets live in Keychain, Windows Credential Manager, or any launcher that
injects them as environment variables at process start, and you do not want a
plaintext copy on disk.

| What changes | With `NERDIO_NO_CONFIG_WRITE=1` |
| --- | --- |
| Token cache | Not written. A fresh token is minted per invocation. |
| `auth login` / `auth set-token` | Refuse, naming the variable, instead of reporting a save that did not happen. |
| `auth logout` | Still clears an existing config file. The erase is not a credential write. |
| MCP server | Honours the same variable, so a Claude Desktop install gets no plaintext token cache either. |

Set it in the shell for CLI use, or through the install prompt of the same name
for the MCP server. Any value other than blank, `0`, `false`, `no` or `off`
turns it on. See issue #270.

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
