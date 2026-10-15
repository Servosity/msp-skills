# WatchGuard Cloud CLI

**WatchGuard Cloud's Firebox, Endpoint Security, ThreatSync, AuthPoint, NDR and MSP account APIs in one CLI and MCP server.**

WatchGuard splits its public API into separate per-product services behind one OAuth client-credentials login and an API key. This CLI covers the Firebox, FireCloud, Endpoint Security, ThreatSync, AuthPoint, NDR and WatchGuard Cloud platform APIs, built from WatchGuard's own published OpenAPI files, and adds Service Provider fan-out across managed accounts, offline sync and search, and agent-ready JSON on every command.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `watchguard-cli` and `watchguard-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.ps1 | iex
   ```
3. Verify: `watchguard-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=watchguard). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/watchguard --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/watchguard --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `watchguard-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the watchguard skill from https://github.com/Servosity/msp-skills/tree/main/skills/watchguard. The skill defines how its required CLI (`watchguard-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=watchguard).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `WATCHGUARD_CLIENT_ID` (Access ID), `WATCHGUARD_CLIENT_SECRET` (API password), `WATCHGUARD_API_KEY` and `WATCHGUARD_ACCOUNT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "watchguard": {
      "command": "watchguard-mcp",
      "env": {
        "WATCHGUARD_ACCOUNT_ID": "ACC-1234567",
        "WATCHGUARD_CLIENT_ID": "<access-id>",
        "WATCHGUARD_CLIENT_SECRET": "<api-password>",
        "WATCHGUARD_API_KEY": "<api-key>"
      }
    }
  }
}
```

</details>

## Authentication

Enable API access in WatchGuard Cloud under Administration > Managed Access (Owner or Administrator role). Set WATCHGUARD_CLIENT_ID to the Access ID (read-only or read-write), WATCHGUARD_CLIENT_SECRET to the matching API password, and WATCHGUARD_API_KEY to the API key. The CLI exchanges the Access ID and password for a one-hour bearer token and sends the API key header on every call; 'auth status' shows what is configured. Set WATCHGUARD_ACCOUNT_ID to your account ID (ACC-... or WGC-...) so commands do not need --account-id. Accounts outside North America set WATCHGUARD_BASE_URL to https://api.deu.cloud.watchguard.com (EMEA) or https://api.jpn.cloud.watchguard.com (APAC) - the host Managed Access shows, without its /rest/ suffix - and WATCHGUARD_TOKEN_URL to the same host plus /oauth/token. A Service Provider reaches one managed account's resources with that account's audience: run 'watchguard-cli audiences --account-id ACC-1234567', then export WATCHGUARD_AUDIENCE=<audience> (or let the fan-out commands do it per account).

## Quick Start

```bash
# Check config and, with credentials set, make one authenticated read of your account
watchguard-cli doctor

# Confirm the credentials reach your WatchGuard Cloud account
watchguard-cli accounts get

# List the accounts a Service Provider manages
watchguard-cli accounts list-managed --agent

# One row per managed account across Firebox, Endpoint Security, ThreatSync and licensing
watchguard-cli accounts health --agent

# Open ThreatSync incidents across accounts, most urgent first
watchguard-cli threatsync-incidents queue --agent

```

## Unique Features

### Service Provider fan-out
- **`endpoint-devices posture`**  -  List every endpoint that is unprotected, out of date, isolated, unlicensed or waiting on a reboot, across the accounts a Service Provider manages.

  _Reach for this when asked which customers have unprotected or isolated endpoints; one call replaces a per-subscriber tour._

  ```bash
  watchguard-cli endpoint-devices posture --agent
  ```
- **`threatsync-incidents queue`**  -  See open ThreatSync incidents across your managed accounts, ranked by severity, threat score and age, with untouched incidents and recommended-but-not-taken actions called out.

  _Use this to decide what to triage next instead of paging raw incident lists._

  ```bash
  watchguard-cli threatsync-incidents queue --duration 7D --agent
  ```
- **`accounts health`**  -  One row per managed account: Fireboxes offline, endpoints at risk, open incidents by severity and licenses expiring soon (Service Provider accounts).

  _Use this for a morning check or customer review that spans every WatchGuard product._

  ```bash
  watchguard-cli accounts health --agent
  ```
- **`allocations reconcile`**  -  Compare a Service Provider's Firebox allocations against the Fireboxes that actually report in, and summarize owned, allocated and available software licenses.

  _Use this before a billing run or after onboarding and offboarding to find allocation drift._

  ```bash
  watchguard-cli allocations reconcile --expiring-within 30d --agent
  ```
- **`endpoint-patches exposure`**  -  Group missing patches by CVE and criticality across managed accounts (needs WatchGuard Patch Management), showing which customers and devices are exposed.

  _Use this on Patch Tuesday or when a new CVE lands to see who is exposed._

  ```bash
  watchguard-cli endpoint-patches exposure --agent
  ```

### Change control
- **`firebox-transactions wait`**  -  Block until a Firebox deployment or certificate-install transaction finishes: exit 0 on complete, 8 if it ended without completing (failed, timed out or canceled), 9 if still running at --max-wait.

  _Use this right after a deploy so a script or agent gets a pass or fail instead of polling._

  ```bash
  watchguard-cli firebox-transactions wait --device FB-12345 --max-wait 10m
  ```

### Local fleet history
- **`firebox-devices changes`**  -  After a sync, see which Fireboxes, FireClusters and Access Points went offline, came back, appeared, disappeared or changed firmware since the previous sync.

  _Use this to answer what changed overnight in the firewall fleet._

  ```bash
  watchguard-cli firebox-devices changes --agent
  ```
- **`firebox-devices firmware-lag`**  -  From synced devices, find Fireboxes and Access Points running older firmware than the newest version seen for the same model in your fleet.

  _Use this after a firmware advisory to list the units that still need an upgrade._

  ```bash
  watchguard-cli firebox-devices firmware-lag --agent
  ```

## Recipes

### Fleet status with only the fields you need

```bash
watchguard-cli firebox-devices --include-hierarchy --agent --select id,name,state,version,offline_since
```

Every Firebox, FireCluster and Access Point including subscriber devices, narrowed to state and firmware.

### What changed in the fleet since the last sync

```bash
watchguard-cli firebox-devices changes --agent
```

After 'sync --resources firebox_devices --resource-param firebox_devices:include_hierarchy=true --full', lists devices that went offline, came back, appeared, were removed or changed firmware since the previous sync.

### Triage queue across customers

```bash
watchguard-cli threatsync-incidents queue --duration 7D --agent
```

Open ThreatSync incidents from every subscriber ranked by severity and age, with untouched ones flagged.

### Endpoint posture sweep

```bash
watchguard-cli endpoint-devices posture --agent
```

Unprotected, isolated, unlicensed or outdated endpoints across all subscribers in one list.

### Wait for a deployment to land

```bash
watchguard-cli firebox-transactions wait --device FB-12345 --max-wait 10m
```

Blocks until the device's newest deployment completes (exit 0) or fails (exit 8); exit 9 if still running at --max-wait.

## Usage

Run `watchguard-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `WATCHGUARD_CONFIG_DIR`, `WATCHGUARD_DATA_DIR`, `WATCHGUARD_STATE_DIR`, or `WATCHGUARD_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `WATCHGUARD_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export WATCHGUARD_HOME=/srv/watchguard
watchguard-cli doctor
```

Under `WATCHGUARD_HOME=/srv/watchguard`, the four dirs resolve to `/srv/watchguard/config`, `/srv/watchguard/data`, `/srv/watchguard/state`, and `/srv/watchguard/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "watchguard": {
      "command": "watchguard-mcp",
      "env": {
        "WATCHGUARD_HOME": "/srv/watchguard"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `WATCHGUARD_DATA_DIR` overrides an explicit `--home` for that kind. Use `WATCHGUARD_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `WATCHGUARD_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `watchguard-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### accounts

Manage accounts

- **`watchguard-cli accounts create`** - Creates a managed account. All request fields are required, including contact phone, billing and mailing addresses, and industry classification.
- **`watchguard-cli accounts delete`** - Deletes the specified account.
- **`watchguard-cli accounts get`** - Retrieves information for the specified account.
- **`watchguard-cli accounts list-managed`** - Retrieves a list of managed accounts. Results can be filtered by account *type*, *name*, and delegation status, and can be sorted and paginated.
- **`watchguard-cli accounts update`** - Updates a managed account. All request fields are required, including contact phone, billing and mailing addresses, and industry classification.

### activations

Manage activations

- **`watchguard-cli activations activate`** - Activates one or more hardware devices or software licenses.
- **`watchguard-cli activations list-recent`** - Returns activations from the last 30 days for the account.
- **`watchguard-cli activations status`** - Returns activation statuses for the specified batch ID.

### allocations

Manage allocations

- **`watchguard-cli allocations allocate`** - Allocate an asset to a managed account.

**Note**: Due to export license and regulatory requirements, this endpoint will no longer accept requests after 15 October 2026 for accounts that do not have company name, industry, first name, last name, and full mailing address.
- **`watchguard-cli allocations deallocate`** - Remove an allocated asset from a managed account.
- **`watchguard-cli allocations list-allocated`** - Returns a list of allocated assets (devices and software licenses) for a Service Provider's managed accounts.
- **`watchguard-cli allocations list-assigned`** - Returns a list of assets (devices and software licenses) allocated to a specific account.
- **`watchguard-cli allocations list-owned`** - Returns a list of assets (devices and software licenses) owned by a specific account.
- **`watchguard-cli allocations summary`** - Get an inventory summary for a Service Provider account, based on allocation status.
Pagination is supported for devices only.
- **`watchguard-cli allocations update-hardware`** - For hardware devices, you can update the allocationExpiryDate and allocationExpiryType of an existing allocation.

**Note**: Due to export license and regulatory requirements, this endpoint will no longer accept requests after 15 October 2026 for accounts that do not have company name, industry, first name, last name, and full mailing address.
- **`watchguard-cli allocations update-software`** - For software products, you can update the quantity, allocationExpiryDate, allocationExpiryType of an existing allocation.

accountId, productName, and allocationType are required in the request body.

Use the same productName and/or allocationType in the request body to update allocationExpiryDate, allocationExpiryType, and quantity.

Use a new productName and/or allocationType in the request body to update those values. The serialOrLicense is required when allocationType is linked_to_license.

For WatchGuard Endpoint Security products that support modules, when you update an allocation for a managed Subscriber account or your own Service Provider account, you must include modules in the request.

For a managed Service Provider account, you can include a single productName or single module to update.

**Note**: Due to export license and regulatory requirements, this endpoint will no longer accept requests after 15 October 2026 for accounts that do not have company name, industry, first name, last name, and full mailing address.

### audiences

Manage audiences

- **`watchguard-cli audiences`** - Sets and returns the audience parameter value for the specified managed account.

### authpoint

Manage authpoint

- **`watchguard-cli authpoint check-policy`** - Returns whether the user can authenticate and which authentication methods they can use (Push, QR code, and OTP).
- **`watchguard-cli authpoint forgot-token`** - Validates the Forgot Token Verification Code and enables Forgot Token mode for the user.
- **`watchguard-cli authpoint otp`** - Validates the one-time password provided by the user.
- **`watchguard-cli authpoint password`** - Validates a user's password.
- **`watchguard-cli authpoint push`** - Request a QR code be generated or a push notification be sent to the user.
- **`watchguard-cli authpoint qrcode`** - Validates the Verification Code provided by a user for the generated QR code.
- **`watchguard-cli authpoint transaction-status`** - Check the status of the push notification sent to the user.

### endpoint-configurations

Manage endpoint configurations

- **`watchguard-cli endpoint-configurations assign`** - Links devices to a specified configuration.
- **`watchguard-cli endpoint-configurations list`** - Retrieves a list of the specified type of managed configurations associated with your WatchGuard Cloud account.

### endpoint-devices

Manage endpoint devices

- **`watchguard-cli endpoint-devices action`** - Initiates an action on the specified devices. For example, send an action to reboot a device.
- **`watchguard-cli endpoint-devices hardware`** - Retrieves the characteristics of a device BIOS.
- **`watchguard-cli endpoint-devices isolate`** - Isolates the specified devices.
- **`watchguard-cli endpoint-devices list`** - Retrieves a list of devices, and additional information, such as the device IP address and operating system. The response to this request is limited to 3,000 records.
- **`watchguard-cli endpoint-devices protection-status`** - Retrieves a list of devices with their protection status and other device information. The response to this request is limited to 3,000 records.
- **`watchguard-cli endpoint-devices scan`** - Starts a task to scan the specified devices immediately.
- **`watchguard-cli endpoint-devices stop-isolation`** - Stops isolation on the specified devices.
- **`watchguard-cli endpoint-devices uninstall`** - Uninstalls protection from the specified devices.

### endpoint-installers

Manage endpoint installers

- **`watchguard-cli endpoint-installers`** - Retrieves the URL to download an installation package that installs Panda software with the specified managed configuration settings on a device.

### endpoint-licenses

Manage endpoint licenses

- **`watchguard-cli endpoint-licenses`** - Retrieves license information for Panda products associated with your WatchGuard Cloud account.

### endpoint-patches

Manage endpoint patches

- **`watchguard-cli endpoint-patches`** - Retrieves published patches not installed on the devices on the network.

### endpoint-risks

Manage endpoint risks

- **`watchguard-cli endpoint-risks company-summary`** - Retrieves a summary of the company risk status.
- **`watchguard-cli endpoint-risks detected`** - Retrieves a count of detected devices for each type of risk.
- **`watchguard-cli endpoint-risks devices`** - Retrieves a count of risks detected on each device by risk level. The response to this request is limited to 3,000 records.
- **`watchguard-cli endpoint-risks get-config`** - Retrieves the risk configuration.
- **`watchguard-cli endpoint-risks statistics`** - Retrieves an overview of the number and types of risks detected over time.
- **`watchguard-cli endpoint-risks update-config`** - Modifies the risk configuration.

### endpoint-security-events

Manage endpoint security events

- **`watchguard-cli endpoint-security-events counters`** - Retrieves counts of detected security events for the specified types.
- **`watchguard-cli endpoint-security-events export`** - Retrieves a list of security events of the specified type for the specified device. The response to this request is limited to 3,000 records.
- **`watchguard-cli endpoint-security-events overview`** - Retrieves counters for security overview.

### endpoint-software

Manage endpoint software

- **`watchguard-cli endpoint-software`** - Retrieves the software installed on the computers on the network.

### endpoint-tasks

Manage endpoint tasks

- **`watchguard-cli endpoint-tasks details`** - Retrieves the details of a specific task.
- **`watchguard-cli endpoint-tasks job-results`** - Retrieves the results of a task specific repetition.
- **`watchguard-cli endpoint-tasks job-status`** - Retrieves the status of task repetitions for a specific task.
- **`watchguard-cli endpoint-tasks jobs`** - Retrieves task repetitions for a specific task.
- **`watchguard-cli endpoint-tasks list`** - Retrieves a list of all tasks created.

### endpoint-unmanaged-devices

Manage endpoint unmanaged devices

- **`watchguard-cli endpoint-unmanaged-devices`** - Retrieves a list of unmanaged devices discovered on the network. The response to this request is limited to 3,000 records.

### firebox-aliases

Manage firebox aliases

- **`watchguard-cli firebox-aliases get`** - Retrieves the alias with the specified object ID.
- **`watchguard-cli firebox-aliases list`** - Retrieves a list of aliases for the account or for a specific device or template. If a *device* is specified as a query parameter, only the aliases for that device are retrieved. If a *templateid* is specified as a query parameter, only the aliases for that template are retrieved.

### firebox-auth-groups

Manage firebox auth groups

- **`watchguard-cli firebox-auth-groups get`** - Retrieves the auth group with the specified object ID.
- **`watchguard-cli firebox-auth-groups list`** - Retrieves a list of auth groups for the account or for a specific device or template. If a *device* is specified as a query parameter, only the auth groups for that device are retrieved. If a *templateid* is specified as a query parameter, only the auth groups for that template are retrieved.

### firebox-authpoint-user-groups

Manage firebox authpoint user groups

- **`watchguard-cli firebox-authpoint-user-groups get`** - Retrieves the AuthPoint user group with the specified object ID.
- **`watchguard-cli firebox-authpoint-user-groups list`** - Retrieves a list of AuthPoint user groups.

### firebox-blocked-sites

Manage firebox blocked sites

- **`watchguard-cli firebox-blocked-sites create`** - Creates a new blocked sites exception. The value of the action parameter specifies whether the Firebox allows or blocks traffic for the site.
- **`watchguard-cli firebox-blocked-sites delete`** - Deletes the specified blocked sites exception.
- **`watchguard-cli firebox-blocked-sites get`** - Retrieves the specified blocked sites exception.
- **`watchguard-cli firebox-blocked-sites list`** - Retrieves blocked sites exceptions saved in the specified WatchGuard Cloud account.
- **`watchguard-cli firebox-blocked-sites update`** - Updates the specified blocked sites exception.

### firebox-botnet-exceptions

Manage firebox botnet exceptions

- **`watchguard-cli firebox-botnet-exceptions create`** - Creates a new botnet site exception.
- **`watchguard-cli firebox-botnet-exceptions delete`** - Deletes the specified botnet site exception.
- **`watchguard-cli firebox-botnet-exceptions get`** - Retrieves the specified botnet site exception.
- **`watchguard-cli firebox-botnet-exceptions list`** - Retrieves botnet site exceptions saved in the specified WatchGuard Cloud account.
- **`watchguard-cli firebox-botnet-exceptions update`** - Updates the specified botnet site exception.

### firebox-bovpn-p1-settings

Manage firebox bovpn p1 settings

- **`watchguard-cli firebox-bovpn-p1-settings create`** - Creates a shared P1 settings object. One object can exist per Firebox. The Firebox ID is specified in the *device* attribute of the configuration object.
- **`watchguard-cli firebox-bovpn-p1-settings get`** - Retrieves the BOVPN IPSec P1 shared settings object for the given device ID. If no *device* is provided, all P1 shared settings objects for the account are retrieved.
- **`watchguard-cli firebox-bovpn-p1-settings update`** - Updates the specified BOVPN IPSec P1 shared settings object for the device. The Firebox ID is specified in the *device* attribute of the configuration object.

### firebox-bovpn-tunnels

Manage firebox bovpn tunnels

- **`watchguard-cli firebox-bovpn-tunnels create`** - Creates a BOVPN IPSec tunnel in the given account. All tunnels require at least one cloud-managed Firebox endpoint. Supports both route-based and policy-based tunnels.
- **`watchguard-cli firebox-bovpn-tunnels delete`** - Deletes the BOVPN IPSec tunnel with the given object ID.
- **`watchguard-cli firebox-bovpn-tunnels delete-by-name`** - Deletes the BOVPN IPSec tunnel with the given tunnel name.
- **`watchguard-cli firebox-bovpn-tunnels get`** - Retrieves the BOVPN IPSec tunnel with the given object ID.
- **`watchguard-cli firebox-bovpn-tunnels list`** - Given an account ID, retrieves a list of BOVPN tunnels for the account or for a specific device. If a *device* is specified as a query parameter, only the BOVPN tunnels for that device are retrieved.
- **`watchguard-cli firebox-bovpn-tunnels patch`** - Update the routes list or certificate of a specific BOVPN tunnel.
- **`watchguard-cli firebox-bovpn-tunnels patch-by-name`** - Update the routes list or certificate of a specific BOVPN tunnel.
- **`watchguard-cli firebox-bovpn-tunnels update`** - Updates the BOVPN IPSec tunnel with the given object ID.
- **`watchguard-cli firebox-bovpn-tunnels update-by-name`** - Update a BOVPN IPSec tunnel specified by its name through a URL parameter.

### firebox-certificates

Manage firebox certificates

- **`watchguard-cli firebox-certificates create`** - Creates an account-level or device-level certificate. Specify the *device* ID in the request data for device-level certificates.
- **`watchguard-cli firebox-certificates delete`** - Deletes the certificate with the given object ID.
- **`watchguard-cli firebox-certificates get`** - Retrieves the certificate with the given object ID.
- **`watchguard-cli firebox-certificates install`** - Install a certificate on a Firebox or a list of Fireboxes. The certificate to install and the target Firebox(es) are specified in the request body. The certificate can be referenced by name or object id.
- **`watchguard-cli firebox-certificates list`** - Retrieves a list of certificates for the account. Or retrieves a list of all certificates for a device when you specify *device*.

### firebox-content-filtering

Manage firebox content filtering

- **`watchguard-cli firebox-content-filtering get`** - Retrieves the content filtering rule with the specified object ID.
- **`watchguard-cli firebox-content-filtering list`** - Retrieves a list of content filtering rules for the account or for a specific device or template. If a *device* is specified as a query parameter, only the content filtering rules for that device are retrieved. If a *templateid* is specified as a query parameter, only the content filtering rules for that template are retrieved.

### firebox-deployments

Manage firebox deployments

- **`watchguard-cli firebox-deployments`** - Deploys exceptions or full configurations saved in WatchGuard Cloud to the specified devices.

### firebox-devices

Manage firebox devices

- **`watchguard-cli firebox-devices`** - Retrieves a list of Fireboxes, FireClusters, and Access Points for the specified WatchGuard Cloud account. You can specify a type of device in the *device_type* parameter, to filter the list with Fireboxes and FireClusters (FB) or Access Points (AP) details. To get details of a specific device, specify the device ID in the *device* parameter. For Service Provider accounts, set *include_hierarchy* to true to include devices from Subscriber accounts. Subscriber accounts ignore this parameter. The *device* and *include_hierarchy* parameters are mutually exclusive.

### firebox-exceptions

Manage firebox exceptions

- **`watchguard-cli firebox-exceptions disable-global`** - Disables the ability to manage and deploy global exceptions with the Firebox Management API for the specified device(s). You can specify whether to retain or delete exceptions on the device(s).
- **`watchguard-cli firebox-exceptions enable-global`** - Enables the ability to manage and deploy exceptions with the Firebox Management API for the specified device(s).
- **`watchguard-cli firebox-exceptions list`** - Retrieves exceptions saved in the specified WatchGuard Cloud account.

### firebox-file-exceptions

Manage firebox file exceptions

- **`watchguard-cli firebox-file-exceptions create`** - Creates a new file exception.
- **`watchguard-cli firebox-file-exceptions delete`** - Deletes the specified file exception.
- **`watchguard-cli firebox-file-exceptions get`** - Retrieves the specified file exception.
- **`watchguard-cli firebox-file-exceptions list`** - Retrieves file exceptions saved in the specified WatchGuard Cloud account.
- **`watchguard-cli firebox-file-exceptions update`** - Updates the specified file exception.

### firebox-geo-exceptions

Manage firebox geo exceptions

- **`watchguard-cli firebox-geo-exceptions create`** - Creates a new Geolocation exception.
- **`watchguard-cli firebox-geo-exceptions delete`** - Deletes the specified Geolocation exception.
- **`watchguard-cli firebox-geo-exceptions get`** - Retrieves the specified Geolocation exception.
- **`watchguard-cli firebox-geo-exceptions list`** - Retrieves all Geolocation exceptions saved in the specified WatchGuard Cloud account.
- **`watchguard-cli firebox-geo-exceptions update`** - Updates the specified Geolocation exception.

### firebox-groups

Manage firebox groups

- **`watchguard-cli firebox-groups get`** - Retrieves the group with the specified object ID.
- **`watchguard-cli firebox-groups list`** - Retrieves a list of groups for the account or for a specific device or template. If a *device* is specified as a query parameter, only the groups for that device are retrieved. If a *templateid* is specified as a query parameter, only the groups for that template are retrieved.

### firebox-ips-exceptions

Manage firebox ips exceptions

- **`watchguard-cli firebox-ips-exceptions create`** - Creates a new IPS signature exception.
- **`watchguard-cli firebox-ips-exceptions delete`** - Deletes the specified IPS signature exception.
- **`watchguard-cli firebox-ips-exceptions get`** - Retrieves the specified IPS signature exception.
- **`watchguard-cli firebox-ips-exceptions list`** - Retrieves all IPS signature exceptions.
- **`watchguard-cli firebox-ips-exceptions update`** - Updates the specified IPS signature exception.

### firebox-networks

Manage firebox networks

- **`watchguard-cli firebox-networks get`** - Retrieves the network with the specified object ID.
- **`watchguard-cli firebox-networks list`** - Retrieves a list of networks for the account or for a specific device or template. If a *device* is specified as a query parameter, only the networks for that device are retrieved. If a *templateid* is specified as a query parameter, only the networks for that template are retrieved.

### firebox-policies

Manage firebox policies

- **`watchguard-cli firebox-policies create`** - Creates a new firewall policy for a device. Specify the *group* parameter to indicate which policy category to add the policy to.
- **`watchguard-cli firebox-policies delete`** - Deletes the firewall policy with the specified object ID. Permanent policies cannot be deleted.
- **`watchguard-cli firebox-policies get`** - Retrieves the firewall policy with the specified object ID.
- **`watchguard-cli firebox-policies list`** - Retrieves a list of firewall policies for the account or for a specific device. When you specify *device* as a query parameter, this operation retrieves only the firewall policies for that device. When you specify *full* (set to 0) as a query parameter, this operation returns a summary list (id, name, group, and device) of firewall policies. When *full* (set to 1) or not specified, returns full policy objects.
- **`watchguard-cli firebox-policies set-enabled`** - Enables or disables the firewall policy with the specified object ID without modifying other settings.
- **`watchguard-cli firebox-policies update`** - Updates the firewall policy with the specified object ID.

### firebox-policy-groups

Manage firebox policy groups

- **`watchguard-cli firebox-policy-groups get`** - Retrieves the policy group with the specified object ID.
- **`watchguard-cli firebox-policy-groups list`** - Retrieves a list of policy groups for the account or for a specific device or template. If a *device* is specified as a query parameter, only the policy groups for that device are retrieved. If a *templateid* is specified as a query parameter, only the policy groups for that template are retrieved.

### firebox-reports

Manage firebox reports

- **`watchguard-cli firebox-reports executive`** - Retrieves the specified Executive Dashboard records for the specified devices.
- **`watchguard-cli firebox-reports security`** - Retrieves the specified Security Dashboard records for the specified devices.

### firebox-saml-user-groups

Manage firebox saml user groups

- **`watchguard-cli firebox-saml-user-groups get`** - Retrieves the SAML user group with the specified object ID.
- **`watchguard-cli firebox-saml-user-groups list`** - Retrieves a list of SAML user groups.

### firebox-schedules

Manage firebox schedules

- **`watchguard-cli firebox-schedules get`** - Retrieves the schedule with the specified object ID.
- **`watchguard-cli firebox-schedules list`** - Retrieves a list of schedules for the account or for a specific device or template. If a *device* is specified as a query parameter, only the schedules for that device are retrieved. If a *templateid* is specified as a query parameter, only the schedules for that template are retrieved.

### firebox-sdwans

Manage firebox sdwans

- **`watchguard-cli firebox-sdwans get`** - Retrieves the SD-WAN with the specified object ID.
- **`watchguard-cli firebox-sdwans list`** - Retrieves a list of SD-WANs for the account or for a specific device or template. If a *device* is specified as a query parameter, only the SD-WANs for that device are retrieved. If a *templateid* is specified as a query parameter, only the SD-WANs for that template are retrieved.

### firebox-snat-actions

Manage firebox snat actions

- **`watchguard-cli firebox-snat-actions get`** - Retrieves the SNAT action with the specified object ID.
- **`watchguard-cli firebox-snat-actions list`** - Retrieves a list of SNAT actions for the account or for a specific device or template. If a *device* is specified as a query parameter, only the SNAT actions for that device are retrieved. If a *templateid* is specified as a query parameter, only the SNAT actions for that template are retrieved.

### firebox-templates

Manage firebox templates

- **`watchguard-cli firebox-templates list-subscriptions`** - With no query parameters, returns templates available to the account, including inherited templates.  Retrieves a list of templates subscribed by a Firebox when the *device* parameter is specified as a query parameter. If a *templateid* is specified as a query parameter, a list of Fireboxes subscribed to a template are retrieved. The *device* and *templateid* parameters are mutually exclusive.
- **`watchguard-cli firebox-templates subscribe`** - Subscribes, unsubscribes, or changes the order of Firebox templates on a Firebox. Specify only the template IDs in the *templates* parameter that you want the Firebox to subscribe to. The Firebox is unsubscribed from templates that you do not include. An empty *templates* array unsubscribes the Firebox from all templates. Specify template IDs in the order of priority to apply on the Firebox.

### firebox-traffic-shaping-rules

Manage firebox traffic shaping rules

- **`watchguard-cli firebox-traffic-shaping-rules get`** - Retrieves the traffic shaping rule with the specified object ID.
- **`watchguard-cli firebox-traffic-shaping-rules list`** - Retrieves a list of traffic shaping rules for the account or for a specific device or template. If a *device* is specified as a query parameter, only the traffic shaping rules for that device are retrieved. If a *templateid* is specified as a query parameter, only the traffic shaping rules for that template are retrieved.

### firebox-traffic-types

Manage firebox traffic types

- **`watchguard-cli firebox-traffic-types get`** - Retrieves the traffic type with the specified object ID.
- **`watchguard-cli firebox-traffic-types list`** - Retrieves a list of traffic types for the account or for a specific device or template. If a *device* is specified as a query parameter, only the traffic types for that device are retrieved. If a *templateid* is specified as a query parameter, only the traffic types for that template are retrieved.

### firebox-transactions

Manage firebox transactions

- **`watchguard-cli firebox-transactions cancel`** - Deletes the specified transaction record.
- **`watchguard-cli firebox-transactions get`** - Retrieves the specified transaction record, which includes details of a specific deployment.
- **`watchguard-cli firebox-transactions list`** - Retrieves a list of transaction records, which include details of individual deployments.
- **`watchguard-cli firebox-transactions update`** - Updates the specified transaction record.

### firebox-users

Manage firebox users

- **`watchguard-cli firebox-users get`** - Retrieves the user with the specified object ID.
- **`watchguard-cli firebox-users list`** - Retrieves a list of users for the account or for a specific device or template. If a *device* is specified as a query parameter, only the users for that device are retrieved. If a *templateid* is specified as a query parameter, only the users for that template are retrieved.

### firebox-webblocker-exceptions

Manage firebox webblocker exceptions

- **`watchguard-cli firebox-webblocker-exceptions create`** - Creates a new WebBlocker exception.
- **`watchguard-cli firebox-webblocker-exceptions delete`** - Deletes the specified WebBlocker exception.
- **`watchguard-cli firebox-webblocker-exceptions get`** - Retrieves the specified WebBlocker exception.
- **`watchguard-cli firebox-webblocker-exceptions list`** - Retrieves all WebBlocker exceptions.
- **`watchguard-cli firebox-webblocker-exceptions update`** - Updates the specified WebBlocker exception.

### firecloud-blocked-sites

Manage firecloud blocked sites

- **`watchguard-cli firecloud-blocked-sites create`** - Creates a new blocked sites exception.
- **`watchguard-cli firecloud-blocked-sites delete`** - Deletes the specified blocked sites exception.
- **`watchguard-cli firecloud-blocked-sites get`** - Retrieves the specified blocked sites exception.
- **`watchguard-cli firecloud-blocked-sites list`** - Retrieves all blocked sites exceptions in the specified FireCloud account.
- **`watchguard-cli firecloud-blocked-sites update`** - Updates the specified blocked sites exception.

### firecloud-botnet-exceptions

Manage firecloud botnet exceptions

- **`watchguard-cli firecloud-botnet-exceptions create`** - Creates a new botnet site exception.
- **`watchguard-cli firecloud-botnet-exceptions delete`** - Deletes the specified botnet site exception.
- **`watchguard-cli firecloud-botnet-exceptions get`** - Retrieves the specified botnet site exception.
- **`watchguard-cli firecloud-botnet-exceptions list`** - Retrieves all botnet site exceptions in the specified FireCloud account.
- **`watchguard-cli firecloud-botnet-exceptions update`** - Updates the specified botnet site exception.

### firecloud-exceptions

Manage firecloud exceptions

- **`watchguard-cli firecloud-exceptions`** - Retrieves all exceptions in the specified FireCloud account.

### firecloud-file-exceptions

Manage firecloud file exceptions

- **`watchguard-cli firecloud-file-exceptions create`** - Creates a new file exception.
- **`watchguard-cli firecloud-file-exceptions delete`** - Deletes the specified file exception.
- **`watchguard-cli firecloud-file-exceptions get`** - Retrieves the specified file exception.
- **`watchguard-cli firecloud-file-exceptions list`** - Retrieves all file exceptions in the specified FireCloud account.
- **`watchguard-cli firecloud-file-exceptions update`** - Updates the specified file exception.

### firecloud-geo-exceptions

Manage firecloud geo exceptions

- **`watchguard-cli firecloud-geo-exceptions create`** - Creates a new Geolocation exception.
- **`watchguard-cli firecloud-geo-exceptions delete`** - Deletes the specified Geolocation exception.
- **`watchguard-cli firecloud-geo-exceptions get`** - Retrieves the specified Geolocation exception.
- **`watchguard-cli firecloud-geo-exceptions list`** - Retrieves all Geolocation exceptions in the specified FireCloud account.
- **`watchguard-cli firecloud-geo-exceptions update`** - Updates the specified Geolocation exception.

### firecloud-https-exceptions

Manage firecloud https exceptions

- **`watchguard-cli firecloud-https-exceptions create`** - Creates a new HTTPS decryption exception.
- **`watchguard-cli firecloud-https-exceptions delete`** - Deletes the specified HTTPS decryption exception.
- **`watchguard-cli firecloud-https-exceptions get`** - Retrieves the specified HTTPS decryption exception.
- **`watchguard-cli firecloud-https-exceptions list`** - Retrieves all HTTPS decryption exceptions in the specified FireCloud account.
- **`watchguard-cli firecloud-https-exceptions update`** - Updates the specified HTTPS decryption exception.

### firecloud-ips-exceptions

Manage firecloud ips exceptions

- **`watchguard-cli firecloud-ips-exceptions create`** - Creates a new IPS signature exception.
- **`watchguard-cli firecloud-ips-exceptions delete`** - Deletes the specified IPS signature exception.
- **`watchguard-cli firecloud-ips-exceptions get`** - Retrieves the specified IPS signature exception.
- **`watchguard-cli firecloud-ips-exceptions list`** - Retrieves all IPS signature exceptions in the specified FireCloud account.
- **`watchguard-cli firecloud-ips-exceptions update`** - Updates the specified IPS signature exception.

### firecloud-webblocker-exceptions

Manage firecloud webblocker exceptions

- **`watchguard-cli firecloud-webblocker-exceptions create`** - Creates a new WebBlocker exception.
- **`watchguard-cli firecloud-webblocker-exceptions delete`** - Deletes the specified WebBlocker exception.
- **`watchguard-cli firecloud-webblocker-exceptions get`** - Retrieves the specified WebBlocker exception.
- **`watchguard-cli firecloud-webblocker-exceptions list`** - Retrieves all WebBlocker exceptions in the specified FireCloud account.
- **`watchguard-cli firecloud-webblocker-exceptions update`** - Updates the specified WebBlocker exception.

### ndr-assets

Manage ndr assets

- **`watchguard-cli ndr-assets create`** - Creates an asset.
- **`watchguard-cli ndr-assets delete`** - Deletes an asset specified by asset ID.
- **`watchguard-cli ndr-assets get`** - Retrieves details for an asset specified by asset ID.
- **`watchguard-cli ndr-assets list`** - Retrieves a list of all the assets for the specified tenant. To filter the list, you can specify the *field* and *value* parameters.
- **`watchguard-cli ndr-assets update`** - Updates an asset detail specified by asset ID.

### ndr-smart-alerts

Manage ndr smart alerts

- **`watchguard-cli ndr-smart-alerts close`** - Closes a Smart Alert specified by a Smart Alert ID. You can also specify the reason to close the Smart Alert.
- **`watchguard-cli ndr-smart-alerts get`** - Retrieves details of a Smart Alert specified by a Smart Alert ID.

### operators

Manage operators

- **`watchguard-cli operators create`** - Creates new operators in the specified account. In the request body, specify the details of the operators to create.
- **`watchguard-cli operators delete`** - Deletes operators from the specified account. In the request body, specify the details of the operators to delete.
- **`watchguard-cli operators list`** - To get details of all operators for an account, specify the account ID in the query parameter.
- **`watchguard-cli operators transaction-status`** - To get the status of a transaction, specify the transaction ID in the query parameter.
- **`watchguard-cli operators update`** - Updates existing operators in the specified account. In the request body, specify the details of the operators to update.

### threatsync-actions

Manage threatsync actions

- **`watchguard-cli threatsync-actions list`** - Retrieves a list of actions.
- **`watchguard-cli threatsync-actions perform`** - Performs a specified action.

### threatsync-comments

Manage threatsync comments

- **`watchguard-cli threatsync-comments create`** - Create comment for a specific incident.
- **`watchguard-cli threatsync-comments delete`** - Deletes a specific comment.
- **`watchguard-cli threatsync-comments list`** - Retrieves a list of comments for a specific incident.
- **`watchguard-cli threatsync-comments update`** - Updates a specific comment.

### threatsync-incident-actions

Manage threatsync incident actions

- **`watchguard-cli threatsync-incident-actions get`** - Retrieves details of a specific action for an incident.
- **`watchguard-cli threatsync-incident-actions list`** - Retrieves a list of actions for a specific incident.
- **`watchguard-cli threatsync-incident-actions perform`** - Performs an action for a specific incident.

### threatsync-incidents

Manage threatsync incidents

- **`watchguard-cli threatsync-incidents get`** - Retrieves a specific incident by incident ID.
- **`watchguard-cli threatsync-incidents list`** - Retrieves a list of specific ThreatSync incidents for an account.
- **`watchguard-cli threatsync-incidents set-status`** - Updates the status of a specific incident.

### threatsync-transactions

Manage threatsync transactions

- **`watchguard-cli threatsync-transactions`** - Submits multiple requests to ThreatSync Management API endpoints at the same time.

### accounts

- **`watchguard-cli accounts health`** - One row per managed account: Fireboxes offline, endpoints at risk, open incidents by severity and licenses expiring soon (Service Provider accounts).

### allocations

- **`watchguard-cli allocations reconcile`** - Compare a Service Provider's Firebox allocations against the Fireboxes that actually report in, and summarize owned, allocated and available software licenses.

### endpoint-devices

- **`watchguard-cli endpoint-devices posture`** - List every endpoint that is unprotected, out of date, isolated, unlicensed or waiting on a reboot, across the accounts a Service Provider manages.

### endpoint-patches

- **`watchguard-cli endpoint-patches exposure`** - Group missing patches by CVE and criticality across managed accounts (needs WatchGuard Patch Management), showing which customers and devices are exposed.

### firebox-devices

- **`watchguard-cli firebox-devices changes`** - After a sync, see which Fireboxes, FireClusters and Access Points went offline, came back, appeared, disappeared or changed firmware since the previous sync.

- **`watchguard-cli firebox-devices firmware-lag`** - From synced devices, find Fireboxes and Access Points running older firmware than the newest version seen for the same model in your fleet.

### firebox-transactions

- **`watchguard-cli firebox-transactions wait`** - Block until a Firebox deployment or certificate-install transaction finishes: exit 0 on complete, 8 if it ended without completing (failed, timed out or canceled), 9 if still running at --max-wait.

### threatsync-incidents

- **`watchguard-cli threatsync-incidents queue`** - See open ThreatSync incidents across your managed accounts, ranked by severity, threat score and age, with untouched incidents and recommended-but-not-taken actions called out.

### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`watchguard-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`watchguard-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`watchguard-cli learnings list`** - Inspect taught rows
- **`watchguard-cli learnings forget <query>`** - Undo a teach
- **`watchguard-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`watchguard-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`watchguard-cli teach-pattern`** - Install a query/resource template up front
- **`watchguard-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `WATCHGUARD_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `watchguard-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
watchguard-cli accounts get

# JSON for scripting and agents
watchguard-cli accounts get --json
# Filter to specific fields
watchguard-cli accounts get --json --select accountInfo

# Dry run  -  show the request without sending
watchguard-cli accounts get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
watchguard-cli accounts get --agent
```

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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit) - for `firebox-transactions wait`, the deployment ended without completing (failed, timed out or canceled), `9` `firebox-transactions wait` only: still running when `--max-wait` elapsed, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `WATCHGUARD_ACCOUNT_ID` resolves `{accountId}`

Base URL: `https://api.usa.cloud.watchguard.com`

## Health Check

```bash
watchguard-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `watchguard-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/watchguard-cli/config.toml`; `--home`, `WATCHGUARD_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `WATCHGUARD_CLIENT_ID` | auth_flow_input | Yes | WatchGuard Cloud API Access ID (read-only or read-write). |
| `WATCHGUARD_CLIENT_SECRET` | auth_flow_input | Yes | API password for that Access ID. |
| `WATCHGUARD_API_KEY` | per_call | Yes | API key, sent as the WatchGuard-API-Key header. |
| `WATCHGUARD_ACCOUNT_ID` | endpoint | Yes | Your account ID (ACC-... or WGC-...); `--account-id` overrides it. |
| `WATCHGUARD_BASE_URL` | endpoint | No | Region API host; default https://api.usa.cloud.watchguard.com (EMEA: api.deu, APAC: api.jpn). |
| `WATCHGUARD_TOKEN_URL` | endpoint | No | Region token URL; default https://api.usa.cloud.watchguard.com/oauth/token. |
| `WATCHGUARD_AUDIENCE` | per_call | No | Send every request with one managed account's token (value from `audiences --account-id`). A convenience, not a security boundary: the audience token also works on your own account. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `watchguard-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `watchguard-cli doctor` to check credentials
- Verify the environment variable is set: `echo $WATCHGUARD_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Missing Authorization Token and/or Watchguard-Api-Key Header**  -  Set WATCHGUARD_API_KEY plus WATCHGUARD_CLIENT_ID and WATCHGUARD_CLIENT_SECRET, then run watchguard-cli doctor
- **OAuth client_credentials mint: HTTP 400 or 401 from /oauth/token**  -  Use the Access ID as WATCHGUARD_CLIENT_ID and its API password (not your login password) as WATCHGUARD_CLIENT_SECRET; region must match WATCHGUARD_TOKEN_URL
- **403 or empty results for an EMEA or APAC account**  -  export WATCHGUARD_BASE_URL=https://api.deu.cloud.watchguard.com WATCHGUARD_TOKEN_URL=https://api.deu.cloud.watchguard.com/oauth/token (use jpn for APAC)
- **403 on a Subscriber account's resources from a Service Provider credential**  -  Run 'watchguard-cli audiences --account-id ACC-1234567', then export WATCHGUARD_AUDIENCE=<audience> before the command
- **429 Too Many Requests**  -  The quota is 500 requests per second and 200,000 per day per API key; sync less often or narrow --resources

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**eduardogarcot/wg-visualizer**](https://github.com/eduardogarcot/wg-visualizer)  -  TypeScript (1 stars)
- [**WYRE-AI/watchguard-cloud-mcp**](https://github.com/WYRE-AI/watchguard-cloud-mcp)  -  TypeScript
- [**eliazv/watchguard-acme-deploy (wgcert)**](https://github.com/eliazv/watchguard-acme-deploy)  -  Go

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
