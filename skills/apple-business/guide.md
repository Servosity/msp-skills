# Apple Business CLI

**Every Apple Business API call, plus offline fleet answers: unassigned devices, AppleCare expiry and Activation Lock.**

Inventory, device management services, AppleCare coverage, Activation Lock, users, groups, Blueprints, Configurations and the audit log, all scriptable with --json, --select and --dry-run. Sync an org into SQLite and run devices enrich once to answer fleet questions offline; each customer org's API account gets its own local store, and fleet summary rolls them up.

Learn more at [Apple Business](https://business.apple.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `apple-business-cli` and `apple-business-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.ps1 | iex
   ```
3. Verify: `apple-business-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=apple-business). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/apple-business --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/apple-business --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `apple-business-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the apple-business skill from https://github.com/Servosity/msp-skills/tree/main/skills/apple-business. The skill defines how its required CLI (`apple-business-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=apple-business).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `APPLE_BUSINESS_CLIENT_ID`, `APPLE_BUSINESS_KEY_ID` and `APPLE_BUSINESS_PRIVATE_KEY` (the path to the downloaded .pem file, or its PEM text) when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "apple-business": {
      "command": "apple-business-mcp",
      "env": {
        "APPLE_BUSINESS_CLIENT_ID": "BUSINESSAPI.<uuid>",
        "APPLE_BUSINESS_KEY_ID": "<key-id>",
        "APPLE_BUSINESS_PRIVATE_KEY": "/path/to/private-key.pem"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`apple-business-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `APPLE_BUSINESS_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `APPLE_BUSINESS_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export APPLE_BUSINESS_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
apple-business-mcp --transport http
```

Set `APPLE_BUSINESS_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Apple Business uses OAuth 2 client credentials with a signed client assertion. An Organization Administrator opens Settings > API in Apple Business, selects Add API Account, then Generate & Download to save the private key (.pem, generated once); Edit shows the Client ID (BUSINESSAPI.<uuid>) and Key ID. Set APPLE_BUSINESS_CLIENT_ID, APPLE_BUSINESS_KEY_ID and APPLE_BUSINESS_PRIVATE_KEY (prefer the path to the .pem file so the key text is never copied to disk), or run auth login --client-id <id> --client-secret <path.pem> --key-id <key id>. The CLI signs a 10-minute ES256 assertion, exchanges it at account.apple.com for a one-hour bearer token, keeps it (in the private credential store after auth login; in memory only for an account given in the environment or --config), and re-mints on expiry or 401. Several customer orgs: API accounts belong to one organization, so use one per customer and switch by changing the three APPLE_BUSINESS_* variables (or --config <org>.toml with business_client_id, business_private_key and business_key_id). The explicit account always wins over a saved login, and each Client ID syncs into its own store in the shared data directory, which --all-orgs and fleet summary read. The --client-profile flag is not used by this CLI. A `--config <org>.toml` file (or APPLE_BUSINESS_CONFIG) is one customer's whole saved identity: `auth login --config` saves the account and key reference in that file, a run under it never uses the shared saved login, and `auth logout --config` clears only that file.

## Quick Start

```bash
# Confirm the CLI is installed without touching the network; run doctor without --dry-run to check the three credentials and connectivity
apple-business-cli doctor --dry-run

# Pull the org's device inventory and device management services into the local store
apple-business-cli sync --resources devices,mdm_servers

# See devices that will not enroll at unboxing because no MDM server holds them
apple-business-cli devices unassigned --added-within 7d

# Everything Apple Business knows about one device: server, AppleCare, Activation Lock
apple-business-cli devices lookup C02XL0ABJGH5

# Preview the assignment activity before sending it
apple-business-cli devices assign --server "Jamf Prod" C02XL0ABJGH5 --wait --dry-run

```

## Unique Features

### Live device workflows
- **`devices lookup`**  -  One row per device with its org record, assigned MDM server, AppleCare and Activation Lock, by serial (live) or by MAC, IMEI, EID or order number (needs sync first).

  _Reach for this when a ticket names one device and you need everything Apple Business knows about it in one call._

  ```bash
  apple-business-cli devices lookup C02XL0ABJGH5 --agent
  ```
- **`devices assign`**  -  Assign or move serials to a device management service by name; with --wait, poll the activity and verify every serial landed.

  _Use this instead of hand-building an orgDeviceActivities body when devices must reach the right MDM._

  ```bash
  apple-business-cli devices assign --server "Jamf Prod" C02XL0ABJGH5 --wait --dry-run
  ```
- **`devices unassign`**  -  Remove serials from whatever device management service they are on now, one activity per server; with --wait, verify each.

  _Use this when returning or re-homing devices without knowing which MDM currently holds them._

  ```bash
  apple-business-cli devices unassign C02XL0ABJGH5 --wait --dry-run
  ```
- **`devices enrich`**  -  Fetch AppleCare coverage, Activation Lock status and the serial-to-server map for every synced device into the local store, resumable.

  _Run this after sync and before any AppleCare or Activation Lock report._

  ```bash
  apple-business-cli devices enrich --applecare --activation-lock --servers --skip-fresh 7d
  ```

### Fleet reports from the local store
- **`applecare expiring`**  -  Devices whose AppleCare ends within a window (or, with --uncovered, with no active coverage), with optional counts by order, family, server or org. Needs sync and devices enrich --applecare.

  _Use this for renewal quotes and QBR material instead of thousands of per-device calls._

  ```bash
  apple-business-cli applecare expiring --within 90d --group-by order --agent
  ```
- **`devices unassigned`**  -  Org-owned devices not assigned to any MDM server, with days since it joined the org and a flag when no server defaults their product family. Needs sync of devices and mdm_servers.

  _Run this weekly so new devices enroll at unboxing instead of failing in front of the user._

  ```bash
  apple-business-cli devices unassigned --added-within 7d --agent
  ```
- **`devices locked`**  -  Devices currently Activation Locked, user locks by default, with the owner when built-in device management data is synced. Needs devices enrich --activation-lock.

  _Use this before redeploying or releasing hardware so a user lock does not strand a device._

  ```bash
  apple-business-cli devices locked --type user --agent
  ```
- **`mdm-servers health`**  -  Each MDM server's status, device count, days since it last connected to Apple and its default product families, flagging stale servers (ACTIVE, owns devices, silent past --stale) and INACTIVE or DELETED servers that still own devices.

  _Use this when devices stop checking in or fleet summary shows stale servers: it names the server and how long it has been silent._

  ```bash
  apple-business-cli mdm-servers health --all-orgs --issues-only --agent
  ```

### Cross-customer and audit
- **`audit-events digest`**  -  A readable change log over a relative window: one flattened row per event, newest first, plus per-category counts.

  _Use this for weekly change reviews and security evidence instead of raw audit-events list calls._

  ```bash
  apple-business-cli audit-events digest --since 7d --agent
  ```
- **`fleet summary`**  -  One row of counts per customer org: devices, unassigned, AppleCare expiring and uncovered, user-locked, stale MDM servers, sync age. Each org needs sync and devices enrich under its own API account.

  _Use this for a Monday cross-customer view before drilling into one org._

  ```bash
  apple-business-cli fleet summary --within 90d --agent
  ```
- **`audit-events actors`**  -  Who made changes over a relative window: one row per actor (admin user or API account, grouped by actor id) with event counts, non-success outcomes, categories and event types.

  _Use this for a SOC 2 or customer security review: who touched the org, how often, and which changes failed._

  ```bash
  apple-business-cli audit-events actors --since 30d --agent
  ```

## Recipes

### Weekly new-device intake

```bash
apple-business-cli devices assign --server "Jamf Prod" --from-unassigned --family Mac --added-within 7d --wait
```

After sync, assigns last week's unassigned Macs to one server by name, re-checks each live, waits and verifies; preview the selection first with devices unassigned --family Mac --added-within 7d, or add --dry-run to see the request with the resolved serials.

### AppleCare renewals this quarter

```bash
apple-business-cli applecare expiring --within 90d --group-by order --agent
```

After sync and devices enrich --applecare: devices whose coverage ends in 90 days, with a count per purchase order for quoting.

### Narrow a large device payload for an agent

```bash
apple-business-cli devices list --agent --select id,attributes.serialNumber,attributes.productFamily,attributes.status
```

List items are already unwrapped from Apple's data envelope, so select fields by attributes.<name>; --agent alone compacts to ids.

### Last week's changes

```bash
apple-business-cli audit-events digest --since 7d --agent
```

Device moves, API key events and domain changes, one flattened row per event, newest first, plus per-category counts.

### Before redeploying returns

```bash
apple-business-cli devices locked --type user --agent
```

After devices enrich --activation-lock: user Activation Locks need the previous owner; the owner is shown when built-in device management data (mdm_devices, users) is synced.

## Usage

Run `apple-business-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `APPLE_BUSINESS_CONFIG_DIR`, `APPLE_BUSINESS_DATA_DIR`, `APPLE_BUSINESS_STATE_DIR`, or `APPLE_BUSINESS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `APPLE_BUSINESS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export APPLE_BUSINESS_HOME=/srv/apple-business
apple-business-cli doctor
```

On Windows (PowerShell):

```powershell
$env:APPLE_BUSINESS_HOME = "C:\ProgramData\apple-business"
apple-business-cli doctor
```

Under `APPLE_BUSINESS_HOME=/srv/apple-business`, the four dirs resolve to `/srv/apple-business/config`, `/srv/apple-business/data`, `/srv/apple-business/state`, and `/srv/apple-business/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:APPLE_BUSINESS_HOME = "C:\srv\apple-business"
apple-business-cli doctor
```

Under `$env:APPLE_BUSINESS_HOME = "C:\srv\apple-business"`, the four dirs resolve to `C:\srv\apple-business\config`, `C:\srv\apple-business\data`, `C:\srv\apple-business\state`, and `C:\srv\apple-business\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "apple-business": {
      "command": "apple-business-mcp",
      "env": {
        "APPLE_BUSINESS_HOME": "/srv/apple-business"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `APPLE_BUSINESS_DATA_DIR` overrides an explicit `--home` for that kind. Use `APPLE_BUSINESS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `APPLE_BUSINESS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `apple-business-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. This CLI keeps no response cache: every live read goes to Apple. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `apple-business-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `apple-business-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `APPLE_BUSINESS_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `APPLE_BUSINESS_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### apps

Manage apps

- **`apple-business-cli apps get`** - Get information about a specific app for an organization using the built-in device management in Apple Business.
- **`apple-business-cli apps list`** - Get a list of apps for an organization using the built-in device management in Apple Business.

### audit_events

Manage audit events

- **`apple-business-cli audit-events`** - Get a list of audit events in an organization that satisfies the query criteria.

### blueprints

Manage blueprints

- **`apple-business-cli blueprints add-apps`** - Add apps to a Blueprint.
- **`apple-business-cli blueprints add-configurations`** - Add Configurations to a Blueprint.
- **`apple-business-cli blueprints add-devices`** - Add devices to a Blueprint.
- **`apple-business-cli blueprints add-packages`** - Add packages to a Blueprint.
- **`apple-business-cli blueprints add-user-groups`** - Add user groups to a Blueprint.
- **`apple-business-cli blueprints add-users`** - Add users to a Blueprint.
- **`apple-business-cli blueprints create`** - Create a new Blueprint in an organization.
- **`apple-business-cli blueprints delete`** - Delete a Blueprint from an organization.
- **`apple-business-cli blueprints get`** - Get information about a specific Blueprint in an organization.
- **`apple-business-cli blueprints list`** - Get a list of Blueprints in an organization.
- **`apple-business-cli blueprints list-apps`** - Get a list of app IDs associated with a Blueprint.
- **`apple-business-cli blueprints list-configurations`** - Get a list of Configuration IDs associated with a Blueprint.
- **`apple-business-cli blueprints list-devices`** - Get a list of device IDs associated with a Blueprint.
- **`apple-business-cli blueprints list-packages`** - Get a list of package IDs associated with a Blueprint.
- **`apple-business-cli blueprints list-user-groups`** - Get a list of user group IDs associated with a Blueprint.
- **`apple-business-cli blueprints list-users`** - Get a list of user IDs associated with a Blueprint.
- **`apple-business-cli blueprints remove-apps`** - Remove apps from a Blueprint.
- **`apple-business-cli blueprints remove-configurations`** - Remove Configurations from a Blueprint.
- **`apple-business-cli blueprints remove-devices`** - Remove devices from a Blueprint.
- **`apple-business-cli blueprints remove-packages`** - Remove packages from a Blueprint.
- **`apple-business-cli blueprints remove-user-groups`** - Remove user groups from a Blueprint.
- **`apple-business-cli blueprints remove-users`** - Remove users from a Blueprint.
- **`apple-business-cli blueprints update`** - Update an existing Blueprint in an organization.

### configurations

Manage configurations

- **`apple-business-cli configurations create`** - Create a new custom Configuration in an organization.
- **`apple-business-cli configurations delete`** - Delete a Configuration from an organization.
- **`apple-business-cli configurations get`** - Get information about a specific Configuration in an organization.
- **`apple-business-cli configurations list`** - Get a list of Configurations in an organization.
- **`apple-business-cli configurations update`** - Update an existing custom Configuration in an organization.

### device_activities

Manage device activities

- **`apple-business-cli device-activities create`** - Assign or unassign devices to a device management service, manage a device management service migration for devices, or release devices from an organization. To assign devices to a device management service and schedule a device management service migration, use the ASSIGN_DEVICES_WITH_MDM_MIGRATION_DEADLINE activity type and include activityTypeMetadata.mdmMigrationDeadlineDateTime, an ISO 8601 date and time by which the migration needs to complete. The deadline can't be more than 90 days in the future.To update the deadline for an in-progress device management service migration, use the UPDATE_MDM_MIGRATION_DEADLINE activity type. This doesn't require a mdmServer relationship. If the new deadline is earlier than the existing one, or in the past, the device honors it without giving the user an option to delay the migration.If the request tries to update the deadline for a serial number that isn't undergoing a device management service migration, the activity log result indicates a failure for that serial number.To cancel an in-progress device management service migration, use the CANCEL_MDM_MIGRATION activity type. This doesn't require a mdmServer relationship or activityTypeMetadata.If the request tries to cancel a device management service migration for a serial number that isn't undergoing one, the activity log result indicates a failure for that serial number.To release devices from an organization, use RELEASE_DEVICES activity type.
- **`apple-business-cli device-activities get`** - Get information for an organization device activity that a device management action, such as assign or unassign, creates.

### devices

Manage devices

- **`apple-business-cli devices activation-lock`** - Get the Activation Lock status of an organization device.
- **`apple-business-cli devices applecare-coverage`** - Get a list of AppleCare coverage resources for an organization device.
- **`apple-business-cli devices assigned-server`** - Get the assigned device management service information for a device.
- **`apple-business-cli devices assigned-server-id`** - Get the assigned device management service ID information for a device.
- **`apple-business-cli devices get`** - Get information about a device in an organization.
- **`apple-business-cli devices list`** - Get a list of devices in an organization that enroll using Automated Device Enrollment.

### mdm_devices

Manage mdm devices

- **`apple-business-cli mdm-devices details`** - Get detailed information about a device enrolled in Apple Device Management service.
- **`apple-business-cli mdm-devices list`** - Get a list of devices enrolled in Apple Device Management service.

### mdm_servers

Manage mdm servers

- **`apple-business-cli mdm-servers create`** - Create a new device management service in an organization.
- **`apple-business-cli mdm-servers delete`** - Delete a device management service from an organization.
- **`apple-business-cli mdm-servers devices`** - Get a list of device serial numbers assigned to a device management service.
- **`apple-business-cli mdm-servers get`** - Get the information for a specific device management service.
- **`apple-business-cli mdm-servers list`** - Get a list of device management services in an organization.
- **`apple-business-cli mdm-servers update`** - Update an existing device management service in an organization.

### org_units

Manage org units

- **`apple-business-cli org-units get`** - Get information about a specific organizational unit in an organization.
- **`apple-business-cli org-units list`** - Get a list of organizational units in an organization.
- **`apple-business-cli org-units users`** - Get a list of user IDs for an organizational unit in an organization.

### packages

Manage packages

- **`apple-business-cli packages get`** - Get information about a specific package for an organization using Apple Business's built-in device management.
- **`apple-business-cli packages list`** - Get a list of packages for an organization using Apple Business's built-in device management.

### user_groups

Manage user groups

- **`apple-business-cli user-groups get`** - Get information about a specific user group in an organization.
- **`apple-business-cli user-groups list`** - Get a list of user groups in an organization.
- **`apple-business-cli user-groups users`** - Get a list of user IDs for a user group in an organization.

### users

Manage users

- **`apple-business-cli users get`** - Get information about a specific user in an organization.
- **`apple-business-cli users list`** - Get a list of users in an organization.


### applecare


- **`apple-business-cli applecare expiring`** - Devices whose AppleCare ends within a window (or, with --uncovered, with no active coverage), with optional counts by order, family, server or org. Needs sync and devices enrich --applecare.


### audit-events


- **`apple-business-cli audit-events digest`** - A readable change log over a relative window: one flattened row per event, newest first, plus per-category counts.


### devices


- **`apple-business-cli devices lookup`** - One row per device with its org record, assigned MDM server, AppleCare and Activation Lock, by serial (live) or by MAC, IMEI, EID or order number (needs sync first).

- **`apple-business-cli devices assign`** - Assign or move serials to a device management service by name; with --wait, poll the activity and verify every serial landed.

- **`apple-business-cli devices unassign`** - Remove serials from whatever device management service they are on now, one activity per server; with --wait, verify each.

- **`apple-business-cli devices enrich`** - Fetch AppleCare coverage, Activation Lock status and the serial-to-server map for every synced device into the local store, resumable.

- **`apple-business-cli devices unassigned`** - Org-owned devices not assigned to any MDM server, with days since it joined the org and a flag when no server defaults their product family. Needs sync of devices and mdm_servers.

- **`apple-business-cli devices locked`** - Devices currently Activation Locked, user locks by default, with the owner when built-in device management data is synced. Needs devices enrich --activation-lock.


### fleet


- **`apple-business-cli fleet summary`** - One row of counts per customer org: devices, unassigned, AppleCare expiring and uncovered, user-locked, stale MDM servers, sync age. Each org needs sync and devices enrich under its own API account.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`apple-business-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`apple-business-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`apple-business-cli learnings list`** - Inspect taught rows
- **`apple-business-cli learnings forget <query>`** - Undo a teach
- **`apple-business-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`apple-business-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`apple-business-cli teach-pattern`** - Install a query/resource template up front
- **`apple-business-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `APPLE_BUSINESS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `apple-business-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
apple-business-cli apps list

# JSON for scripting and agents
apple-business-cli apps list --json
# Filter to specific fields
apple-business-cli apps list --json --select id

# Dry run  -  show the request without sending
apple-business-cli apps list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
apple-business-cli apps list --agent
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

## Health Check

```bash
apple-business-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `apple-business-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/apple-business-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\apple-business-cli\config.toml` on Windows; `--home`, `APPLE_BUSINESS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `APPLE_BUSINESS_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `APPLE_BUSINESS_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `APPLE_BUSINESS_CLIENT_ID` | per_call | Yes | API account Client ID, e.g. BUSINESSAPI.<uuid>. |
| `APPLE_BUSINESS_PRIVATE_KEY` | per_call | Yes | ES256 private key for the API account: PEM text or a path to the downloaded .pem file. |
| `APPLE_BUSINESS_KEY_ID` | per_call | Yes | Key ID shown for the API account's private key (JWT kid header). |
| `APPLE_BUSINESS_ACCESS_TOKEN` | per_call | No | Optional override; leave blank. A bearer token minted elsewhere: while set, it is the run's whole identity. It replaces any configured or saved account, requests send it, its data stays in its own local store, the private key is not read and nothing is minted or saved. It expires in an hour and is not renewed. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `apple-business-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `apple-business-cli doctor` to check credentials
- `[credkit: withheld from <sink>: it carried a credential this program holds]` means the whole output unit repeated a credential this run holds, so the CLI replaced it. It says nothing about the credential itself
- A token request `withheld from outbound request` means `APPLE_BUSINESS_TOKEN_URL` points somewhere other than `https://account.apple.com` or the API base URL's own origin. The signed assertion goes only to those two
- Verify all three variables are set without printing the key: `test -n "$APPLE_BUSINESS_CLIENT_ID" && test -n "$APPLE_BUSINESS_KEY_ID" && test -n "$APPLE_BUSINESS_PRIVATE_KEY" && echo "all three credentials set"`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **auth error: Apple Business key ID is required**  -  export APPLE_BUSINESS_KEY_ID=<Key ID shown for the API account's private key>
- **token endpoint returned HTTP 400 or 401**  -  Check APPLE_BUSINESS_CLIENT_ID starts with BUSINESSAPI. and that the private key matches the Key ID shown under Settings > API > Edit; a lost .pem cannot be re-downloaded, so generate a new key
- **HTTP 429 Too Many Requests**  -  Lower request pace with --rate-limit 2 and prefer sync + local reads over repeated live list calls
- **curl to api-business.apple.com fails with 'Invalid HTTP header field' (error 28)**  -  Apple's HTTP/2 responses include a transfer-encoding header curl rejects; use curl --http1.1. The CLI is unaffected.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**asbmutil**](https://github.com/rodchristiansen/asbmutil)  -  Swift (142 stars)
- [**ABMate**](https://github.com/pathaksomesh06/ABMate)  -  Swift (82 stars)
- [**AxMJamfSync**](https://github.com/karthikeyan-mac/AxMJamfSync)  -  Swift (46 stars)
- [**terraform-provider-axm**](https://github.com/neilmartin83/terraform-provider-axm)  -  Go (27 stars)
- [**pyaxm**](https://github.com/kaincenteno/pyaxm)  -  Python (4 stars)
- [**go-sdk-appleservices**](https://github.com/deploymenttheory/go-sdk-appleservices)  -  Go (4 stars)
- [**abcli**](https://github.com/GigaionLLC/abcli)  -  Go (1 stars)
- [**abm-mcp**](https://github.com/o-sokhan/abm-mcp)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
