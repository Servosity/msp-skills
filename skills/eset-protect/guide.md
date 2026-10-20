# ESET PROTECT CLI

**Every ESET Connect endpoint in one binary, plus per-customer health, exposure and QBR reports for MSP trees.**

Covers devices, detections, incidents, EDR rules, automation, vulnerabilities, patches, policies, quarantine, installers and IAM, with the password-grant token minted and refreshed for you (the jpn region has no quarantine, user-management or v2 detection list). Sync before you ask: 'fleet health', 'fleet gaps' and 'triage' answer from the local mirror (re-sync detections and incidents before each triage run, and run 'sync --full' now and then so devices and groups deleted in ESET leave the mirror), while 'exposure', 'patches backlog' and 'report customer' scan ESET live and use the mirror to name each customer.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `eset-protect-cli` and `eset-protect-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.ps1 | iex
   ```
3. Verify: `eset-protect-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=eset-protect). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`. On Windows, run `Unblock-File <binary>.exe` in PowerShell if SmartScreen blocks the downloaded file.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/eset-protect --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/eset-protect --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `eset-protect-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the eset-protect skill from https://github.com/Servosity/msp-skills/tree/main/skills/eset-protect. The skill defines how its required CLI (`eset-protect-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=eset-protect).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. When Claude Desktop prompts you, fill in the API user's email (`ESET_PROTECT_USERNAME`), its password (`ESET_PROTECT_PASSWORD`) and the data-center region (`ESET_PROTECT_REGION`, default `eu`).

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "eset-protect": {
      "command": "eset-protect-mcp",
      "env": {
        "ESET_PROTECT_REGION": "eu",
        "ESET_PROTECT_USERNAME": "api-user@your-msp.example",
        "ESET_PROTECT_PASSWORD": "the-api-user-password"
      }
    }
  }
}
```

</details>

## Authentication

ESET Connect has no API keys. Create a dedicated API user: a Superuser or Root adds it in ESET PROTECT Hub (Users), ESET Business Account or ESET MSP Administrator 2 (User management, Access Rights) and turns on the Integrations toggle. The new user accepts the invitation email and finishes account setup, and a Hub user must also sign in to ESET PROTECT Hub once. Then set ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD (`export VAR=value` on macOS/Linux, `$env:VAR = "value"` in PowerShell) and set ESET_PROTECT_REGION to the tenant's data center (eu, de, us, ca or jpn; default eu). The CLI exchanges them for a one-hour bearer token with the OAuth2 password grant, caches it for that region and user, and refreshes it automatically. ESET_PROTECT_TOKEN overrides with a token you minted yourself.

## Quick Start

```bash
# Report configuration, credential variables and reachability
eset-protect-cli doctor

# Mint a token from ESET_PROTECT_USERNAME / ESET_PROTECT_PASSWORD and prove it with one read
eset-protect-cli auth login

# Mirror the MSP tree and the data the per-customer commands join (vulnerabilities are always read live)
eset-protect-cli sync --resources devices,device-groups,detections,incidents,device-patches,executables

# See which customer needs attention first
eset-protect-cli fleet health --stale-days 7

# Work this morning's detections and open incidents in one ranked queue
eset-protect-cli triage --since 24h

```

## Unique Features

### Per-customer rollups
- **`fleet health`**  -  One row per customer with device count, attention-required and attention-recommended devices, problem count, muted, stale and unenrolled devices, worst customer first.

  _Reach for this to answer 'which customer needs attention today' across the whole MSP tree in one call._

  ```bash
  eset-protect-cli fleet health --stale-days 7 --agent
  ```
- **`report customer`**  -  One customer's QBR one-pager: current device health, detections by severity and response, incidents with triage and response times, top CVEs, pending and failed security patches, and vulnerability-scan coverage (ESET keeps only a few days of scan and patching history).

  _Use it to build a customer QBR or answer 'how is this customer doing' without five separate queries._

  ```bash
  eset-protect-cli report customer "Acme Dental" --since 90d --agent
  ```
- **`fleet gaps`**  -  Devices missing a component or product their customer's other devices have, running an older component version, or not finished enrolling.

  _Use it in the weekly agent-health sweep to find unprotected or out-of-date machines per customer._

  ```bash
  eset-protect-cli fleet gaps --only missing,outdated --agent
  ```

### Vulnerability and patch intelligence
- **`exposure`**  -  Which customers and devices have a given CVE or vulnerable application, with risk score, first detection and whether a patch is available.

  _Use it the moment a CVE makes the news and a customer asks 'are we affected?'._

  ```bash
  eset-protect-cli exposure --cve CVE-2024-3400 --agent
  ```
- **`patches backlog`**  -  Available-but-not-installed patches ranked by the devices and CVE risk they fix per customer, plus recent failed patch attempts by reason.

  _Use it after Patch Tuesday to decide what to push first and to see what failed._

  ```bash
  eset-protect-cli patches backlog --security-only --since 30d --agent
  ```

### Triage and response
- **`detections spread`**  -  How far each detected file hash has spread across devices and customers in a window, joined to whether that executable is blocked, its signer and its LiveGrid reputation when executables are synced.

  _Use it to decide false positive versus outbreak before resolving or writing an exclusion._

  ```bash
  eset-protect-cli detections spread --since 7d --min-customers 2
  ```
- **`respond`**  -  Scan, isolate or release a device by name (or scan a whole customer): prints the exact ESET task first and creates it only with --yes; with --wait it polls the task runs until they finish. Over MCP the root --yes is never forwarded, so the tool only previews.

  _Use it to act on a finding by device name without hand-building an automation task payload._

  ```bash
  eset-protect-cli respond scan FRONTDESK-01 --yes --wait 10m
  ```

### Everyday ESET workflows
- **`triage`**  -  Endpoint detections in a window (v1 has no resolved state) and open incidents in one ranked queue, worst first, with device, customer and a suggested next command.

  _Use it for the morning queue instead of paging detections and incidents separately._

  ```bash
  eset-protect-cli triage --since 24h --agent
  ```
- **`fleet stale`**  -  Devices that stopped checking in for N days, most stale first, with customer and group path.

  _Use it to find offboarding candidates or broken agents per customer._

  ```bash
  eset-protect-cli fleet stale --days 14 --agent
  ```
- **`detections latest`**  -  The newest endpoint detections straight from ESET for a time window and minimum severity, with device and customer names when the mirror is synced.

  _Use it when you need what fired in the last hours without a sync._

  ```bash
  eset-protect-cli detections latest --since 6h --min-severity high --agent
  ```
- **`devices profile`**  -  Everything about one device in one call: health, recent detections, CVEs, pending patches and recent scans, with partial-failure reporting.

  _Use it before touching a device or answering a ticket about it._

  ```bash
  eset-protect-cli devices profile FRONTDESK-01 --agent
  ```
- **`incidents context`**  -  One incident with its comments and every linked detection and device, falling back from v1 to v2 detection lookups.

  _Use it to work one incident end to end._

  ```bash
  eset-protect-cli incidents context 8d4f2a17-3b6c-4e1d-9a2f-5c7e1b0d4f6a --agent
  ```
- **`report vulnerabilities`**  -  Vulnerable devices, CVE counts by severity, top CVEs and patchable share per customer from a live vulnerability scan.

  _Use it for a monthly vulnerability review across customers._

  ```bash
  eset-protect-cli report vulnerabilities --top 10 --agent
  ```
- **`executables find`**  -  Turn an MD5, SHA-1 or SHA-256 hash or a name into the executable UUID that 'executables block' needs, with blocked state and signer.

  _Use it before blocking or excluding a file._

  ```bash
  eset-protect-cli executables find --hash 3395856ce81f2b7382dee72602f798b642f14140 --agent
  ```
- **`policies decode`**  -  Decode a policy's base64 and LZMA settings blobs into readable JSON, list settings that mention a word, or pull one value by path.

  _Use it to see what a policy actually sets before cloning or assigning it._

  ```bash
  eset-protect-cli policies decode 0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f --find firewall
  ```
- **`policies clone`**  -  Copy an existing policy's feature settings byte for byte into a new policy under a new name; preview unless --yes.

  _Use it to start a customer-specific policy from a known-good one._

  ```bash
  eset-protect-cli policies clone 0f8c2a4e-1b2d-4c3e-9f00-6a7b8c9d0e1f --name "Acme Dental - Workstations"
  ```

## Recipes

### Morning triage

```bash
eset-protect-cli triage --since 24h --agent
```

Endpoint detections from the last day (v1 has no resolved state) and open incidents in one ranked queue with device and customer names.

### Are we affected by a CVE

```bash
eset-protect-cli exposure --cve CVE-2024-3400 --agent --select customers.customer,customers.affected_devices,customers.patch_available,truncated,scanned_vulnerabilities,note
```

Per-customer exposure; truncated, scanned_vulnerabilities and note show whether the live scan was complete before you tell a customer they are clear.

### Outbreak or false positive

```bash
eset-protect-cli detections spread --since 7d --min-customers 2
```

Hashes seen at two or more customers this week (devices the mirror cannot place count as possible customers, and thresholds_proven is false when only they reach the threshold); blocked state and signer appear when executables are synced.

### Customer QBR

```bash
eset-protect-cli report customer "Acme Dental" --since 90d --agent
```

One customer's health, detections, incidents, CVEs and patches; failed live sections are null and listed in fetch_failures.

### Isolate a machine

```bash
eset-protect-cli respond isolate FRONTDESK-01
```

Prints the exact StartNetworkIsolation task for the named device; nothing is sent until you add --yes, and 'respond release' ends the isolation.

## Usage

Run `eset-protect-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml` (the cached ESET Connect token, bound to region and API user) and the synced mirror `data-<hash>.db` (the hash is scoped to region and API user; `eset-protect-cli doctor` prints the exact path) |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `ESET_PROTECT_CONFIG_DIR`, `ESET_PROTECT_DATA_DIR`, `ESET_PROTECT_STATE_DIR`, or `ESET_PROTECT_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `ESET_PROTECT_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export ESET_PROTECT_HOME=/srv/eset-protect
eset-protect-cli doctor
```

On Windows (PowerShell):

```powershell
$env:ESET_PROTECT_HOME = "$env:LOCALAPPDATA\eset-protect"
eset-protect-cli doctor
```

Under `ESET_PROTECT_HOME=/srv/eset-protect`, the four dirs resolve to `/srv/eset-protect/config`, `/srv/eset-protect/data`, `/srv/eset-protect/state`, and `/srv/eset-protect/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "eset-protect": {
      "command": "eset-protect-mcp",
      "env": {
        "ESET_PROTECT_HOME": "/srv/eset-protect"
      }
    }
  }
}
```

On Windows, use a Windows path with escaped backslashes, for example `"ESET_PROTECT_HOME": "C:\\Users\\<you>\\AppData\\Local\\eset-protect"` (a folder under your own profile, so other local users cannot read the mirror or the cached tokens).

Precedence matters in fleets: an ambient per-kind variable such as `ESET_PROTECT_DATA_DIR` overrides an explicit `--home` for that kind. Use `ESET_PROTECT_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `ESET_PROTECT_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `eset-protect-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### application-patching-processes

Manage application patching processes

- **`eset-protect-cli application-patching-processes`** - List the patching details of all application patching attempts performed recently (within the last few days).

The list might be empty if no application patching has been performed recently.
Stale data (that is, not recent) is removed from the list over the course of several days.

### detection-groups

Manage detection groups

- **`eset-protect-cli detection-groups get`** - Get details about a detection group.
- **`eset-protect-cli detection-groups list`** - Return a list of all the [detection group]s matching criteria.
- **`eset-protect-cli detection-groups resolve`** - Mark all the [detections] in a group as resolved.

### detection-groups-search

Manage detection groups search

- **`eset-protect-cli detection-groups-search`** - Search a collection for [detection groups]s matching criteria defined by the filter (if given).

> **Info:**
> [Search method](https://google.aip.dev/136#common_custom_methods) is used instead of [standard List](https://google.aip.dev/132) to provide extra filtering semantics.

### detections

Manage detections

- **`eset-protect-cli detections get`** - Endpoint detection from ESET PROTECT and ESET Inspect (v1).
- **`eset-protect-cli detections list`** - Endpoint detections from ESET PROTECT and ESET Inspect (v1).

Return a list of all the [detection]s matching criteria.

### detections-v2

Manage detections v2

- **`eset-protect-cli detections-v2 batch-get`** - Batch-get detections from the v2 API (ESET Inspect and ESET Cloud Office Security).

Batch-get detections.

The operation is atomic: it fails for all resources or succeeds for all resources (no partial success).

If the operation covers multiple locations and at least one location is down, the operation fails.

> **Info:**
> Modeled after [Batch Methods: Get](https://google.aip.dev/231)
- **`eset-protect-cli detections-v2 get`** - Detection by UUID from the v2 API (ESET Inspect and ESET Cloud Office Security).

Get detection.
- **`eset-protect-cli detections-v2 list`** - ESET Cloud Office Security detections only (v2). The API user must have signed in to the ESET Cloud Office Security console once. Not available in the jpn region.

Return a list of all the [detection]s matching criteria.
- **`eset-protect-cli detections-v2 resolve`** - Resolve an ESET Inspect detection (v2).

Mark a [detection] as resolved.

### device-groups

Manage device groups

- **`eset-protect-cli device-groups`** - Hierarchy can be reconstructed from the returned `device_groups`.

### device-os-vulnerabilities

Manage device os vulnerabilities

- **`eset-protect-cli device-os-vulnerabilities`** - List vulnerabilities of the operating system of the given device.

> **Note:**
> Besides vulnerabilities of the Linux kernel, there are also some "core packages" whose vulnerabilities are treated as operating system vulnerabilities.
> For example: glibc, systemd, etc.
>
> Such vulnerabilities are reported twice, once as an operating system vulnerability and once as a package vulnerability.

### device-patches

Manage device patches

- **`eset-protect-cli device-patches`** - If the same application, package or operating system is unpatched on multiple devices, the collection would contain entities for each device and patch individually.

The list may be empty if no patches for any device are found.

### device-tasks

Manage device tasks

- **`eset-protect-cli device-tasks create`** - Create a [device task].

There is one [device task template] for every unique `display_name`, description, and `action` combination.
This template inherits `display_name`, `description` and `action` from the `task` in the request.

The created task requires to have at least one trigger set. Otherwise, the call fails.
- **`eset-protect-cli device-tasks delete`** - Delete the specified task.

Deletion of the template leads to deletion of all templated tasks.
- **`eset-protect-cli device-tasks get`** - Get the task entity.

For a complete history, call `ListTaskRuns()`.
- **`eset-protect-cli device-tasks list`** - List all device tasks matching criteria.
- **`eset-protect-cli device-tasks update-targets`** - Update task targets.
- **`eset-protect-cli device-tasks update-triggers`** - Update the list of task triggers.

### device-vulnerabilities

Manage device vulnerabilities

- **`eset-protect-cli device-vulnerabilities`** - List device vulnerabilities where the vulnerabilities fit the criteria from the request.

Each vulnerability is treated individually; that is, if the device has multiple vulnerabilities, it will be listed multiple times.

### devices

Manage devices

- **`eset-protect-cli devices get`** - Get details of a [device].
- **`eset-protect-cli devices list`** - List all the managed [device]s.
- **`eset-protect-cli devices move`** - Move [Device] under new [Group].

It is possible to move Groups only in the scope of the same tenant.
The assignment of policies is updated according to the new hierarchy.
- **`eset-protect-cli devices rename`** - Update the device's display name.

The device display name is objective; it changes for everyone requesting such a device.

### devices-batch-get

Manage devices batch get

- **`eset-protect-cli devices-batch-get`** - Retrieves a specific set of devices at a consistent time point.

> **Info:**
> Modeled after [Batch methods: Get](https://google.aip.dev/231)

### devices-batch-import

Manage devices batch import

- **`eset-protect-cli devices-batch-import`** - Import a batch of devices into device management. Imported devices cannot be managed until properly enrolled. 

The size of the batch is limited to 1000 devices. If this limit is exceeded, OUT_OF_RANGE (gRPC) | 400 Bad Request (HTTP) is returned.

> **Warning:**
> Only devices imported with `is mobile` flag == `true` are suitable for Mobile Device Management scenarios.

The method can fail atomically with:
- PERMISSION_DENIED (gRPC) | 403 Forbidden (HTTP) if the caller does not have rights for the specified group.
- INVALID_ARGUMENT (gRPC) | 400 Bad Request (HTTP) if provided device `display names` are not mutually unique within the request.
- DEADLINE_EXCEEDED (gRPC) | 504 Gateway Timeout (HTTP) if the batch cannot be processed within the time limit of one call.

The method can fail partially. In such a case, OK (gRPC) | 200 OK (HTTP) is returned, and the response contains a list of errors for every device where an error occurred during enrollment. Possible errors are:
- INVALID_ARGUMENT (gRPC) if the attributes of the enrolled device are not valid. Also, this error is raised for every imported device where the `parent group` attribute is not empty and does not match the `parent group` from the request.
Failed devices are identified by the display names from the request (they do not have other unique identifiers yet).

Errors are accompanied by an error message in plain English.

> **Note:**
> Method parameters follow the semantics of [Batch methods: Create](https://google.aip.dev/233)

### edr-rule-exclusions

Manage edr rule exclusions

- **`eset-protect-cli edr-rule-exclusions create`** - Create an [EDR rule exclusion].
- **`eset-protect-cli edr-rule-exclusions delete`** - Delete an [EDR rule exclusion].
- **`eset-protect-cli edr-rule-exclusions get`** - Get details of an [EDR rule exclusion].
- **`eset-protect-cli edr-rule-exclusions list`** - List [EDR rule exclusion]s matching criteria.
- **`eset-protect-cli edr-rule-exclusions update-definition`** - Update the definition of an [EDR rule exclusion].

### edr-rules

Manage edr rules

- **`eset-protect-cli edr-rules create`** - Create an [EDR rule].

Only the rule with valid `xml_definition` can be accepted.
- **`eset-protect-cli edr-rules delete`** - Delete the given [EDR rule].
- **`eset-protect-cli edr-rules disable`** - Disable the given [EDR rule].
- **`eset-protect-cli edr-rules enable`** - Enable the given [EDR rule].
- **`eset-protect-cli edr-rules get`** - Get [EDR rule] details.
- **`eset-protect-cli edr-rules list`** - List [EDR rule]s matching the criteria.
- **`eset-protect-cli edr-rules update-definition`** - Update the [EDR rule] definition.

If the rule_definition is invalid, an error 400 BAD REQUEST is returned.

### executables

Manage executables

- **`eset-protect-cli executables block`** - Blocked executables will not be executed.

Idempotent
- **`eset-protect-cli executables get`** - Get details of a specific executable.
- **`eset-protect-cli executables list`** - List all the executables.
- **`eset-protect-cli executables unblock`** - Unblock a file to be executable again.

Idempotent

### gpo-sccm-files-generate

Manage gpo sccm files generate

- **`eset-protect-cli gpo-sccm-files-generate`** - Generates a GPO/SCCM-compatible configuration file for deploying the security product via centralized IT management systems (such as Group Policy or System Center Configuration Manager). 


The configuration is generated ad-hoc and not stored in the system.

### groups

Manage groups

- **`eset-protect-cli groups create`** - Create a [Group] in the hierarchy and assign a UUID to it.
- **`eset-protect-cli groups delete`** - Delete the group referenced by the uuid and any groups under it. If there are objects under the deleted groups, all of them will be deleted as well.
It is not possible to delete a group that has a linked entity (CUSTOMER, MSP, ...).
- **`eset-protect-cli groups move`** - Move a [Group] under another [Group].

It is possible to move groups only in the scope of the same tenant.
The assignment of policies is updated according to the new hierarchy.
- **`eset-protect-cli groups rename`** - Update the group's display name.

### incidents

Manage incidents

- **`eset-protect-cli incidents close`** - Close an incident.
- **`eset-protect-cli incidents get`** - Get details of a specific incident.
- **`eset-protect-cli incidents list`** - Returns a list of incidents matching the `filter`.
The list is ordered according to `order_by`.
- **`eset-protect-cli incidents reopen`** - Reopening an incident causes the change of the status to 'Open'. This can only be done if the incident is not in the 'Open' state.

The assignee is not affected.

If the incident has a comment that was added when the incident was closed, it is changed to a normal comment. This means that 'is_closed_comment' is set to false.

### installers

Manage installers

- **`eset-protect-cli installers create`** - Create [installer].
- **`eset-protect-cli installers delete`** - Delete [installer].
- **`eset-protect-cli installers get`** - Get details of the requested [installer].
- **`eset-protect-cli installers list`** - List all installers available to the caller.

### ip-sets

Manage ip sets

- **`eset-protect-cli ip-sets get`** - Get details about an IP set.
- **`eset-protect-cli ip-sets list`** - List all IP sets.
- **`eset-protect-cli ip-sets update`** - Update the selected IP set; attributes that can be updated:
- display_name
- description
- ip_addresses

If IP addresses are updated, there is no check for uniqueness, which means that one IP address can appear multiple times inside the IP set.

Read-only IP sets cannot be updated.
For built-in IP sets, only the ip_addresses attribute can be updated.

### mobile-devices-batch-activate-product

Manage mobile devices batch activate product

- **`eset-protect-cli mobile-devices-batch-activate-product`** - Create a product activation task with mobile devices as its targets.

The product is picked automatically based on the parent group of a particular device.

The method can fail atomically with:
- PERMISSION_DENIED (gRPC) | 403 Forbidden (HTTP) if the caller cannot does not have rights to relevant resources (devices, device tasks and triggers).
- FAILED_PRECONDITION (gRPC) | 400 Bad Request (HTTP) if 
- the devices on input do not share the same parent [device group] or
- if there is no such subscription with available product on the parent [device group].
- NOT_FOUND (gRPC) | 404 Not Found (HTTP) if some of the devices on the input does not exist.

### mobile-devices-batch-get-enrollment-links

Manage mobile devices batch get enrollment links

- **`eset-protect-cli mobile-devices-batch-get-enrollment-links`** - Get enrollment links for a batch of **mobile** devices in device management. The mobile devices must have `is mobile` flag set to `true`.

>**Warning:**
> A link only works on Android and iOS operating systems.

There is no limit on number of enrollment links issued for one device. All of them can be valid at the same time (within limits of their own expiration) until the device gets enrolled.

The size of the batch is limited to 1000 devices. If this limit is exceeded, INVALID_ARGUMENT (gRPC) | 400 Bad Request (HTTP) is returned.

The operation fails atomically. If any error occurs, it is returned for the whole batch. The devices are processed in an undetermined order.
- If some device is already enrolled, the link is not generated, and the error FAILED_PRECONDITION (gRPC) | 400 Bad Request (HTTP) is returned.
- For a device not marked as `mobile` in the device management (effectively not an iOS or Android device), the link is not generated, and error FAILED_PRECONDITION (gRPC) | 400 Bad Request (HTTP) is returned.
- For non-existing devices, the error NOT_FOUND (gRPC) | 404 Not Found (HTTP) is returned.

### patching-process-details

Manage patching process details

- **`eset-protect-cli patching-process-details`** - History is pruned repeatedly to prevent database flooding. Entries from at least the previous 30 days should be available at any time.

### permissions

Manage permissions

- **`eset-protect-cli permissions`** - List all possible permissions.

### policies

Manage policies

- **`eset-protect-cli policies create`** - Create a policy.

All the included feature policies must be valid. Otherwise, INVALID_ARGUMENT (gRPC) or 400 BAD Request (REST) is returned.
- **`eset-protect-cli policies delete`** - Delete the given policy.
- **`eset-protect-cli policies get`** - Get policy details.
- **`eset-protect-cli policies list`** - List all the policies accessible to the caller (based on the token).

### policy-assignments

Manage policy assignments

- **`eset-protect-cli policy-assignments assign-policy`** - Create a policy assignment.

Assignments to the same targets are ordered and prioritized.

Every new assignment to the target is put at the end of the stack. The assignment ranking will be equal to the number of target's assignments.

Ranking 1 is the highest, and policies are merged from rank 1 towards lower ranks.

> **Info:** 
> A similar concept is described in [Group assignment ranking](https://learn.microsoft.com/en-us/microsoftteams/assign-policies-users-and-groups#group-assignment-ranking)
- **`eset-protect-cli policy-assignments get`** - Get details of the given policy assignment.
- **`eset-protect-cli policy-assignments list`** - List all the policy assignments available to the caller.
- **`eset-protect-cli policy-assignments unassign-policy`** - Delete the policy assignment. 

The remaining assignments to the target are reordered.
- **`eset-protect-cli policy-assignments update-ranking`** - Move the policy assignment within ranks.

### quarantined-objects

Manage quarantined objects

- **`eset-protect-cli quarantined-objects get`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Get a quarantined object.
- **`eset-protect-cli quarantined-objects get-count`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Get the count of quarantined objects matching the criteria.
- **`eset-protect-cli quarantined-objects list`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

List all quarantined objects matching the criteria in the filter.

### quarantined-objects-batch-delete

Manage quarantined objects batch delete

- **`eset-protect-cli quarantined-objects-batch-delete`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Delete a batch of quarantined objects. The objects will be deleted permanently.

### quarantined-objects-batch-download

Manage quarantined objects batch download

- **`eset-protect-cli quarantined-objects-batch-download`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Download a batch of quarantined objects.

The response is a ZIP archive of quarantined objects.

> **Info:**
> Modeled after [gRPC-JSON transcoder: Sending arbitrary content](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/grpc_json_transcoder_filter#sending-arbitrary-content)
> and
> [GitHub: Google APIs](https://github.com/googleapis/googleapis/blob/master/google/api/httpbody.proto)

### quarantined-objects-batch-restore

Manage quarantined objects batch restore

- **`eset-protect-cli quarantined-objects-batch-restore`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Restore a batch of quarantined objects to their original location.

Objects might be scanned anytime later; if the reasons for being quarantined persist, they will be quarantined again.

### quarantined-objects-download

Manage quarantined objects download

- **`eset-protect-cli quarantined-objects-download`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Download quarantined objects matching the criteria in the filter.

The response is a ZIP archive of quarantined objects.

> **Info:**
> Modeled after [gRPC-JSON transcoder: Sending arbitrary content](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/grpc_json_transcoder_filter#sending-arbitrary-content)
> and
> [GitHub: Google APIs](https://github.com/googleapis/googleapis/blob/master/google/api/httpbody.proto)

### quarantined-objects-purge

Manage quarantined objects purge

- **`eset-protect-cli quarantined-objects-purge`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Delete quarantined objects matching criteria in the filter. The objects will be deleted permanently.

> **Info:**
> Name `Purge` is chosen to comply with [Criteria-based delete](https://google.aip.dev/165)

### quarantined-objects-restore

Manage quarantined objects restore

- **`eset-protect-cli quarantined-objects-restore`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Restore quarantined objects matching the criteria in the filter to their original location.

Objects might be scanned anytime later; if the reasons for being quarantined persist, they will be quarantined again.

### role-assignments

Manage role assignments

- **`eset-protect-cli role-assignments`** - List [role]s of [subject]s.

> **Info:**
> Not only Users are [subject]s, but anything that can have roles attached. For example, Devices.

> **Info:**
> Modeled after [List role assignments](https://learn.microsoft.com/en-us/azure/role-based-access-control/role-assignments-list-portal).

Might be filtered by the specified [subject] or [scope]s.

If the filters are left empty, all the roles available to the caller are returned.

Example:
Request*
```yaml
---
scopes:
- msp_uuid: af631cc9-3e9f-4fd7-8f29-ed8121b4cf8a
```

Response*
```yaml
---
assignments:
- subject_reference: 286f5456-a0ac-4e8a-8508-5c2224b47ae6
roles:
- role_name: admin
scopes:
- msp_uuid: af631cc9-3e9f-4fd7-8f29-ed8121b4cf8a
- customer_uuid: 30bd93aa-c0ef-4fcf-a73f-ce80610bd161
- role_name: auditor
scopes:
- msp_uuid: 30bd93aa-c0ef-4fcf-a73f-ce80610bd161
- subject_reference: ae9e45d2-9ee6-43e7-9b68-a650d62eeff1
roles:
- role_name: admin
scopes:
- msp_uuid: af631cc9-3e9f-4fd7-8f29-ed8121b4cf8a
- customer_uuid: ff572f63-8965-47da-9d9d-cb994dc9da10
- role_name: auditor
scopes:
- msp_uuid: af631cc9-3e9f-4fd7-8f29-ed8121b4cf8a
- customer_uuid: 30bd93aa-c0ef-4fcf-a73f-ce80610bd161
```

### role-assignments-assign-role

Manage role assignments assign role

- **`eset-protect-cli role-assignments-assign-role`** - Assign a role to a subject. Eventually, the subject ends with its existing roles and those assigned by this operation.

For example:
"User X" is an "admin" of the Subscription B. 
`AssignRole( "User X", { "admin", ["Subscription A", "Subscription C"]) `
leaves the user being an admin of the Subscriptions A, B, and C.

### role-assignments-revoke-role

Manage role assignments revoke role

- **`eset-protect-cli role-assignments-revoke-role`** - Remove the assigned role from the subject. Eventually, the subject ends without roles or scopes unassigned by this operation.

For example:
"User X" is an "admin" of the Subscriptions A, B, C. 
`RevokeRole( "User X", { "admin", "Subscription B"}) `
leaves the user being an admin of the Subscriptions A and C.

### roles

Manage roles

- **`eset-protect-cli roles create`** - Creates a new custom [Role].
The role name must be unique within the scope in which the role is created.

> **Info:**
> For EPC, name must be unique within the EPC Instance.
- **`eset-protect-cli roles delete`** - Delete specified role.

### scans

Manage scans

- **`eset-protect-cli scans`** - List the scan details of all vulnerability scan attempts performed recently (within the last few days).

The list might be empty if no vulnerability scans have been performed recently.
Stale data (that is, not recent) is removed from the list over the course of several days.

### users

Manage users

- **`eset-protect-cli users get`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Retrieves the details of a specific user based on their unique identifier.
- **`eset-protect-cli users list`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Retrieves a list of users, optionally filtered by certain criteria defined in the user filter.

### users-batch-get-users

Manage users batch get users

- **`eset-protect-cli users-batch-get-users`** - Requires ESET Cloud Office Security access (the API user must sign in to that console once); not available in the jpn region.

Retrieves a specific set of [user]s at some point in time.

The size of the batch is limited to 1000 users. If this limit is exceeded, OUT_OF_RANGE (gRPC) | 400 Bad Request (HTTP) is returned.

### vulnerable-devices

Manage vulnerable devices

- **`eset-protect-cli vulnerable-devices`** - List vulnerable devices matching the criteria from the request.

Even if a device has multiple vulnerabilities, it will be listed only once.

### web-address-rules

Manage web address rules

- **`eset-protect-cli web-address-rules <policyUuid>`** - List all address rules containing given domain


### detections


- **`eset-protect-cli detections spread`** - How far each detected file hash has spread across devices and customers in a window, joined to whether that executable is blocked, its signer and its LiveGrid reputation when executables are synced.

- **`eset-protect-cli detections latest`** - The newest endpoint detections straight from ESET for a time window and minimum severity, with device and customer names when the mirror is synced.


### devices


- **`eset-protect-cli devices profile`** - Everything about one device in one call: health, recent detections, CVEs, pending patches and recent scans, with partial-failure reporting.


### executables


- **`eset-protect-cli executables find`** - Turn an MD5, SHA-1 or SHA-256 hash or a name into the executable UUID that 'executables block' needs, with blocked state and signer.


### exposure


- **`eset-protect-cli exposure`** - Which customers and devices have a given CVE or vulnerable application, with risk score, first detection and whether a patch is available.


### fleet


- **`eset-protect-cli fleet health`** - One row per customer with device count, attention-required and attention-recommended devices, problem count, muted, stale and unenrolled devices, worst customer first.

- **`eset-protect-cli fleet gaps`** - Devices missing a component or product their customer's other devices have, running an older component version, or not finished enrolling.

- **`eset-protect-cli fleet stale`** - Devices that stopped checking in for N days, most stale first, with customer and group path.


### incidents


- **`eset-protect-cli incidents context`** - One incident with its comments and every linked detection and device, falling back from v1 to v2 detection lookups.


### patches


- **`eset-protect-cli patches backlog`** - Available-but-not-installed patches ranked by the devices and CVE risk they fix per customer, plus recent failed patch attempts by reason.


### policies


- **`eset-protect-cli policies decode`** - Decode a policy's base64 and LZMA settings blobs into readable JSON, list settings that mention a word, or pull one value by path.

- **`eset-protect-cli policies clone`** - Copy an existing policy's feature settings byte for byte into a new policy under a new name; preview unless --yes.


### report


- **`eset-protect-cli report customer`** - One customer's QBR one-pager: current device health, detections by severity and response, incidents with triage and response times, top CVEs, pending and failed security patches, and vulnerability-scan coverage (ESET keeps only a few days of scan and patching history).

- **`eset-protect-cli report vulnerabilities`** - Vulnerable devices, CVE counts by severity, top CVEs and patchable share per customer from a live vulnerability scan.


### respond


- **`eset-protect-cli respond`** - Scan, isolate or release a device by name (or scan a whole customer): prints the exact ESET task first and creates it only with --yes; with --wait it polls the task runs until they finish. Over MCP the root --yes is never forwarded, so the tool only previews.


### triage


- **`eset-protect-cli triage`** - Endpoint detections in a window (v1 has no resolved state) and open incidents in one ranked queue, worst first, with device, customer and a suggested next command.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`eset-protect-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`eset-protect-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`eset-protect-cli learnings list`** - Inspect taught rows
- **`eset-protect-cli learnings forget <query>`** - Undo a teach
- **`eset-protect-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`eset-protect-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`eset-protect-cli teach-pattern`** - Install a query/resource template up front
- **`eset-protect-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `ESET_PROTECT_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `eset-protect-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
eset-protect-cli detection-groups list

# JSON for scripting and agents
eset-protect-cli detection-groups list --json
# Filter to specific fields
eset-protect-cli detection-groups list --json --select category,circumstances,cloudOfficeTenantUuid

# Dry run  -  show the request without sending
eset-protect-cli detection-groups list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
eset-protect-cli detection-groups list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending (the API request only: `--deliver webhook:<url>` still posts the output, `feedback --send` still sends, and `import --dry-run` may still request an access token)
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Runtime Endpoint

ESET Connect runs one host per service, `https://{region}.<service>.eset.systems` (device-management, incident-management, automation, vulnerability-management and so on), and the token endpoint `https://{region}.business-account.iam.eset.systems/oauth/token`.

- `ESET_PROTECT_REGION` (eu, de, us, ca or jpn; default `eu`) picks the data center for every service host and the token endpoint. It selects a region, not a tenant: the API user decides which tenant you see.
- The jpn region does not offer quarantine-management, user-management or `GET /v2/detections`.

## Health Check

```bash
eset-protect-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `eset-protect-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/eset-protect-cli/config.toml` (`%USERPROFILE%\.config\eset-protect-cli\config.toml` on Windows); `--home`, `ESET_PROTECT_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ESET_PROTECT_REGION` | endpoint | No (default `eu`) | Data center: eu, de, us, ca or jpn. |
| `ESET_PROTECT_USERNAME` | auth_flow_input | No | Email of the dedicated ESET Connect API user. |
| `ESET_PROTECT_PASSWORD` | auth_flow_input | No | Password of the dedicated ESET Connect API user; the CLI exchanges it for a bearer token and never writes it to disk. |
| `ESET_PROTECT_TOKEN` | per_call | No | Optional pre-minted ESET Connect bearer access token (expires after one hour); leave unset to sign in with the username and password. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `eset-protect-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `eset-protect-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ESET_PROTECT_USERNAME` (PowerShell: `$env:ESET_PROTECT_USERNAME`)
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **Token request rejected: HTTP 400 unauthorized_client: invalid_username_or_password**  -  Use the API user's email and password (not the Superuser/Root account, which cannot hold API rights), and set ESET_PROTECT_REGION to the tenant's data center: eu, de, us, ca or jpn.
- **Every command fails with HTTP 400 and an empty body (exit 5)**  -  No credentials reached the request: set ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD (or ESET_PROTECT_TOKEN) in the environment (`export` on macOS/Linux, `$env:` in PowerShell), then run 'eset-protect-cli auth login'.
- **HTTP 403 on some resources**  -  The API user lacks permission for that domain: grant it in ESET PROTECT Web Console > Permission Sets, or ESET Cloud Office Security access for quarantine, users and detections-v2 list.
- **quarantined-objects, users or detections-v2 list fail in the jpn region**  -  ESET Connect does not offer quarantine-management, user-management or GET /v2/detections in jpn; use detections list (v1) instead.
- **A large query times out**  -  ESET answers slow queries with 202 and the CLI keeps polling until the request timeout (60s by default); raise it with --timeout 3m. ESET itself answers 504 to a query that is not ready within 120 seconds, so if that happens narrow the query (--customer, a shorter --since, fewer --max-scan-pages).
- **fleet, triage or detections spread return nothing, or exposure/report rows are not split by customer**  -  fleet, triage and detections spread read only the local mirror; exposure, patches backlog and the report commands read vulnerabilities live but need the synced device tree to name customers. Run 'eset-protect-cli sync --resources devices,device-groups,detections,incidents,device-patches,executables'. The JSON from fleet, triage, detections spread, exposure and patches backlog carries a synced_hint naming the missing sync, and an unattributed_* count with the UUIDs of rows whose owner the mirror cannot prove: the device, its group or an ancestor group between it and its customer is not in the mirror (deleted, or added after the last sync), the groups form a loop, or a synced group's parent is outside the API user's permissions (labelled (parent group not visible); a sync does not attribute those). Devices in the MSP's own group, or under the one top group your API user cannot see, are filed as (unassigned), not unattributed; an empty result with no synced_hint and no unattributed count is a real zero for the data as of the last sync (a stderr hint says when the mirror is older than --max-age), so re-sync detections and incidents before trusting an empty triage.
- **HTTP 429, or 202 and 50x answers during a large burst**  -  ESET Connect allows 10 calls per second per credential, account and IP, and its Fair Use Policy slows a large burst (about 1000+ requests) to 2 requests per second and then rejects the rest with 50x; pass --rate-limit 5 when other tools share the API user, and keep sync --concurrency low.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**ESET-MCP**](https://github.com/maciekaz/ESET-MCP)  -  Python (4 stars)
- [**eset-protect-mcp (Fenrindale)**](https://github.com/Fenrindale/eset-protect-mcp)  -  TypeScript (2 stars)
- [**eset-protect-mcp (WYRE-AI)**](https://github.com/WYRE-AI/eset-protect-mcp)  -  TypeScript
- [**esetconnect_psm**](https://github.com/m11tch/esetconnect_psm)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
