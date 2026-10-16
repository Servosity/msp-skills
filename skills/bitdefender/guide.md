# Bitdefender GravityZone CLI

**140 GravityZone Public API methods as commands, plus fleet health, billing and incident views across child companies.**

140 GravityZone Partners and Cloud Public API methods are typed commands with --json and --dry-run; over MCP they are reached through bitdefender_search and bitdefender_execute. On top, fleet health, fleet incident-queue and usage billing walk the whole partner tree so an MSP sees every customer at once, and fleet changes diffs saved fleet health snapshots. Not yet verified against a live tenant.

Learn more at [Bitdefender GravityZone](https://www.bitdefender.com/business/).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `bitdefender-cli` and `bitdefender-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.ps1 | iex
   ```
3. Verify: `bitdefender-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=bitdefender). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/bitdefender --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/bitdefender --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `bitdefender-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the bitdefender skill from https://github.com/Servosity/msp-skills/tree/main/skills/bitdefender. The skill defines how its required CLI (`bitdefender-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=bitdefender).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `BITDEFENDER_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "bitdefender": {
      "command": "bitdefender-mcp",
      "env": {
        "BITDEFENDER_API_KEY": "<your-key>",
        "BITDEFENDER_BASE_URL": "https://cloud.gravityzone.bitdefender.com/api"
      }
    }
  }
}
```

</details>

## Authentication

GravityZone authenticates with HTTP Basic: the API key is the username and the password is empty. Create the key in Control Center under your user menu > My Account > API keys and enable the API areas you will use: keys are scoped per area, with no read/write split. The fleet commands need a partner-level key with the Companies and Network APIs (companies tree, fleet health, fleet locate, fleet rmm-gap), plus Licensing for usage billing, Incidents for fleet incident-queue, and Quarantine and Incidents for fleet threats. A customer-level key works but every fleet view then covers only that one company. Export the key as BITDEFENDER_API_KEY (`export BITDEFENDER_API_KEY=<key>` in bash or zsh; `$env:BITDEFENDER_API_KEY = "<key>"` in PowerShell), or pipe it to `bitdefender-cli auth set-token` to save it. The API host differs by region and on-premises install, so also set BITDEFENDER_BASE_URL to the Access URL shown under My Account > Control Center API (for example https://cloud.gravityzone.bitdefender.com/api).

## Quick Start

```bash
# Check config, the API key and connectivity
bitdefender-cli doctor

# Confirm the key works and see which API areas it may call
bitdefender-cli api-key --json

# List every child company and sub-partner with ids
bitdefender-cli companies tree --agent

# Rank customers by unprotected, outdated and unscanned endpoints
bitdefender-cli fleet health --agent

# Open incidents across all customers, worst first
bitdefender-cli fleet incident-queue --sla 24h --agent

```

## Unique Features

### Across every child company
- **`fleet health`**  -  One ranked table of every child company's unmanaged, outdated, unscanned, policy-not-applied and isolated endpoints, saved as a dated snapshot.

  _Reach for this for a Monday sweep or any 'which customers are unprotected' question instead of paging inventory company by company._

  ```bash
  bitdefender-cli fleet health --agent
  ```
- **`companies tree`**  -  Your own company plus every child company and sub-partner under it as one flat list with ids, type, parent and depth.

  _Call it first for any cross-tenant question to get the company ids every other command needs._

  ```bash
  bitdefender-cli companies tree --agent
  ```
- **`fleet locate`**  -  Find a machine by hostname, IP, MAC or label across every child company and get its endpoint id and company.

  _Use it before any isolate, scan or move to get the endpoint id; pass --data-source live when acting, since the snapshot can be stale._

  ```bash
  bitdefender-cli fleet locate LAPTOP-7 --agent
  ```

### Security operations across tenants
- **`fleet incident-queue`**  -  Open EDR/XDR incidents across all child companies, ranked by severity and age with unassigned and SLA-breach flags.

  _Use it as the daily triage queue; it emits incident ids the generated incidents set-status and assign commands take._

  ```bash
  bitdefender-cli fleet incident-queue --sla 24h --agent
  ```
- **`fleet threats`**  -  The same quarantined file (SHA256) seen at two or more companies in a window, and which of those companies already blocklist it.

  _Use it for the weekly cross-client threat review; unblocked_hashes lists the SHA256 values to add with blocklist add --type hash._

  ```bash
  bitdefender-cli fleet threats --window 7d --agent
  ```

### Billing and reconciliation
- **`usage billing`**  -  Per-company billable usage for a month with own share after the child rollup, month-over-month seat swings, and flags wherever a needed fetch failed.

  _Use it for month-end PSA billing reconciliation instead of pasting monthly usage into a spreadsheet._

  ```bash
  bitdefender-cli usage billing --month 09/2026 --agent
  ```
- **`fleet rmm-gap`**  -  Joins an RMM device export to GravityZone inventory to list devices with no agent, discovered-but-unmanaged machines, and agents the RMM does not know.

  _Use it for the weekly 'is every RMM device protected' check. --devices takes your RMM's device export: a CSV with a hostname column (Hostname, Device Name, Computer Name and similar headers all match; a MAC column is optional) or a JSON array of device objects, such as another connector's --json output._

  ```bash
  bitdefender-cli fleet rmm-gap --devices ninja-devices.csv --agent
  ```

### Local history
- **`fleet changes`**  -  What changed between saved fleet health snapshots (needs at least two): new and removed endpoints, newly unmanaged, outdated or isolated machines, policy flips.

  _Use it after fleet health to see drift instead of re-reading the whole fleet._

  ```bash
  bitdefender-cli fleet changes --since 7d --agent
  ```

## Recipes

### Monday fleet sweep

```bash
bitdefender-cli fleet health --agent --select companies.name,companies.status,companies.issues,companies.unmanaged,companies.outdated,companies.unscanned,companies.policy_not_applied,companies.isolated,fetch_failures
```

One row per customer, ranked by total issues, with the counts that need action. A company whose fetch failed has status fetch_failed and is listed in fetch_failures: its zero counts mean unknown, not clean.

### What changed this week

```bash
bitdefender-cli fleet changes --since 7d --agent
```

Diffs the latest fleet snapshot against the newest one at least 7 days older, or the oldest saved snapshot if none is that old (the note says so).

### Month-end billing

```bash
bitdefender-cli usage billing --month 09/2026 --agent --select rows.company_name,rows.product_type,rows.usage,rows.own_share,rows.delta,rows.flags,fetch_failures
```

Per-company seats with own share and month-over-month swings; flags mark a comparison withheld because a fetch failed, and a company whose usage could not be read at all is listed in fetch_failures, not in rows.

### Find then isolate a machine

```bash
bitdefender-cli fleet locate LAPTOP-7 --data-source live --agent
```

Get the current endpoint id and company, then pass the id to isolation isolate --endpoint-ids.

### Endpoints in one company

```bash
bitdefender-cli endpoints list --parent-id 54a28b41b1a43d89367b23fd --per-page 100 --filters '{"depth":{"allItemsRecursively":true}}' --agent --select items.name,items.isManaged,items.policy.name
```

Include endpoints in custom groups and narrow the large payload to the fields an agent needs.

## Usage

Run `bitdefender-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `BITDEFENDER_CONFIG_DIR`, `BITDEFENDER_DATA_DIR`, `BITDEFENDER_STATE_DIR`, or `BITDEFENDER_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `BITDEFENDER_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export BITDEFENDER_HOME=/srv/bitdefender
bitdefender-cli doctor
```

On Windows (PowerShell):

```powershell
$env:BITDEFENDER_HOME = "C:\ProgramData\bitdefender"
bitdefender-cli doctor
```

Under `BITDEFENDER_HOME=/srv/bitdefender`, the four dirs resolve to `/srv/bitdefender/config`, `/srv/bitdefender/data`, `/srv/bitdefender/state`, and `/srv/bitdefender/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "bitdefender": {
      "command": "bitdefender-mcp",
      "env": {
        "BITDEFENDER_HOME": "/srv/bitdefender"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `BITDEFENDER_DATA_DIR` overrides an explicit `--home` for that kind. Use `BITDEFENDER_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `BITDEFENDER_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `bitdefender-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### accounts

Control Center user accounts

- **`bitdefender-cli accounts create`** - Create a user account and assign it a password.
- **`bitdefender-cli accounts delete`** - Delete a specific user account.
- **`bitdefender-cli accounts get`** - Retrieve information for a specific GravityZone account.
- **`bitdefender-cli accounts list`** - Display all the user accounts that belong to a company.
- **`bitdefender-cli accounts update`** - Updates a user account identified through the account ID.

### api_key

Details of the API key in use

- **`bitdefender-cli api-key`** - Returns details about the API key used.

### aws_ec2

Amazon EC2 integration and hourly usage

- **`bitdefender-cli aws-ec2 company-by-account`** - Retrieves information regarding the managed companies that have configured the Amazon EC2 integration with a specific AWS account.
- **`bitdefender-cli aws-ec2 configure`** - Configures the Amazon EC2 integration using the provided Amazon Resource Name of a valid AWS Cross-Account Role.
- **`bitdefender-cli aws-ec2 disable`** - Disables a previously configured Amazon EC2 integration.
- **`bitdefender-cli aws-ec2 external-id`** - Returns the last External ID that was generated using the generateAmazonEC2ExternalIdForCrossAccountRole method.
- **`bitdefender-cli aws-ec2 generate-external-id`** - Generates an External ID. External IDs are required to configure AWS Cross-Account Roles, which you will need for Amazon EC2 integrations.
- **`bitdefender-cli aws-ec2 hourly-usage`** - Exposes the hourly usage for each Amazon instance category (micro, medium etc.).

### blocklist

Hash, path and connection blocklist rules

- **`bitdefender-cli blocklist add`** - Set up one or more rules that you can use to add items to blocklists.
- **`bitdefender-cli blocklist list`** - Lists all items present in a Blocklist.
- **`bitdefender-cli blocklist remove`** - Removes one or more items from the Blocklist, identified by their IDs.

### companies

Partner company tree: list, find, inspect, create, suspend, activate, delete

- **`bitdefender-cli companies activate`** - Activates a suspended company.
- **`bitdefender-cli companies create`** - Creates a Customer or Partner company account.
- **`bitdefender-cli companies delete`** - Deletes a company.
- **`bitdefender-cli companies find`** - Searches for all managed companies containing the specified string in their name.
- **`bitdefender-cli companies get`** - Retrieves the details of a company that you manage.
- **`bitdefender-cli companies get-by-user`** - Retrieves the details of a company linked to an account identified through the given username.
- **`bitdefender-cli companies list`** - Returns the list of companies under a parent company or from a company folder.
- **`bitdefender-cli companies suspend`** - Suspends an active company account, with the following implications:
- **`bitdefender-cli companies update`** - Updates the details of a company that you manage.

### company_folders

Company folders that organize child companies under a partner

- **`bitdefender-cli company-folders create`** - Creates a new folder under an existing folder or company.
- **`bitdefender-cli company-folders delete`** - Deletes a company folder.
- **`bitdefender-cli company-folders list`** - Returns the list of company folders under a company or under a company folder.
- **`bitdefender-cli company-folders move`** - Moves a company or company folder under a parent company or company folder.

### custom_fields

Custom field definitions attached to companies

- **`bitdefender-cli custom-fields create`** - Creates a custom field definition.
- **`bitdefender-cli custom-fields delete`** - Deletes a custom field definition.
- **`bitdefender-cli custom-fields list`** - Retrieves the custom fields definitions.
- **`bitdefender-cli custom-fields update`** - Updates a custom field definition.

### custom_rules

Custom detection and exclusion rules

- **`bitdefender-cli custom-rules create`** - Create a custom rule.
- **`bitdefender-cli custom-rules delete`** - Delete a custom rule.
- **`bitdefender-cli custom-rules list`** - Retrieves the list of custom rules for a specified company.
- **`bitdefender-cli custom-rules update`** - Edit any existing custom exclusion or detection rule by referencing its Rule ID.

### email_verification

Email verification codes for account contact addresses

- **`bitdefender-cli email-verification`** - Send a verification code to a specific email address and verify that a user has access to it. This is done to allow the email to be used as contact information or to be assigned to a specific account.

### endpoint_tags

Endpoint tags

- **`bitdefender-cli endpoint-tags`** - Returns details about the endpoint tags associated with the specified companies.

### endpoints

Managed and unmanaged endpoints

- **`bitdefender-cli endpoints assign-policy`** - Assign a policy to one or more endpoints. This method works on all endpoint types, including containers. The policy is not assigned to targets that have an enforced policy.
- **`bitdefender-cli endpoints delete`** - Deletes an endpoint.
- **`bitdefender-cli endpoints get`** - Retrieves comprehensive information about a specific endpoint managed in GravityZone.
- **`bitdefender-cli endpoints kill-process`** - Terminate an active process using its process ID, its path, the endpoint where it is running, and, if available, the ID of the incident it generated.
- **`bitdefender-cli endpoints list`** - Returns the list of endpoints.
- **`bitdefender-cli endpoints move`** - Moves a list of endpoints to a custom group.
- **`bitdefender-cli endpoints move-to-company`** - Moves a list of endpoints to another protected entity (move between companies)
- **`bitdefender-cli endpoints set-label`** - Sets a new label to an endpoint.

### groups

Custom endpoint groups

- **`bitdefender-cli groups create`** - Creates a new custom group of endpoints.
- **`bitdefender-cli groups delete`** - Deletes a custom group.
- **`bitdefender-cli groups list`** - Retrieves the list of groups under a specified group.
- **`bitdefender-cli groups move`** - Moves a custom group to another custom group.

### incidents

EDR/XDR incidents: list, inspect, triage, assign, annotate

- **`bitdefender-cli incidents assign`** - Enables bulk assignment and reassignment of EDR and XDR incidents to eligible GravityZone users. Assignees can also be removed through the same method.
- **`bitdefender-cli incidents get`** - Retrieve information regarding a specific incident, by referencing its ID.
- **`bitdefender-cli incidents get-many`** - Retrieve information on multiple incidents, by referencing their IDs.
- **`bitdefender-cli incidents list`** - Retrieves details about Endpoint and Organization incidents for a specified company based on the applied filters.
- **`bitdefender-cli incidents note`** - Assigns a note to an incident.
- **`bitdefender-cli incidents set-priority`** - Change the priority of an incident.
- **`bitdefender-cli incidents set-status`** - Changes the status of an incident.
- **`bitdefender-cli incidents similar-emails`** - Retrieve emails similar to a given email, scoped to the same company.

### integrations

Configured third-party integrations (VMware and others)

- **`bitdefender-cli integrations create`** - Create a new integration under the Integrations hub.
- **`bitdefender-cli integrations delete`** - Delete a specific integration that was configured under the Integrations hub from a target company.
- **`bitdefender-cli integrations get`** - Request information about an integration configured under the Integrations hub.
- **`bitdefender-cli integrations list`** - Allows you to get a list of all the integrations configured on a specific company under the Integrations hub.
- **`bitdefender-cli integrations manage`** - Enable or disable an integration configured under the Integrations hub.
- **`bitdefender-cli integrations update`** - Update an integration configured under the Integrations hub.

### integrators

Integrator accounts linked to the company

- **`bitdefender-cli integrators add`** - Assign an endpoint as an integrator for a specific integration.
- **`bitdefender-cli integrators list`** - Retrieve a list of integrators configured for a specific integration.
- **`bitdefender-cli integrators remove`** - Unassign a specific endpoint as an integrator. The endpoint remains available, but it will no longer serve as the integrator.

### inventory

Network inventory tree: companies, groups and endpoints

- **`bitdefender-cli inventory list`** - Returns network inventory items.
- **`bitdefender-cli inventory roots`** - Returns the root containers for a company. A root container refers to special groups, such as: Companies, Network, Computers and Groups.

### investigation

Investigation packages, file retrieval and remote command execution

- **`bitdefender-cli investigation collect-package`** - Initiate a forensic collection task on an endpoint. The endpoint collects a predefined investigation package and uploads it to a centralized storage location in GravityZone for further analysis. For details on the data gathered as part of an investigation package, refer to Investigation Package data.
- **`bitdefender-cli investigation file-url`** - Check the status of a task started through the collectInvestigationPackage method. Once the task has been completed, you can also retrieve the download URL for the investigation package uploaded in GravityZone using this method.
- **`bitdefender-cli investigation kill-process`** - Initiate a task that terminates a running process on a managed endpoint.
- **`bitdefender-cli investigation retrieve-file`** - Retrieve a file from an endpoint within your company and upload it as an archive to a centralized storage location in GravityZone, where it can be downloaded for further analysis.
- **`bitdefender-cli investigation run-command`** - Remotely execute a command on an endpoint and upload the resulting output as a log file to the GravityZone centralized storage.

### isolation

Network isolation of endpoints and restore from isolation

- **`bitdefender-cli isolation isolate`** - Creates one or more tasks to isolate the specified endpoint or endpoints.
- **`bitdefender-cli isolation restore`** - Creates one or more tasks to restore the specified endpoint or endpoints from isolation.

### license

License and subscription details for a company

- **`bitdefender-cli license add-product-key`** - Adds a license or add-on key to the existing list of licenses and add-on keys.
- **`bitdefender-cli license get`** - Retrieves the license information for a company.
- **`bitdefender-cli license remove-product-key`** - Removes a license key from the given company.
- **`bitdefender-cli license set-key`** - Sets a standalone or an add-on license key for a company.
- **`bitdefender-cli license set-monthly-subscription`** - Enables monthly subscription for a company.

### live_search

Live Search osquery-style queries on endpoints

- **`bitdefender-cli live-search`** - Create tasks that retrieve information about events and system statistics directly from online endpoints. The task uses Osquery, an operating system instrumentation framework that uses the SQLite query language.

### maintenance_windows

Patch management maintenance windows

- **`bitdefender-cli maintenance-windows approved-patches`** - Returns a list of manually approved patches from the endpoints of a specific company.
- **`bitdefender-cli maintenance-windows assign`** - Assigns a set of maintenance windows to a policy.
- **`bitdefender-cli maintenance-windows create`** - Creates a maintenance window which you can use in Patch Management.
- **`bitdefender-cli maintenance-windows delete`** - Deletes a maintenance window.
- **`bitdefender-cli maintenance-windows get`** - Retrieves all the information related to a maintenance window. It can only apply to your own company or companies that can be managed from above.
- **`bitdefender-cli maintenance-windows list`** - Will display a list of available Maintenance Windows for a company. It only applies for companies where the user has visibility.
- **`bitdefender-cli maintenance-windows unassign`** - Unassigns a set of maintenance windows from a policy.
- **`bitdefender-cli maintenance-windows update`** - Modifies a patch management maintenance window.

### notifications

Notification settings for the API key's user account

- **`bitdefender-cli notifications configure`** - Configures the notification settings for a given user account.
- **`bitdefender-cli notifications get`** - Returns the notifications settings.

### packages

Security agent installation packages and download links

- **`bitdefender-cli packages create`** - Creates an installation package.
- **`bitdefender-cli packages delete`** - Deletes a package identified through the provided package ID.
- **`bitdefender-cli packages get`** - Retrieves information about the configuration of a specific package identified through the provided ID.
- **`bitdefender-cli packages install-links`** - Returns the installation links and full kits for an installation package.
- **`bitdefender-cli packages list`** - Returns the list of available packages.
- **`bitdefender-cli packages update`** - Update an installation package.

### patches

Installed and missing patches per endpoint or company (Patch Management add-on)

- **`bitdefender-cli patches installed`** - Request a list of installed patches that are associated to specific endpoints or a specific company.
- **`bitdefender-cli patches missing`** - Request a list of missing patches that are associated to specific endpoints or a specific company.

### phasr

PHASR attack-surface recommendations, resources and identities

- **`bitdefender-cli phasr apply`** - Applies recommendations by ID and returns success and failure details for each one requested.
- **`bitdefender-cli phasr edit-rules-access`** - Restrict or allow access to behavioral profiles based on the recommendation generated by a specific PHASR rule.
- **`bitdefender-cli phasr identities`** - Retrieve all detected identities for a company.
- **`bitdefender-cli phasr monitored-rules`** - Displays all the monitored PHASR rules across all behavioral categories for a specific company.
- **`bitdefender-cli phasr profiles`** - Retrieve information for the behavioral profiles that a specific recommendation applies to.
- **`bitdefender-cli phasr recommendations`** - Retrieve PHASR recommendations for a company.
- **`bitdefender-cli phasr request-access-action`** - Takes action on request access recommendations by either allowing or denying them. This method processes multiple recommendations in bulk and returns detailed status for each operation.
- **`bitdefender-cli phasr resources`** - Retrieve all detected resources for a company.
- **`bitdefender-cli phasr rule-data`** - Retrieved detailed information regarding a specific PHASR rule and the behavioral profiles it applies to.

### policies

Security policies

- **`bitdefender-cli policies get`** - Retrieves all information related to a security policy.
- **`bitdefender-cli policies list`** - Retrieves the list of available policies.
- **`bitdefender-cli policies set-modules`** - Enable or disable the settings associated to a specific policy.

### push

Event Push Service settings and statistics

- **`bitdefender-cli push get`** - Gets the push event settings.
- **`bitdefender-cli push reset-stats`** - Resets the push event statistics and errors.
- **`bitdefender-cli push set`** - Sets the push event settings.
- **`bitdefender-cli push stats`** - Gets the push event statistics and errors.
- **`bitdefender-cli push test`** - Sends a test event.

### quarantine

Quarantined files and Exchange items

- **`bitdefender-cli quarantine add-file`** - Creates a new task to add a file to quarantine.
- **`bitdefender-cli quarantine empty`** - Creates a new task to empty the quarantine.
- **`bitdefender-cli quarantine list`** - Retrieves the list of quarantined items available for a company.
- **`bitdefender-cli quarantine release-exchange`** - Creates a new task to release items from the quarantine to their intended recipients.
- **`bitdefender-cli quarantine remove`** - Creates a new task to remove items from quarantine.
- **`bitdefender-cli quarantine restore`** - Creates a new task to restore items from the quarantine.
- **`bitdefender-cli quarantine restore-exchange`** - Creates a new task to restore items from the quarantine for Exchange Servers.

### reports

Scheduled reports and their download links

- **`bitdefender-cli reports create`** - Creates a new instant or scheduled report, based on the parameters received, and returns the ID of the new created report.
- **`bitdefender-cli reports delete`** - Deletes a report by its ID.
- **`bitdefender-cli reports download-links`** - Returns an Object with information regarding the report availability for download and the corresponding download links.
- **`bitdefender-cli reports list`** - Returns the list of scheduled reports, according to the parameters received.

### response_actions

Incident response actions and their status

- **`bitdefender-cli response-actions create`** - Take response actions on user nodes generated in GravityZone XDR incidents or your own SOC-generated incidents. You can make the request based on an XDR incident ID, or based on user data specified in the node.
- **`bitdefender-cli response-actions status`** - Request the status and the result of a response action.

### tasks

Scan, reconfigure, gather-logs and sandbox tasks

- **`bitdefender-cli tasks delete`** - Delete a specific task, based on its ID.
- **`bitdefender-cli tasks gather-logs`** - Create a task that collects diagnostic logs from an online endpoint. The task retrieves log files from the target endpoint and stores them according to the specified storage configuration: locally on the endpoint, on a network share, or in an S3-compatible bucket.
- **`bitdefender-cli tasks list-scans`** - Returns the list of scan tasks.
- **`bitdefender-cli tasks reconfigure`** - Creates a new Reconfigure Agent task. With this task you can choose which modules to install on target agents.
- **`bitdefender-cli tasks sandbox-submit`** - Create a Sandbox Analyzer task and submit up to 5 files for analysis. The task pulls the files that need to be analyzed from a target endpoint, along with any other files that need to be invloved in the process.
- **`bitdefender-cli tasks scan`** - Creates a new scan task.
- **`bitdefender-cli tasks scan-by-mac`** - Generate a scan task for managed endpoints identified by their MAC address.
- **`bitdefender-cli tasks status`** - Retrieves information about the status of a given task identified using its ID.

### usage

Monthly license usage (billing seats) per company

- **`bitdefender-cli usage monthly`** - Exposes the monthly usage for a company in a target month. Returns only usage from endpoints with the default product type - Endpoint Security. If usage from other product types is needed, refer to the getMonthlyUsagePerProductType method.
- **`bitdefender-cli usage monthly-by-product`** - Exposes the monthly usage of a company in a target month. Returns the usage per product type, for all available product types.

### yara

YARA scans on endpoints

- **`bitdefender-cli yara`** - Initiate an On-demand scan on the specified targets, using a YARA rule to detect files that match the rule conditions.


### companies


- **`bitdefender-cli companies tree`** - Your own company plus every child company and sub-partner under it as one flat list with ids, type, parent and depth.


### fleet


- **`bitdefender-cli fleet health`** - One ranked table of every child company's unmanaged, outdated, unscanned, policy-not-applied and isolated endpoints, saved as a dated snapshot.

- **`bitdefender-cli fleet incident-queue`** - Open EDR/XDR incidents across all child companies, ranked by severity and age with unassigned and SLA-breach flags.

- **`bitdefender-cli fleet rmm-gap`** - Joins an RMM device export to GravityZone inventory to list devices with no agent, discovered-but-unmanaged machines, and agents the RMM does not know.

- **`bitdefender-cli fleet changes`** - What changed between saved fleet health snapshots (needs at least two): new and removed endpoints, newly unmanaged, outdated or isolated machines, policy flips.

- **`bitdefender-cli fleet threats`** - The same quarantined file (SHA256) seen at two or more companies in a window, and which of those companies already blocklist it.

- **`bitdefender-cli fleet locate`** - Find a machine by hostname, IP, MAC or label across every child company and get its endpoint id and company.


### usage


- **`bitdefender-cli usage billing`** - Per-company billable usage for a month with own share after the child rollup, month-over-month seat swings, and flags wherever a needed fetch failed.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`bitdefender-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`bitdefender-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`bitdefender-cli learnings list`** - Inspect taught rows
- **`bitdefender-cli learnings forget <query>`** - Undo a teach
- **`bitdefender-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`bitdefender-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`bitdefender-cli teach-pattern`** - Install a query/resource template up front
- **`bitdefender-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `BITDEFENDER_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `bitdefender-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
bitdefender-cli accounts list

# JSON for scripting and agents
bitdefender-cli accounts list --json
# Filter to specific fields
bitdefender-cli accounts list --json --select email,id,profile

# Dry run  -  show the request without sending
bitdefender-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
bitdefender-cli accounts list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` prints the JSON-RPC method (after the `#` in the URL) and the parameters it would send, without sending; the wire body wraps them in a JSON-RPC envelope
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Writes run immediately** - generated write commands (create, delete, isolate, quarantine, tasks) run as soon as they are invoked and do not prompt; preview with `--dry-run` first. `--agent` does not imply `--yes`
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - the fleet commands save dated snapshots to a local SQLite store; `fleet changes`, `fleet locate` and `fleet rmm-gap` can answer from them offline
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` `fleet health --fail-on` found issues, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
bitdefender-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `bitdefender-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/bitdefender-cli/config.toml` (`%USERPROFILE%\.config\bitdefender-cli\config.toml` on Windows); `--home`, `BITDEFENDER_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `BITDEFENDER_API_KEY` | per_call | Yes | Set to your API credential. |
| `BITDEFENDER_BASE_URL` | per_call | No | Control Center API Access URL from My Account > Control Center API, including `/api`. Defaults to `https://cloud.gravityzone.bitdefender.com/api`; set it for other regions and on-premises installs. |
| `BITDEFENDER_CONFIG` | path | No | Path to the config file, instead of `config.toml` in the config directory. |
| `BITDEFENDER_NO_LEARN` | per_call | No | Set to `true` to turn off the self-learning loop for deterministic runs. |
| `BITDEFENDER_MCP_HTTP_TOKEN` | mcp | Only with `--transport http` | Bearer token the MCP server requires on every HTTP request. |
| `BITDEFENDER_MCP_READ_ONLY` | mcp | No | Set to exactly `1` so the MCP server's `bitdefender_execute` runs only GravityZone read methods and refuses writes and the reads that return or take a credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `bitdefender-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `bitdefender-cli doctor` to check credentials
- Run `bitdefender-cli auth status` to see where the key comes from (it never prints the key)
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **HTTP 401: Authorization header not passed, HTTP Basic Authentication should be used**  -  export BITDEFENDER_API_KEY=<key> (PowerShell: $env:BITDEFENDER_API_KEY = "<key>"); the CLI sends it as the Basic username with an empty password
- **HTTP 403: API Key is not allowed to access the selected API: <Area>**  -  Edit the key in Control Center (My Account > API keys) and enable that API area, or run bitdefender-cli api-key --json to see enabled areas
- **HTTP 400: The requested API is not available**  -  Set BITDEFENDER_BASE_URL to the Access URL from My Account > Control Center API, including the /api suffix
- **HTTP 429 Too many requests on getIncidentsList or a per-company incidents loop**  -  getIncidentsList allows about 3 calls per minute per key; use fleet incident-queue (one partner-scope call that spaces extra --max-pages pages 21s apart), or wait for Retry-After

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**bitdefender/n8n-nodes-gravityzone**](https://github.com/bitdefender/n8n-nodes-gravityzone)  -  TypeScript
- [**demisto/content GravityZone pack**](https://github.com/demisto/content/tree/master/Packs/GravityZone)  -  Python
- [**WYRE-AI/bitdefender-mcp**](https://github.com/WYRE-AI/bitdefender-mcp)  -  TypeScript
- [**guardiankey/gsage-ai-soc**](https://github.com/guardiankey/gsage-ai-soc)  -  Python
- [**SEKOIA-IO/automation-library**](https://github.com/SEKOIA-IO/automation-library/tree/develop/BitDefender%20GravityZone)  -  Python
- [**coval3nte/go-bd-gravityzone**](https://github.com/coval3nte/go-bd-gravityzone)  -  Go
- [**Michael-Bockhoff-GmbH/gravityzone-erpnext-billing**](https://github.com/Michael-Bockhoff-GmbH/gravityzone-erpnext-billing)  -  Python
- [**runZeroInc/runzero-custom-integrations**](https://github.com/runZeroInc/runzero-custom-integrations/tree/main/bitdefender-gravityzone)  -  Starlark

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
