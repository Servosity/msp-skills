# Auvik CLI

**Every Auvik endpoint as a command, plus the cross-client answers the Auvik UI and API cannot give you.**

Auvik holds the richest network-truth dataset an MSP has, behind a read-only JSON:API with bracketed filters, cursor pagination, and per-region hosts. Every existing tool is a language binding that hands you typed structs and leaves the question unanswered. This CLI mirrors Auvik into local SQLite so you can ask things no Auvik surface supports: what is end-of-life across every client (eol), which devices have no configuration backup at all (configuration audit), and which devices disappeared since last sync (inventory diff) - a removal Auvik never reports.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer places the `auvik-cli` and `auvik-mcp` binaries and registers the skill with your agent:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/auvik/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/auvik/install.ps1 | iex
   ```
3. Verify: `auvik-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=auvik). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.


## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `AUVIK_USERNAME` (your Auvik user email) and `AUVIK_API_KEY` when Claude Desktop prompts you. Set `AUVIK_BASE_URL` to your region's host (`us1`, `us2`, `eu1`, ...) if you are not on `us1`.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/auvik/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/auvik/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "auvik": {
      "command": "auvik-mcp",
      "env": {
        "AUVIK_USERNAME": "<your-auvik-user-email>",
        "AUVIK_API_KEY": "<your-auvik-api-key>",
        "AUVIK_BASE_URL": "https://auvikapi.us1.my.auvik.com"
      }
    }
  }
}
```

</details>

## Authentication

Auvik uses HTTP Basic authentication: your Auvik user email is the username and your API key is the password. Both are required - there is no single-token form. Save them with 'auvik-cli auth set-credentials <your-email> <your-api-key>', or export AUVIK_USERNAME and AUVIK_API_KEY. Check state any time with 'auvik-cli auth status'.

Set your region before your first call. The base URL is per-region and the built-in default is us1, which is WRONG for every tenant outside us1 - and a valid key against the wrong region returns 401 exactly like a bad key. Find your region in the URL of your Auvik dashboard and export AUVIK_BASE_URL to match: https://auvikapi.<region>.my.auvik.com where <region> is one of us1, us2, us3, us4, eu1, ca1, au1.

The API user also needs an appropriate role in every tenant you query, or that tenant returns 403 while others succeed.

## Quick Start

```bash
# Confirm the CLI is wired up before adding credentials
auvik-cli doctor --dry-run

# Prove auth and region are right by listing the clients you can see
auvik-cli tenants list --agent

# Build the local mirror the cross-client commands read from
auvik-cli sync --resources tenants,inventory,auvik-inventory-lifecycle,inventory-device-warranty --full

# The headline question: what is aging out across every client
auvik-cli eol --agent

# And which devices are not backed up at all
auvik-cli configuration audit --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-client answers Auvik cannot give
- **`eol`**  -  See every device approaching or past end-of-support across all your clients at once, bucketed by urgency.

  _Reach for this when asked what hardware is aging out across a book of business  -  it is the one question Auvik's UI cannot answer at all._

  ```bash
  auvik-cli eol --agent
  ```
- **`changes`**  -  Merge config revisions, audit entries, notes, and alerts for one device into a single chronological story.

  _Use this to answer 'what happened to this device and who touched it' without opening four screens._

  ```bash
  auvik-cli changes 5f2b91c4 --agent
  ```
- **`inventory diff`**  -  List devices added, removed, or changed fleet-wide since the last sync, attributed to each client.

  _Reach for this when a device count moved and you need to know which devices caused it._

  ```bash
  auvik-cli inventory diff --since 7d --agent
  ```

### Config integrity at fleet scale
- **`configuration audit`**  -  Find devices with no configuration backup, a stale backup, or no running-config backup, across every client.

  _Reach for this to prove which devices are unprotected across a whole book of business; it cannot search config text, because the API does not expose it._

  ```bash
  auvik-cli configuration audit --agent
  ```
- **`device discovery-gaps`**  -  List every device Auvik cannot fully poll, per client, with the credential state behind each gap.

  _Answers 'why can't we see this box' in one command instead of three screens._

  ```bash
  auvik-cli device discovery-gaps --agent
  ```

### Billing and count integrity
- **`usage reconcile`**  -  Put each client's billable usage count next to the actual synced inventory and show the device rows behind the difference.

  _Use this before invoicing when a client's billable count disagrees with the agreement._

  ```bash
  auvik-cli usage reconcile --agent
  ```
- **`asm shadow`**  -  Surface SaaS apps with active users but no license record, and licenses nobody is using, per client.

  _Reach for this when building a client's software-spend narrative._

  ```bash
  auvik-cli asm shadow --agent
  ```

### Alert memory
- **`alert noise`**  -  Rank devices and clients by alert volume over a window, with device names, types, clients and severity mix resolved.

  _Use this for the shift handoff or the chronic-offender conversation. Auvik publishes no dismissal timestamp, so this reports dismissal counts, not time-to-dismiss._

  ```bash
  auvik-cli alert noise --since 30d --agent
  ```

## Recipes

### End-of-support exposure across the whole book

```bash
auvik-cli eol --agent --select rows.client,rows.device_name,rows.make_model,rows.last_support_date
```

The quarterly-business-review answer in one line. The report is an envelope, so select through 'rows.' to narrow to the four fields that go on the slide (Auvik exposes no single end-of-life date; `last_support_date` is the support-lifecycle one).

### Prove which devices are not backed up

```bash
auvik-cli configuration audit --finding no_backup --agent
```

Auvik's API exposes backup metadata but not config bodies, so this is the fleet-wide compliance question it can answer: which devices have no configuration backup at all.

### Narrow a deeply nested JSON:API device response

```bash
auvik-cli device list --agent --select attributes.deviceName,attributes.deviceType,attributes.onlineStatus
```

Auvik wraps everything in JSON:API resource objects. The CLI strips the outer envelope, so select from 'attributes.' to cut a multi-KB device list down to the three fields you actually read.

### Explain a billable-count change before invoicing

```bash
auvik-cli usage reconcile --agent
```

Shows the device rows behind each client's count delta instead of just the number the usage endpoint returns.

### Reconstruct what happened to one device

```bash
auvik-cli changes 5f2b91c4 --agent
```

Merges config revisions, audit entries, notes, and alerts for that device into one chronological stream.

## Usage

Run `auvik-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `AUVIK_CONFIG_DIR`, `AUVIK_DATA_DIR`, `AUVIK_STATE_DIR`, or `AUVIK_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `AUVIK_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export AUVIK_HOME=/srv/auvik
auvik-cli doctor
```

Under `AUVIK_HOME=/srv/auvik`, the four dirs resolve to `/srv/auvik/config`, `/srv/auvik/data`, `/srv/auvik/state`, and `/srv/auvik/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "auvik": {
      "command": "auvik-mcp",
      "env": {
        "AUVIK_HOME": "/srv/auvik"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `AUVIK_DATA_DIR` overrides an explicit `--home` for that kind. Use `AUVIK_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `AUVIK_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `auvik-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### alert

The Auvik Alert API allows you to dismiss an alert that Auvik has triggered.

There is a single endpoint availble within the Alert API.

- Dismiss Alert. Dismiss a single alert.

- **`auvik-cli alert dismiss-single`** - Use the Dismiss Alert API to dismiss a specific alert that Auvik has triggered
- **`auvik-cli alert read-multiple-info`** - Use the Read Multiple Alerts’ Info API to pull the collected information about the various alerts that Auvik has triggered.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli alert read-single-info`** - Use the Read Single Alert’s Info API to pull the collected information about a specific alert that Auvik has triggered.

To find the alert IDs, run the Read Multiple Alerts API.

### asm

The Auvik SaaS Management API allows you to access all SaaS related data collected via the Auvik SaaS Management product. The Auvik SaaS Management API includes all core data included as a part of the product including Applications, Monitored Users, Security Logs, and other key data.

- **`auvik-cli asm read-multiple-asmapp-info`** - Use the Read Multiple ASM Applications' Info to retrieve the information related to the SaaS applications discovered within an ASM client deployment. This data includes the number of users of an application as well as key metadata such as whether the application is approved or unapproved.
- **`auvik-cli asm read-multiple-asmclient-info`** - The Read Multiple ASM Clients' Info API returns relevant meta data about clients that exist within the Auvik SaaS Management product. The response includes the number of total users as well as clientIDs which may be used to filter other relevant API calls.
- **`auvik-cli asm read-multiple-asmlicense-info`** - Use the Read Multiple ASM Licenses' Info endpoint to retrieve information about an application licenses discovered within an ASM client deployment. This data includes the license type and associated user email assigned to each license, alongside other key metadata such as last login time. To access this data, a SaaS Ops integration with the desired application must be configured.
- **`auvik-cli asm read-multiple-asmsecurity-log-info`** - Use the Read Multiple ASM Security Logs' Info API to retrieve in depth information about the security logs within an Auvik SaaS Management client. Security logs include specific SaaS events, such as logins or downloads with timestamps for each event.
- **`auvik-cli asm read-multiple-asmtag-info`** - Use the Read Multiple ASM Tags' Info API to retrieve information about tags configured within an ASM client.
- **`auvik-cli asm read-multiple-asmuser-info`** - Use the Read Multiple ASM Users' Info API to retrieve information about any monitored users that exist within a specific Auvik SaaS Management tenant. ASM user data includes core information about the SaaS applications a monitored user is using as well as which accounts a user is leveraging to log into those applications.

### authentication

Manage authentication

- **`auvik-cli authentication`** - Use the Verify Credentials API to verify your credentials are correct before making a call to an endpoint.

### auvik-inventory

Manage auvik inventory

- **`auvik-cli auvik-inventory read-multiple-device-discovery-status-v2`** - Returns the discovery status of multiple devices for a tenant. You'll need the client ID for the client you want to fetch discovery statuses from.

To find the client ID, run the Read Multiple Tenants API.
- **`auvik-cli auvik-inventory read-multiple-device-info-v2`** - Use the Read Multiple Devices’ Info API to pull the collected information about the various devices Auvik has discovered. You’ll need the client ID for the client you want to fetch devices from. The response includes the devices from the children of the requested client.

To find the client ID, run the Read Multiple Tenants API.
- **`auvik-cli auvik-inventory read-multiple-device-lifecycle-v2`** - Returns lifecycle information for multiple devices for a tenant. You'll need the client ID for the client you want to fetch device lifecycle records from. The response includes devices from the children of the requested client.

To find the client ID, run the Read Multiple Tenants API.
- **`auvik-cli auvik-inventory read-multiple-interface-info-v2`** - Use the Read Multiple Interfaces’ Info API to pull the collected information about the various interfaces Auvik has discovered. You’ll need the client ID for the client you want to fetch interfaces from. The response includes the interfaces from the children of the requested client.

To find the client ID, run the Read Multiple Tenants API.
- **`auvik-cli auvik-inventory read-single-device-discovery-status-v2`** - Returns the discovery status of a single device. You will need the device ID for the specific device.
- **`auvik-cli auvik-inventory read-single-device-info-v2`** - Use the Read Single Device’s Info API to pull the collected information about a specific device Auvik has discovered. You’ll need the device ID for the specific device.

To find the device IDs, run the Read Multiple Devices API.
- **`auvik-cli auvik-inventory read-single-device-lifecycle-v2`** - Returns lifecycle information for a single device. You will need the device ID for the specific device.
- **`auvik-cli auvik-inventory read-single-interface-info-v2`** - Use the Read Single Interface’s Info API to pull the collected information about a specific interface Auvik has discovered. You’ll need the interface ID for the specific interface.

To find the interface IDs, run the Read Multiple Interfaces API.

### auvik-stat

Manage auvik stat

- **`auvik-cli auvik-stat`** - Use the Read Service Statistics API to fetch detailed statistics of a client's (and client's children if a multi-client) services for a given time range.

### billing

Manage billing

- **`auvik-cli billing read-client-usage`** - Use the Read Client Usage API to pull a summary of a client’s (and client’s children if a multi-client) usage for a given time range.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli billing read-device-usage`** - Use the Read Device Usage API to pull a summary of a client’s (and client’s children if a multi-client) usage for a given time range.

### inventory

Manage inventory

- **`auvik-cli inventory read-multiple-component-info`** - Use the Read Multiple Components’ Info API to pull collected information about various device components Auvik has discovered. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-configurations`** - Use the Read Multiple Device Configuration API to pull all device configurations. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-device-details`** - Use the Read Multiple Devices’ Details API to pull extra collected information about the various devices Auvik has discovered not already included in the Device Info API. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-device-extended-detail`** - Use the Read Multiple Devices’ Extended Details API to get many devices’ extended details. Many device types have information collected and tracked by Auvik that are unique to that device type. Use this endpoint to access such information for a given device. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-device-info`** - Use the Read Multiple Devices’ Info API to pull the collected information about the various devices Auvik has discovered. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-device-lifecycle`** - Use the Read Multiple Devices’ Lifecycle API to pull the collected lifecycle information about the various devices Auvik has discovered. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-device-warranty`** - Use the Read Multiple Devices’ Warranty API to pull the collected warranty information about the various devices Auvik has discovered. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-entity-audit`** - Use the Read Multiple Entity Audits API pull information about multiple entity audits for you clients. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-entity-note`** - Use the Read Multiple Entity Notes API pull information about multiple entity notes. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-interface-info`** - Use the Read Multiple Interfaces Info API to pull the collected information about the various device interfaces Auvik has discovered. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-network-details`** - Use the Read Multiple Networks’ Details API to pull extra collected information about the various networks Auvik has discovered not already included in the Network Info API. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-multiple-network-info`** - Use the Read Multiple Networks’ Info API to pull the collected information about the various networks Auvik has discovered. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-single-component-info`** - Use the Read Single Component’s Info API to pull collected information about a specific device component Auvik has discovered. You’ll need the component ID you want audit detail for.

To find the component IDs, run the Read Multiple Components’ Info API.
- **`auvik-cli inventory read-single-configuration`** - Use the Read Single Device Configuration API to pull a single device configuration. You’ll need the client IDs for the clients you want to run the multiple read against.

To find the client IDs, run the Read Multiple Tenants API.
- **`auvik-cli inventory read-single-device-details`** - Use the Read Single Device’s Details API to pull extra collected information about a specific device Auvik has discovered not already included in the Device Info API. You’ll need the device ID for the specific device.

To find the device IDs, run the Read Multiple Devices API.
- **`auvik-cli inventory read-single-device-extended-detail`** - Use the Read Single Device’s Extended Details API to get a device’s extended details. Many device types have information collected and tracked by Auvik that are unique to that device type. Use this endpoint to access such information for a given device.

To find the device IDs, run the Read Multiple Devices API.
- **`auvik-cli inventory read-single-device-info`** - Use the Read Single Device’s Info API to pull the collected information about a specific device Auvik has discovered. You’ll need the device ID for the specific device.

To find the device IDs, run the Read Multiple Devices API.
- **`auvik-cli inventory read-single-device-lifecycle`** - Use the Read Single Device’s Lifecycle Info API to pull the collected information about a specific device Auvik has discovered. You’ll need the device ID for the specific device.

To find the device IDs, run the Read Multiple Devices API.
- **`auvik-cli inventory read-single-device-warranty`** - Use the Read Single Device’s Warranty Info API to pull the collected information about a specific device Auvik has discovered. You’ll need the device ID for the specific device.

To find the device IDs, run the Read Multiple Devices API.
- **`auvik-cli inventory read-single-entity-audit`** - Use the Single Multiple Entity Audit API pull information about a single entity audit. You’ll need the audit entry ID for the specific audit.

To find the audit ID, run the Read Multiple Entity Audits API
- **`auvik-cli inventory read-single-entity-note`** - Use the Read Single Entity Note API to pull the information about a specific entity note. You’ll need the entity note ID for the specific entity note.

To find the note IDs the Read Multiple Entity Notes API.
- **`auvik-cli inventory read-single-interface-info`** - Use the Read Single Interface Info API to pull the collected information about a specific device interface Auvik has discovered. You’ll need the interface ID for the specific interface.

To find the interface IDs, run the Read Multiple Interfaces Info API.
- **`auvik-cli inventory read-single-network-details`** - Use the Read Single Networks’s Details API to pull extra collected information about a specific network Auvik has discovered not already included in the network Info API. You’ll need the network ID for the specific network.

To find the network IDs, run the Read Multiple Networks API.
- **`auvik-cli inventory read-single-network-info`** - Use the Read Single Network’s Info API to pull the collected information about a specific network Auvik has discovered. You’ll need the network ID for the specific network.

To find the network IDs, run the Read Multiple Networks’ Info API.

### meta

Manage meta

- **`auvik-cli meta`** - Pulls metadata information for a specific API endpoint and field. NOTE: this endpoint is documented by Auvik and implemented by the Auvik-PowerShell-Module community wrapper, but it is absent from Auvik's published OpenAPI documents and could not be probe-verified without credentials (unauthenticated requests to any path return 401).

### settings

Manage settings

- **`auvik-cli settings read-multiple-snmp-poller`** - Use the Read Multiple SNMP Poller Settings API to pull the list of SNMP Poller Settings configured in Auvik.
- **`auvik-cli settings read-multiple-snmp-poller-devices`** - Use Read SNMP Poller Setting's Devices API to pull the list of devices that apply to a specific SNMP Poller Setting Id.
- **`auvik-cli settings read-snmp-poller-single`** - Use the Read Single SNMP Poller Setting API to pull details of a specific SNMP Poller Setting configured in Auvik.

### stat

Manage stat

- **`auvik-cli stat read-component-statistics`** - Use the Read Component Statistics API to fetch detailed statistics of a client's (and client's children if a multi-client) components for a given time range.
- **`auvik-cli stat read-device-availability-statistics`** - Use the Read Device Availability Statistics API to fetch detailed availability statistics of a client’s (and client’s children if a multi-client) devices for a given time range.
- **`auvik-cli stat read-device-statistics`** - Use the Read Device Statistics API to fetch detailed statistics of a client’s (and client’s children if a multi-client) devices for a given time range.
- **`auvik-cli stat read-interface-statistics`** - Use the Read Interface Statistics API to fetch detailed statistics of a client's (and client's children if a multi-client) interfaces for a given time range.
- **`auvik-cli stat read-multiple-snmp-poller-setting-int-history`** - Use the Read SNMP Poller Setting's History API to fetch the list of historical vaules for a SNMP Poller Setting.
- **`auvik-cli stat read-multiple-snmp-poller-setting-string-history`** - Use the Read SNMP Poller Setting's History API to fetch the list of historical vaules for a SNMP Poller Setting.
- **`auvik-cli stat read-oid-statistics`** - Use the Read OID Statistics API to fetch the last recorded value of a monitored device OID.
- **`auvik-cli stat read-service-statistics`** - Use the Read Service Statistics API to fetch detailed statistics of a client’s (and client’s children if a multi-client) services for a given time range.

### tenants

The Auvik Tenant API allows you to see if you have access to multi-clients or clients associated to your Auvik user account. The output from the API shows if you have permissions to a multi-client or client, but doesn’t show the associated role permissions.

 There are three endpoints within the Tenant API.

- Read Multiple Tenants: Pulls access detail about multiple multi-clients and clients associated with your Auvik user account.

- Read Multiple Tenants Detail: Pulls details for multiple multi-clients and clients associated with your Auvik user account.

- Read single Tenant Detail: Pulls detail for a specific multiple multi-client or client associated with your Auvik user account.

- **`auvik-cli tenants read-multiple`** - Use the Read Multiple Tenants API to pull access detail about multiple multi-clients and clients associated with your Auvik user account.
- **`auvik-cli tenants read-multiple-detail`** - Use the Read Multiple Tenants API to pull details for multiple multi-clients and clients associated with your main Auvik account.
- **`auvik-cli tenants read-single-detail`** - Use the Read a Single Tenant API to pull detail about a specific multi-client and client associated with your main Auvik account. You’ll need the tenant ID for the specific multi-client or client you want detail for.

 You can find the tenant ID for the multi-client or client by Read Multiple Tenants Detail.


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`auvik-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`auvik-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`auvik-cli learnings list`** - Inspect taught rows
- **`auvik-cli learnings forget <query>`** - Undo a teach
- **`auvik-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`auvik-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`auvik-cli teach-pattern`** - Install a query/resource template up front
- **`auvik-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `AUVIK_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `auvik-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
auvik-cli alert read-multiple-info

# JSON for scripting and agents
auvik-cli alert read-multiple-info --json
# Filter to specific fields
auvik-cli alert read-multiple-info --json --select alertDefinitionId,description,detectedOn

# Dry run  -  show the request without sending
auvik-cli alert read-multiple-info --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
auvik-cli alert read-multiple-info --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
auvik-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `auvik-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/auvik-cli/config.toml`; `--home`, `AUVIK_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `AUVIK_USERNAME` | per_call | Yes |  |
| `AUVIK_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `auvik-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `auvik-cli doctor` to check credentials
- Verify the environment variable is set: `echo $AUVIK_USERNAME`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized with a completely empty response body**  -  Auvik sends no error body on auth failure. Check the REGION first - a valid key against the wrong region host returns 401 identically to a bad key. Set AUVIK_BASE_URL to https://auvikapi.<region>.my.auvik.com (us1, us2, us3, us4, eu1, ca1, au1), then run 'auvik-cli auth status'.
- **403 Forbidden, but the same credentials work elsewhere**  -  Auvik returns 403 for BOTH insufficient tenant role AND IP rate-limiting (about 2,500 requests per 5 minutes). Wait 5 minutes and retry; if it still fails, check the API user's role in that specific tenant.
- **429 Too Many Requests during a large sync**  -  Scope the run with 'auvik-cli sync --resources <one-resource>' and let the built-in backoff drain the window before syncing the rest.
- **A cross-client command prints nothing**  -  All eight cross-client commands read the local mirror and each needs its own resources synced: eol needs auvik-inventory-lifecycle (the v2 resource that carries the dates) and inventory-device-warranty; configuration audit needs inventory-configuration; alert noise needs alert; usage reconcile needs billing; asm shadow needs asm; device discovery-gaps needs auvik-inventory-discovery-status; changes needs inventory-configuration, inventory-entity-audit and inventory-entity-note. The command's own 'note' field names the exact resource to sync.
- **A command returns 200 but no rows**  -  Either the data does not exist in the Auvik UI either, or the result spans pages. 'sync' exhausts cursors for you; prefer it over paging endpoint commands by hand.
- **--select returned the whole payload and warned 'matched no fields'**  -  Select paths are relative to the emitted shape. For the local-store reports select through the row array (rows.client); for API endpoint commands the JSON:API envelope is already stripped, so select from attributes (attributes.deviceName).

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Auvik-PowerShell-Module**](https://github.com/DarrenWhite99/Auvik-PowerShell-Module)  -  PowerShell (13 stars)
- [**Auvik.Api**](https://github.com/panoramicdata/Auvik.Api)  -  C# (2 stars)
- [**netbox_plugin_auvik**](https://github.com/cstreich/netbox_plugin_auvik)  -  Python (2 stars)
- [**auvik-mcp**](https://github.com/wyre-technology/auvik-mcp)  -  TypeScript (1 stars)
- [**n8n-nodes-auvik**](https://github.com/msoukhomlinov/n8n-nodes-auvik)  -  TypeScript (1 stars)
- [**Celerium.Auvik**](https://github.com/Celerium/Celerium.Auvik)  -  PowerShell
- [**go-auvik**](https://github.com/stellaraf/go-auvik)  -  Go
- [**node-auvik**](https://github.com/wyre-technology/node-auvik)  -  TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
