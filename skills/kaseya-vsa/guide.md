# Kaseya VSA 10 CLI

**Every VSA 10 API endpoint, plus a local fleet database that answers patch, health, and software questions across all your clients offline.**

Sync devices, assets, organizations, and audit logs once, then ask fleet-wide questions without burning the per-endpoint rate limit: health lists devices failing posture checks, patch-exposure ranks missing KBs by how many devices lack them (or by CVSS with --sort cvss), and software finds version drift. scripts run-wait and tasks run-wait run VSA automation and return the output in one call.

Learn more at [Kaseya VSA 10](https://api.vsax.net).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `kaseya-vsa-cli` and `kaseya-vsa-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.ps1 | iex
   ```
3. Verify: `kaseya-vsa-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=kaseya-vsa). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/kaseya-vsa --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/kaseya-vsa --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `kaseya-vsa-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the kaseya-vsa skill from https://github.com/Servosity/msp-skills/tree/main/skills/kaseya-vsa. The skill defines how its required CLI (`kaseya-vsa-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=kaseya-vsa).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `KASEYA_VSA_SERVER` (your server host, for example `acme.vsax.net`; the prefilled `yourcompany.vsax.net` is a placeholder that does not resolve), `KASEYA_VSA_TOKEN_ID`, and `KASEYA_VSA_TOKEN_SECRET` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "kaseya-vsa": {
      "command": "kaseya-vsa-mcp",
      "env": {
        "KASEYA_VSA_SERVER": "<your-server-host>",
        "KASEYA_VSA_TOKEN_ID": "<your-token-id>",
        "KASEYA_VSA_TOKEN_SECRET": "<your-token-secret>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`kaseya-vsa-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `KASEYA_VSA_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `KASEYA_VSA_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export KASEYA_VSA_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
kaseya-vsa-mcp --transport http
```

Set `KASEYA_VSA_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

VSA 10 uses HTTP Basic over HTTPS with an API token created under Administration > Configuration > API Access. Set KASEYA_VSA_TOKEN_ID and KASEYA_VSA_TOKEN_SECRET, and set KASEYA_VSA_SERVER to your server host (for example acme.vsax.net); there is no working default server. KASEYA_VSA_BASE_URL overrides the whole base URL (https://<server>/api/v3) if you need it. This is the VSA 10 (api/v3) API, not the VSA 9 /api/v1.0 API.

## Quick Start

```bash
# Confirm the CLI is installed; the dry run makes no API call
kaseya-vsa-cli doctor --dry-run

# Check the token, KASEYA_VSA_SERVER and connectivity against your VSA 10 server
kaseya-vsa-cli doctor

# Build the local fleet database the health, patch-exposure, software and audit-logs digest commands read
kaseya-vsa-cli sync --resources organizations,sites,assets,audit_logs

# Find devices that need attention; scanned_devices 0 means nothing is synced yet
kaseya-vsa-cli health --check offline,av,firewall --agent

# Rank the critical updates missing on the most devices
kaseya-vsa-cli patch-exposure --by kb --severity critical --agent

```

## Unique Features

### Fleet analytics from the local store
- **`health`**  -  From the synced local mirror (run sync first), list devices across every client that are offline, have antivirus off or stale, firewall or UAC off, low disk, or high CPU or memory; rows are fail or unknown, with scanned and per-check counts.

  _Reach for this for Monday-morning fleet triage instead of paging assets and inspecting each record._

  ```bash
  kaseya-vsa-cli health --check offline,av,firewall --offline-for 72h --agent
  ```
- **`patch-exposure`**  -  From the synced local mirror (run sync first), rank pending OS updates across all clients by KB, client, or device, filtered by severity, CVSS, KB, or CVE.

  _Use this to answer which KB or CVE is the worst exposure and where it lives._

  ```bash
  kaseya-vsa-cli patch-exposure --by kb --severity critical,important --min-cvss 7 --agent
  ```
- **`software`**  -  From the synced local mirror (run sync first), find which devices run an application, its version spread, devices below a version, or devices missing it.

  _Use this for version drift, risky remote-access tools, or devices missing a security agent._

  ```bash
  kaseya-vsa-cli software "Google Chrome" --versions --agent
  ```
- **`audit-logs digest`**  -  From the synced local mirror (run sync first), see who did what in the tenant over a time window, with audit categories and event types decoded to names.

  _Use this for weekly review of remote control, API token, account, and bulk-action activity._

  ```bash
  kaseya-vsa-cli audit-logs digest --since 7d --agent
  ```

### Agent-native automation
- **`devices brief`**  -  Get one device's online state, notification counts, security posture, disks, pending-update counts by severity, antivirus, policies, and recent audit activity from six live API calls.

  _Use this first when triaging a single machine escalation._

  ```bash
  kaseya-vsa-cli devices brief FRONTDESK-01 --agent
  ```
- **`scripts run-wait`**  -  Run an automation script on a managed device (starts immediately, no confirmation) and get its exit code and output when it finishes.

  _Use this when an agent needs the result of a script, not just an execution ID._

  ```bash
  kaseya-vsa-cli scripts run-wait 11111111-2222-3333-4444-555555555555 --device FRONTDESK-01 --max-wait 10m --agent
  ```
- **`tasks run-wait`**  -  Run a saved task on chosen devices (starts immediately, no confirmation) or attach to a running execution, and collect per-device, per-script exit codes and output.

  _Use this to collect which devices failed a task execution and why, or to run a task on named devices and wait for the results._

  ```bash
  kaseya-vsa-cli tasks run-wait 1234 --attach 10450 --failed-only --agent
  ```

## Recipes

### Offline devices by client

```bash
kaseya-vsa-cli health --check offline --offline-for 7d --by org --agent
```

Counts devices that had not checked in for a week as of the last sync, per organization.

### Critical KBs across the fleet

```bash
kaseya-vsa-cli patch-exposure --by kb --severity critical --min-cvss 8 --agent
```

Ranks the critical updates missing on the most devices.

### Narrow a device list

```bash
kaseya-vsa-cli devices list --filter "contains(tolower(GroupName), 'acme')" --agent --select Identifier,Name,GroupName,OrganizationName
```

Uses an OData filter and keeps only the fields an agent needs.

### Run a script and wait

```bash
kaseya-vsa-cli scripts run-wait 11111111-2222-3333-4444-555555555555 --device FRONTDESK-01 --agent
```

Runs a script and returns its exit code and output.

## Usage

Run `kaseya-vsa-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `KASEYA_VSA_CONFIG_DIR`, `KASEYA_VSA_DATA_DIR`, `KASEYA_VSA_STATE_DIR`, or `KASEYA_VSA_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `KASEYA_VSA_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export KASEYA_VSA_HOME=/srv/kaseya-vsa
kaseya-vsa-cli doctor
```

On Windows (PowerShell):

```powershell
$env:KASEYA_VSA_HOME = "C:\ProgramData\kaseya-vsa"
kaseya-vsa-cli doctor
```

Under `KASEYA_VSA_HOME=/srv/kaseya-vsa`, the four dirs resolve to `/srv/kaseya-vsa/config`, `/srv/kaseya-vsa/data`, `/srv/kaseya-vsa/state`, and `/srv/kaseya-vsa/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:KASEYA_VSA_HOME = "C:\srv\kaseya-vsa"
kaseya-vsa-cli doctor
```

Under `$env:KASEYA_VSA_HOME = "C:\srv\kaseya-vsa"`, the four dirs resolve to `C:\srv\kaseya-vsa\config`, `C:\srv\kaseya-vsa\data`, `C:\srv\kaseya-vsa\state`, and `C:\srv\kaseya-vsa\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "kaseya-vsa": {
      "command": "kaseya-vsa-mcp",
      "env": {
        "KASEYA_VSA_HOME": "/srv/kaseya-vsa"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `KASEYA_VSA_DATA_DIR` overrides an explicit `--home` for that kind. Use `KASEYA_VSA_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `KASEYA_VSA_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `kaseya-vsa-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. This connector keeps no on-disk response cache. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `kaseya-vsa-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `kaseya-vsa-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `KASEYA_VSA_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `KASEYA_VSA_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### assets

Manage assets

- **`kaseya-vsa-cli assets get-device`** - Returns the assets for a specific device.
- **`kaseya-vsa-cli assets list`** - Returns a list of assets.

Sync requests include=Tags,Updates,AvailableUpdates,Security,IpAddresses,Disks,InstalledSoftware,LocalIpAddresses (every section this endpoint offers except AssetInfo, which carries the Windows product key).

### audit_logs

Manage audit logs

- **`kaseya-vsa-cli audit-logs`** - Returns a list of audit log entries.

### custom_field_usage

Manage custom field usage

- **`kaseya-vsa-cli custom-field-usage <customFieldId>`** - Returns a list of places where the Custom Field is used. Organizations, Sites, Groups, Devices, Scopes that are inaccessible to the API token are not displayed in the response.

### custom_fields

List and get custom fields; assign, update, or unassign their values

- **`kaseya-vsa-cli custom-fields assign`** - Assigns a VSA 10 Custom Field.
- **`kaseya-vsa-cli custom-fields get`** - Returns the custom field details.
- **`kaseya-vsa-cli custom-fields list`** - Returns a list of Custom Fields.
- **`kaseya-vsa-cli custom-fields unassign`** - Unassigns a VSA 10 Custom Field.
- **`kaseya-vsa-cli custom-fields update-assigned`** - Updates the Value of a VSA 10 Custom Field.

### devices

List and get devices, their antivirus, notifications, policies and custom fields; move, publish, or brief one

- **`kaseya-vsa-cli devices get`** - Returns the device details.
- **`kaseya-vsa-cli devices get-antivirus`** - Returns the antivirus status of a device.
- **`kaseya-vsa-cli devices list`** - Returns a list of devices.
- **`kaseya-vsa-cli devices list-applied-policies`** - Returns a list of policies and profiles applied to a device.
- **`kaseya-vsa-cli devices list-custom-fields`** - Returns the device custom fields.
- **`kaseya-vsa-cli devices list-notifications`** - Return the device notifications.
- **`kaseya-vsa-cli devices move`** - Moves a Device to a different Group.
- **`kaseya-vsa-cli devices publish`** - Publish registers or updates a VSA 10 Device instance.

### endpoint_protection

Manage endpoint protection

- **`kaseya-vsa-cli endpoint-protection <policyId>`** - Returns the details of the endpoint protection policy.

### environment

Manage environment

- **`kaseya-vsa-cli environment`** - Returns environment information for the instance.

### group_custom_fields

Manage group custom fields

- **`kaseya-vsa-cli group-custom-fields <groupId>`** - Return the group custom fields.

### groups

Manage groups

- **`kaseya-vsa-cli groups create`** - Creates a new Group within a Site.
- **`kaseya-vsa-cli groups delete`** - Deletes a Group.
- **`kaseya-vsa-cli groups get`** - Returns the group details.
- **`kaseya-vsa-cli groups get-package`** - Returns the Name and Download URL for Group install package. Rate limit: 600 requests per hour.
- **`kaseya-vsa-cli groups list`** - Returns a list of active (not deleted) groups.
- **`kaseya-vsa-cli groups update`** - Update a Group.

### notifications

Manage notifications

- **`kaseya-vsa-cli notifications create`** - Creates a device notification.
- **`kaseya-vsa-cli notifications delete`** - Deletes a specific notification.
- **`kaseya-vsa-cli notifications get`** - Returns the notification details.
- **`kaseya-vsa-cli notifications list`** - Returns the device notifications.

### organization_custom_fields

Manage organization custom fields

- **`kaseya-vsa-cli organization-custom-fields <organizationId>`** - Return the organization custom fields.

### organizations

Manage organizations

- **`kaseya-vsa-cli organizations create`** - Creates a new Organization.
- **`kaseya-vsa-cli organizations delete`** - Deletes an Organization.
- **`kaseya-vsa-cli organizations get`** - Returns the organization details.
- **`kaseya-vsa-cli organizations list`** - Returns a list of organizations.
- **`kaseya-vsa-cli organizations update`** - Updates an Organization.

### patch_management

Get patch management global rules and policy details

- **`kaseya-vsa-cli patch-management get-global-rules`** - Returns the global rules for patch management.
The global rules get automatically applied ahead of all policy rules and are evaluated in a top-down order.
- **`kaseya-vsa-cli patch-management get-policy`** - Returns the details of the patch management policy.

### rate_limits

Manage rate limits

- **`kaseya-vsa-cli rate-limits`** - Returns rate limits information.

### scope_usage

Manage scope usage

- **`kaseya-vsa-cli scope-usage <scopeId>`** - Returns a list of places where the Scope is used.

### scopes

Manage scopes

- **`kaseya-vsa-cli scopes get`** - Returns the scope details.
- **`kaseya-vsa-cli scopes list`** - Returns a list of Scopes.

### scripts

Manage scripts

- **`kaseya-vsa-cli scripts get`** - Returns the automation script details.
- **`kaseya-vsa-cli scripts get-execution`** - Returns the details of a script execution. Note that execution details may not be available if the device is offline.
- **`kaseya-vsa-cli scripts list`** - Returns a list of automation scripts.
- **`kaseya-vsa-cli scripts list-executions`** - Returns script executions for the specified device. Note that executions may not be available if the device is offline.
- **`kaseya-vsa-cli scripts run`** - Runs a specific automation script.

### site_custom_fields

Manage site custom fields

- **`kaseya-vsa-cli site-custom-fields <siteId>`** - Return the Site custom fields.

### sites

Manage sites

- **`kaseya-vsa-cli sites create`** - Creates a new Site within an Organization.
- **`kaseya-vsa-cli sites delete`** - Deletes a Site.
- **`kaseya-vsa-cli sites get`** - Returns the Site details.
- **`kaseya-vsa-cli sites list`** - Returns a list of Sites.
- **`kaseya-vsa-cli sites update`** - Updates a Site.

### tasks

Manage tasks

- **`kaseya-vsa-cli tasks get`** - Returns the task details.
- **`kaseya-vsa-cli tasks get-execution`** - Returns the status of a task execution.
- **`kaseya-vsa-cli tasks get-execution-script-output`** - Returns the output from a script executed as part of a task on a specific device.
- **`kaseya-vsa-cli tasks list`** - Returns a list of automation tasks.
- **`kaseya-vsa-cli tasks list-execution-devices`** - Returns the execution status for devices on which the task has been executed.
- **`kaseya-vsa-cli tasks list-execution-scripts`** - Returns a list of scripts that were executed as part of a task on a specific device.
- **`kaseya-vsa-cli tasks run`** - Runs a specific automation task.

### webhooks

Manage webhooks

- **`kaseya-vsa-cli webhooks create`** - Creates a Notification Webhook.
- **`kaseya-vsa-cli webhooks delete`** - Deletes a Specific Notification Webhook.
- **`kaseya-vsa-cli webhooks get`** - Returns the notification webhook details.
- **`kaseya-vsa-cli webhooks list`** - Returns a list of Notification Webhooks.
- **`kaseya-vsa-cli webhooks regenerate-secret-key`** - Re-generates a Notification Webhook Secret Key.
This will cause the previous Secret Key to become outdated, and it will be replaced by a new one in the calculation of the x-hmac-signature header when sending a Notification Message to the Webhook URL.
- **`kaseya-vsa-cli webhooks update`** - Updates a Notification Webhook.

### workflows

Manage workflows

- **`kaseya-vsa-cli workflows cancel-executions`** - Cancels specified workflow executions.
- **`kaseya-vsa-cli workflows get`** - Returns the workflow details.
- **`kaseya-vsa-cli workflows get-execution`** - Returns the details of a workflow execution.
- **`kaseya-vsa-cli workflows list`** - Returns a list of workflows.
- **`kaseya-vsa-cli workflows list-executions`** - Returns running workflow executions and the latest completed workflow executions.
- **`kaseya-vsa-cli workflows run`** - Runs a specific automation workflow.


### audit-logs


- **`kaseya-vsa-cli audit-logs digest`** - From the synced local mirror (run sync first), see who did what in the tenant over a time window, with audit categories and event types decoded to names.


### devices


- **`kaseya-vsa-cli devices brief`** - Get one device's online state, notification counts, security posture, disks, pending-update counts by severity, antivirus, policies, and recent audit activity from six live API calls.


### health


- **`kaseya-vsa-cli health`** - From the synced local mirror (run sync first), list devices across every client that are offline, have antivirus off or stale, firewall or UAC off, low disk, or high CPU or memory; rows are fail or unknown, with scanned and per-check counts.


### patch-exposure


- **`kaseya-vsa-cli patch-exposure`** - From the synced local mirror (run sync first), rank pending OS updates across all clients by KB, client, or device, filtered by severity, CVSS, KB, or CVE.


### scripts


- **`kaseya-vsa-cli scripts run-wait`** - Run an automation script on a managed device (starts immediately, no confirmation) and get its exit code and output when it finishes.


### software


- **`kaseya-vsa-cli software`** - From the synced local mirror (run sync first), find which devices run an application, its version spread, devices below a version, or devices missing it.


### tasks


- **`kaseya-vsa-cli tasks run-wait`** - Run a saved task on chosen devices (starts immediately, no confirmation) or attach to a running execution, and collect per-device, per-script exit codes and output.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`kaseya-vsa-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`kaseya-vsa-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`kaseya-vsa-cli learnings list`** - Inspect taught rows
- **`kaseya-vsa-cli learnings forget <query>`** - Undo a teach
- **`kaseya-vsa-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`kaseya-vsa-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`kaseya-vsa-cli teach-pattern`** - Install a query/resource template up front
- **`kaseya-vsa-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `KASEYA_VSA_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `kaseya-vsa-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
kaseya-vsa-cli assets list

# JSON for scripting and agents
kaseya-vsa-cli assets list --json
# Filter to specific fields
kaseya-vsa-cli assets list --json --select Identifier,Name,AntivirusEnabled,AntivirusUpToDate,FirewallEnabled

# Dry run  -  show the request without sending
kaseya-vsa-cli assets list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
kaseya-vsa-cli assets list --agent
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

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `KASEYA_VSA_SERVER` resolves `{server}`

Base URL: `https://{server}/api/v3`

## Health Check

```bash
kaseya-vsa-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `kaseya-vsa-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/kaseya-vsa-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\kaseya-vsa-cli\config.toml` on Windows; `--home`, `KASEYA_VSA_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `KASEYA_VSA_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `KASEYA_VSA_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `KASEYA_VSA_SERVER` | endpoint | Yes | Your VSA 10 server host, for example `acme.vsax.net`. Fills `{server}` in `https://{server}/api/v3`; the built-in `yourcompany.vsax.net` is a placeholder that does not resolve. |
| `KASEYA_VSA_TOKEN_ID` | per_call | Yes | VSA 10 API token ID (Administration > Configuration > API Access). |
| `KASEYA_VSA_TOKEN_SECRET` | per_call | Yes | VSA 10 API token secret, shown once when the token is created. |
| `KASEYA_VSA_BASE_URL` | endpoint | No | Overrides the whole base URL (`https://<server>/api/v3`) instead of setting `KASEYA_VSA_SERVER`. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `kaseya-vsa-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `kaseya-vsa-cli doctor` to check credentials
- Verify the environment variable is set: `echo $KASEYA_VSA_TOKEN_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **429 Too Many Requests**  -  Each token and endpoint pair allows about 3600 requests per hour; wait for Retry-After, or answer fleet questions from the synced store (health, patch-exposure, software, audit-logs digest)
- **401 Unauthorized or 403 Forbidden (exit 4)**  -  Check both KASEYA_VSA_TOKEN_ID and KASEYA_VSA_TOKEN_SECRET (kaseya-vsa-cli auth status), that the token is enabled under Administration > Configuration > API Access, and that it has access to the resource (403)
- **Connection or DNS failure, or a config error naming KASEYA_VSA_SERVER**  -  Set KASEYA_VSA_SERVER to your own server host, e.g. acme.vsax.net (the default yourcompany.vsax.net is a placeholder)
- **health, patch-exposure, software or audit-logs digest return no rows with scanned_devices 0 or synced_rows 0**  -  Nothing is synced yet: run kaseya-vsa-cli sync --resources organizations,sites,assets,audit_logs; an empty result before sync is not an all-clear
- **scripts run-wait or tasks run-wait exits 8**  -  The run is still going after --max-wait; re-run with --attach <execution-id> to keep waiting (running again without --attach starts a second run)
- **scripts run-wait or tasks run-wait exits 6**  -  The script or task ran and failed, was stopped, or exited non-zero; read exit_code and output in the JSON result

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

TLS certificates are verified by default. For a trusted development or self-signed endpoint only, pass `--insecure` for one invocation, or set `skip_tls_verify = true` in the config file for a persistent override. No environment variable turns verification off; for a private CA, set `KASEYA_VSA_CA_CERT` to its PEM bundle instead. `--insecure` overrides `--ca-cert` and `--tls-server-name`.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**scjalliance/vsax**](https://github.com/scjalliance/vsax)  -  Go
- [**VSAX-MCP**](https://github.com/perrytrienekens/VSAX-MCP)  -  JavaScript
- [**VSA10---API**](https://github.com/creativeheadz/VSA10---API)  -  Python
- [**kaseya-vsa-mcp (VSA 9 API)**](https://github.com/WYRE-AI/kaseya-vsa-mcp)  -  TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
