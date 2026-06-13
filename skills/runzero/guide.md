# runZero CLI

**Every runZero query, plus a local SQLite copy of your whole attack surface that diffs over time, joins assets to vulnerabilities offline, and costs zero API quota to re-slice.**

runZero's console and SDK answer one query at a time, online-only, against a license-bound daily quota. This CLI syncs your assets, services, software, certificates, and vulnerability findings into local SQLite once, then turns 'what changed on my attack surface' (diff), 'what critical thing is exposed and vulnerable' (triage), and 'who is affected by this CVE' (affected) into a single quota-free local command  -  with --json, --select, --agent output, and typed exit codes for clean scripting.

## Install

The recommended path installs both the `runzero-cli` binary and the `pp-runzero` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install runzero
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install runzero --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install runzero --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install runzero --agent claude-code
npx -y @mvanhorn/printing-press-library install runzero --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/runzero/cmd/runzero-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/runzero-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install runzero --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-runzero --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-runzero --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install runzero --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/runzero-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `RUNZERO_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/runzero/cmd/runzero-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "runzero": {
      "command": "runzero-mcp",
      "env": {
        "RUNZERO_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

runZero uses an HTTP Bearer API token. The token prefix encodes its scope: an Account key (starts with CT) reaches everything, an Organization key (starts with OT) reaches the org and export endpoints, and an Export token (starts with ET) reaches export endpoints only. Set your token in the environment and run doctor to confirm it is accepted before syncing (doctor checks connectivity, not scope  -  the prefix letters above are the scope signal).

## Quick Start

```bash
# Confirm the API is reachable and your token scope (CT/OT/ET) is valid before anything else.
runzero-cli doctor

# Pull assets, services, software, certificates, and vulnerabilities into local SQLite  -  the foundation for the offline transcendence commands.
runzero-cli inventory sync

# Query the live API with native runZero search syntax when you need fresh data.
runzero-cli org get-assets --search 'alive:t os:"Windows"'

# Rank internet-facing assets by criticality and vulnerability exposure in one local join.
runzero-cli triage --internet-facing --agent

# See what appeared or disappeared on your attack surface in the last week.
runzero-cli diff --since 7d

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Attack-surface intelligence (local store)
- **`diff`**  -  Show which assets, services, ports, and software appeared or disappeared between two local syncs.

  _Reach for this to answer 'what changed on my attack surface' without re-querying the live API or paying any daily quota._

  ```bash
  runzero-cli diff --since 7d --agent
  ```
- **`triage`**  -  Rank assets by criticality joined to their exposed services and high-severity vulnerability findings.

  _Pick this when you need the single ranked list of 'what critical thing is exposed and vulnerable' instead of three separate API queries reconciled by hand._

  ```bash
  runzero-cli triage --internet-facing --agent
  ```
- **`affected`**  -  Given a CVE, list every affected asset with its services and criticality.

  _During an incident, use this to get the blast radius of an advisory in one command._

  ```bash
  runzero-cli affected CVE-2024-3094 --agent
  ```
- **`exposure-map`**  -  Roll up which protocols and ports are exposed across a CIDR, asset-counted per service.

  _Use this to see which subnets concentrate risky exposed services before planning a scan or segmentation change._

  ```bash
  runzero-cli exposure-map 10.0.0.0/8 --agent
  ```
- **`exposure-delta`**  -  List services and ports that became newly exposed or newly vulnerable since the last sync, ranked by asset criticality.

  _Run this after each sync for the Monday-morning answer to 'what NEW thing is exposed', without burning daily API quota._

  ```bash
  runzero-cli exposure-delta --agent
  ```
- **`certs-expiring`**  -  List TLS certificates expiring within a window or using weak crypto, joined to the asset and service presenting them.

  _Use this in the weekly exposure sweep to catch expiring or weak certs with the asset context needed to act._

  ```bash
  runzero-cli certs-expiring --days 30 --agent
  ```

### Fleet hygiene (local store)
- **`software rollup`**  -  Group installed software by product and version with a count of assets running each.

  _Use this to find how many distinct versions of a package are deployed and which assets run the laggards._

  ```bash
  runzero-cli software rollup <name> --agent   # e.g. <name> = openssl
  ```
- **`stale`**  -  Bucket assets by last-seen age, end-of-life OS, and missing tag/owner  -  emitting IDs ready to pipe into the bulk commands.

  _Run this for a weekly hygiene pass, then act on the asset_ids with the org bulk-asset commands (e.g. org remove-bulk-assets to retire them)._

  ```bash
  runzero-cli stale --days 30 --agent
  ```

### Scan orchestration
- **`scan-watch`**  -  Start a scan on a site and follow the task to completion in one command, with a typed exit code on the result.

  _Use this to script 'scan and wait' into a pipeline instead of babysitting the console task page or re-pasting a polling loop._

  ```bash
  runzero-cli scan-watch 550e8400-e29b-41d4-a716-446655440000 --targets 10.0.0.0/24 --json
  ```

## Recipes


### Sync then triage exposure offline

```bash
runzero-cli inventory sync && runzero-cli triage --internet-facing --agent
```

Pull the inventory once, then rank internet-facing assets by criticality and vulnerability exposure entirely from the local store.

### Narrow a deeply-nested asset list for an agent

```bash
runzero-cli org get-assets --search 'alive:t protocols:smb2' --agent --select id,addresses,os,services.protocol,services.port
```

Live asset records nest services and addresses; --select with dotted paths returns only the fields an agent needs so it does not burn context on the full payload.

### CVE blast radius during an incident

```bash
runzero-cli affected CVE-2024-3094 --agent
```

Pivot from a single advisory to every affected asset, its services, and its criticality in one local join.

### Weekly stale-asset hygiene list

```bash
runzero-cli stale --days 45 --json --select asset_id,name,reasons
```

Find assets not seen in 45 days (plus EOL-OS and untagged/unowned) locally; feed the asset_ids into org update-bulk-asset-tags or org remove-bulk-assets to clean up.

### Software version rollup across the fleet

```bash
runzero-cli software rollup <name> --agent   # e.g. <name> = openssl
```

Group every installed build of a product (here, OpenSSL) by version with the count of assets running each, so you can target the outdated ones. Omit `<name>` to roll up the entire software inventory.

## Usage

Run `runzero-cli --help` for the full command reference and flag list.

## Commands

### account

Requires Account key (starts with CT), or OAuth

- **`runzero-cli account create-asset-ownership-types`** - Create new asset ownership types
- **`runzero-cli account create-credential`** - Create a new credential
- **`runzero-cli account create-custom-integration`** - Create a new custom integration
- **`runzero-cli account create-custom-integration-and-id`** - Replace custom integration at provided ID
- **`runzero-cli account create-group`** - Create a new group
- **`runzero-cli account create-group-mapping`** - Create a new SSO group mapping
- **`runzero-cli account create-key`** - Create a new key
- **`runzero-cli account create-organization`** - Create a new organization
- **`runzero-cli account create-organization-export-token`** - Create a new export token for an organization
- **`runzero-cli account create-scan-template`** - Create a new scan template
- **`runzero-cli account create-user`** - Create a new user account
- **`runzero-cli account create-user-invite`** - Create a new user account and send an email invite
- **`runzero-cli account delete-asset-ownership-type`** - Delete a single asset ownership type
- **`runzero-cli account delete-asset-ownership-types`** - Delete asset ownership types
- **`runzero-cli account delete-custom-integration`** - Delete an custom integration
- **`runzero-cli account delete-organization-export-token`** - Removes the export token from the specified organization
- **`runzero-cli account delete-organization-export-token-deprecated`** - This API has been deprecated.  Please use `DELETE /account/orgs/{org_id}/exportTokens/{key_id}` instead.  This API will fail if more than one export tokens exist for the given organization.
- **`runzero-cli account export-events-json`** - System event log as JSON
- **`runzero-cli account export-events-jsonl`** - System event log as JSON line-delimited
- **`runzero-cli account get-agents`** - Get all agents across all organizations
- **`runzero-cli account get-apitoken`** - Generate an access token using an API client
- **`runzero-cli account get-asset-ownership-types`** - Get all asset ownership types
- **`runzero-cli account get-credential`** - Get credential details
- **`runzero-cli account get-credentials`** - Get all account credentials
- **`runzero-cli account get-custom-integration`** - Get single custom integration
- **`runzero-cli account get-custom-integrations`** - Get all custom integrations
- **`runzero-cli account get-group`** - Get group details
- **`runzero-cli account get-group-mapping`** - Get SSO group mapping details
- **`runzero-cli account get-group-mappings`** - Get all SSO group mappings
- **`runzero-cli account get-groups`** - Get all groups
- **`runzero-cli account get-key`** - Get key details
- **`runzero-cli account get-keys`** - Get all active API keys
- **`runzero-cli account get-license`** - Get license details
- **`runzero-cli account get-organization`** - Get organization details
- **`runzero-cli account get-organization-export-token`** - Get export token details
- **`runzero-cli account get-organization-export-tokens`** - Get all active export tokens for an organization
- **`runzero-cli account get-organizations`** - Get all organization details
- **`runzero-cli account get-scan-template`** - Get scan template details
- **`runzero-cli account get-scan-templates`** - Get all scan templates across all organizations (up to 1000)
- **`runzero-cli account get-sites`** - Get all sites details across all organizations
- **`runzero-cli account get-tasks`** - Get all task details across all organizations (up to 1000)
- **`runzero-cli account get-user`** - Get user details
- **`runzero-cli account get-users`** - Get all users
- **`runzero-cli account remove-credential`** - Remove this credential
- **`runzero-cli account remove-group`** - Remove this group
- **`runzero-cli account remove-group-mapping`** - Remove this SSO group mapping
- **`runzero-cli account remove-key`** - Remove this key
- **`runzero-cli account remove-organization`** - Remove this organization
- **`runzero-cli account remove-scan-template`** - Remove scan template
- **`runzero-cli account remove-user`** - Remove this user
- **`runzero-cli account reset-user-lockout`** - Resets the user's lockout status
- **`runzero-cli account reset-user-mfa`** - Resets the user's MFA tokens
- **`runzero-cli account reset-user-password`** - Sends the user a password reset email
- **`runzero-cli account rotate-apitoken`** - Rotate the API client secret
- **`runzero-cli account rotate-key`** - Rotates the key secret
- **`runzero-cli account rotate-organization-export-token`** - Rotates an organization export token and returns the updated token
- **`runzero-cli account rotate-organization-export-token-deprecated`** - This API has been deprecated.  Please use `PATCH /account/orgs/{org_id}/exportTokens/{key_id}/rotate` instead.  This API will fail if more than one export tokens exist for the given organization.
- **`runzero-cli account update-asset-ownership-type`** - Update a single asset ownership type
- **`runzero-cli account update-asset-ownership-types`** - Update asset ownership types
- **`runzero-cli account update-custom-integration`** - Update a single custom integration
- **`runzero-cli account update-group`** - Update an existing group
- **`runzero-cli account update-group-mapping`** - Update an existing SSO group mapping
- **`runzero-cli account update-organization`** - Update organization details
- **`runzero-cli account update-scan-template`** - Update scan template
- **`runzero-cli account update-user`** - Update a user's details

### health

Manage health

- **`runzero-cli health`** - Returns a health check status (cloud and self-hosted)

### org

Manage org

- **`runzero-cli org bulk-remove-custom-integration`** - Remove custom integration from a list of assets
- **`runzero-cli org clear-bulk-asset-owners`** - Clear all owners across multiple assets based on a search query
- **`runzero-cli org clear-bulk-asset-tags`** - Clear all tags across multiple assets based on a search query
- **`runzero-cli org create-sample`** - Create a traffic sampling task for a given site
- **`runzero-cli org create-scan`** - Create a scan task for a given site
- **`runzero-cli org create-site`** - Create a new site
- **`runzero-cli org export-asset-metrics-json`** - Export asset metrics
- **`runzero-cli org export-asset-top-hwcsv`** - Top asset hardware products as CSV
- **`runzero-cli org export-asset-top-oscsv`** - Top asset operating systems as CSV
- **`runzero-cli org export-asset-top-tags-csv`** - Top asset tags as CSV
- **`runzero-cli org export-asset-top-types-csv`** - Top asset types as CSV
- **`runzero-cli org export-services-top-products-csv`** - Top service products as CSV
- **`runzero-cli org export-services-top-protocols-csv`** - Top service protocols as CSV
- **`runzero-cli org export-services-top-tcpcsv`** - Top TCP services as CSV
- **`runzero-cli org export-services-top-udpcsv`** - Top UDP services as CSV
- **`runzero-cli org get-agent`** - Get details for a single agent. Legacy path for /org/explorers/{explorer_id}
- **`runzero-cli org get-agents`** - Get all agents. Legacy path for /org/explorers
- **`runzero-cli org get-asset`** - Get asset details
- **`runzero-cli org get-assets`** - Get all assets
- **`runzero-cli org get-custom-integration`** - Get single custom integration
- **`runzero-cli org get-custom-integrations`** - Get all custom integrations
- **`runzero-cli org get-explorer`** - Get details for a single explorer. This is the same call as legacy path /org/agents/{agent_id}
- **`runzero-cli org get-explorers`** - Get all explorers. This is the same call as legacy path /org/agents
- **`runzero-cli org get-hosted-zone`** - Get details for a single Hosted Zone. Hosted Zones are only available to Enterprise licensed customers.
- **`runzero-cli org get-hosted-zones`** - Get all hosted zones. Hosted Zones are only available to Enterprise licensed customers.
- **`runzero-cli org get-key`** - Get API key details
- **`runzero-cli org get-organization`** - Get organization details
- **`runzero-cli org get-service`** - Get service details
- **`runzero-cli org get-services`** - Get all services
- **`runzero-cli org get-site`** - Get site details
- **`runzero-cli org get-sites`** - Get all sites
- **`runzero-cli org get-task`** - Get task details
- **`runzero-cli org get-task-change-report`** - Returns a temporary task change report data url
- **`runzero-cli org get-task-log`** - Returns a temporary task log data url
- **`runzero-cli org get-task-scan-data`** - Returns a temporary task scan data url
- **`runzero-cli org get-tasks`** - Get all tasks (last 1000)
- **`runzero-cli org get-wireless-lan`** - Get wireless LAN details
- **`runzero-cli org get-wireless-lans`** - Get all wireless LANs
- **`runzero-cli org hide-task`** - Signal that a completed task should be hidden
- **`runzero-cli org import-nessus-scan-data`** - Import a Nessus scan data file into a site
- **`runzero-cli org import-packet-data`** - Import a packet capture file into a site
- **`runzero-cli org import-scan-data`** - Import a scan data file into a site
- **`runzero-cli org merge-assets`** - Merge multiple assets
- **`runzero-cli org remove-agent`** - Remove and uninstall an agent. Legacy path for /org/explorers/{explorer_id}
- **`runzero-cli org remove-asset`** - Remove an asset
- **`runzero-cli org remove-asset-source`** - Remove single source from asset
- **`runzero-cli org remove-bulk-assets`** - Removes multiple assets by ID
- **`runzero-cli org remove-custom-integration`** - Remove single custom integration from asset
- **`runzero-cli org remove-explorer`** - Remove and uninstall an explorer. This is the same call as legacy path /org/agents/{agent_id}
- **`runzero-cli org remove-key`** - Remove the current API key
- **`runzero-cli org remove-service`** - Remove a service
- **`runzero-cli org remove-site`** - Remove a site and associated assets
- **`runzero-cli org remove-wireless-lan`** - Remove a wireless LAN
- **`runzero-cli org rotate-key`** - Rotate the API key secret and return the updated key
- **`runzero-cli org stop-task`** - Signal that a task should be stopped or canceled.This will also remove recurring and scheduled tasks
- **`runzero-cli org update-agent-settings`** - Update the settings associated with the agent. Legacy path for /org/explorers/{explorer_id}
- **`runzero-cli org update-asset-comments`** - Update asset comments
- **`runzero-cli org update-asset-criticality`** - Update asset criticality
- **`runzero-cli org update-asset-owners`** - Update asset owners
- **`runzero-cli org update-asset-tags`** - Update asset tags
- **`runzero-cli org update-bulk-asset-criticality`** - Update criticality across multiple assets based on a search query
- **`runzero-cli org update-bulk-asset-owners`** - Update asset owners across multiple assets based on a search query
- **`runzero-cli org update-bulk-asset-tags`** - Update tags across multiple assets based on a search query
- **`runzero-cli org update-explorer-settings`** - Update the settings associated with the Explorer. This is the same call as legacy path /org/agents/{agent_id}
- **`runzero-cli org update-organization`** - Update organization details
- **`runzero-cli org update-site`** - Update a site definition
- **`runzero-cli org update-task`** - Update task parameters
- **`runzero-cli org upgrade-agent`** - Force an agent to update and restart. Legacy path for /org/explorers/{explorer_id}/update
- **`runzero-cli org upgrade-explorer`** - Force an explorer to update and restart. This is the same call as legacy path /org/agents/{agent_id}/update

### releases

Manage releases

- **`runzero-cli releases get-latest-agent-version`** - Returns latest agent version
- **`runzero-cli releases get-latest-platform-version`** - Returns latest platform version
- **`runzero-cli releases get-latest-scanner-version`** - Returns latest scanner version

### runzero-export

Manage runzero export

- **`runzero-cli runzero-export assets-cisco-csv`** - Cisco serial number and model name export for Cisco Smart Net Total Care Service.
- **`runzero-cli runzero-export assets-csv`** - Asset inventory as CSV
- **`runzero-cli runzero-export assets-json`** - Exports the asset inventory
- **`runzero-cli runzero-export assets-jsonl`** - Asset inventory as JSON line-delimited
- **`runzero-cli runzero-export assets-nmap-xml`** - Asset inventory as Nmap-style XML
- **`runzero-cli runzero-export certificates-csv`** - Export the certificate inventory as CSV
- **`runzero-cli runzero-export certificates-json`** - Export the certificate inventory as JSON
- **`runzero-cli runzero-export certificates-jsonl`** - Export the certificate inventory as JSONL line-delimited
- **`runzero-cli runzero-export directory-groups-csv`** - Group inventory as CSV
- **`runzero-cli runzero-export directory-groups-json`** - Exports the group inventory
- **`runzero-cli runzero-export directory-groups-jsonl`** - Group inventory as JSON line-delimited
- **`runzero-cli runzero-export directory-users-csv`** - User inventory as CSV
- **`runzero-cli runzero-export directory-users-json`** - Exports the user inventory
- **`runzero-cli runzero-export directory-users-jsonl`** - User inventory as JSON line-delimited
- **`runzero-cli runzero-export findings-csv`** - Export findings as CSV
- **`runzero-cli runzero-export findings-json`** - Export findings as JSON
- **`runzero-cli runzero-export findings-jsonl`** - Export findings as JSON line-delimited
- **`runzero-cli runzero-export services-csv`** - Service inventory as CSV
- **`runzero-cli runzero-export services-json`** - Service inventory as JSON
- **`runzero-cli runzero-export services-jsonl`** - Service inventory as JSON line-delimited
- **`runzero-cli runzero-export sites-csv`** - Site list as CSV
- **`runzero-cli runzero-export sites-json`** - Export all sites
- **`runzero-cli runzero-export sites-jsonl`** - Site list as JSON line-delimited
- **`runzero-cli runzero-export snmparpcache-csv`** - SNMP ARP cache data as CSV
- **`runzero-cli runzero-export snow-assets-csv`** - Export an asset inventory as CSV for ServiceNow integration
- **`runzero-cli runzero-export snow-assets-json`** - Exports the asset inventory as JSON
- **`runzero-cli runzero-export snow-service-graph-assets-json`** - Exports the asset inventory as JSON
- **`runzero-cli runzero-export snow-services-csv`** - Export a service inventory as CSV for ServiceNow integration
- **`runzero-cli runzero-export software-csv`** - Software inventory as CSV
- **`runzero-cli runzero-export software-json`** - Exports the software inventory
- **`runzero-cli runzero-export software-jsonl`** - Software inventory as JSON line-delimited
- **`runzero-cli runzero-export splunk-asset-sync-created-json`** - Exports the asset inventory in a sync-friendly manner using created_at as a checkpoint. Requires the Splunk entitlement.
- **`runzero-cli runzero-export splunk-asset-sync-updated-json`** - Exports the asset inventory in a sync-friendly manner using updated_at as a checkpoint. Requires the Splunk entitlement.
- **`runzero-cli runzero-export subnet-utilization-stats-csv`** - Subnet utilization statistics as as CSV
- **`runzero-cli runzero-export tasks-json`** - Exports organization tasks
- **`runzero-cli runzero-export tasks-jsonl`** - Organization tasks as JSON line-delimited
- **`runzero-cli runzero-export vulnerabilities-csv`** - Export the vulnerability inventory as CSV
- **`runzero-cli runzero-export vulnerabilities-json`** - Export the vulnerability inventory as JSON
- **`runzero-cli runzero-export vulnerabilities-jsonl`** - Export the vulnerability inventory as JSON line-delimited
- **`runzero-cli runzero-export wireless-csv`** - Wireless inventory as CSV
- **`runzero-cli runzero-export wireless-json`** - Wireless inventory as JSON
- **`runzero-cli runzero-export wireless-jsonl`** - Wireless inventory as JSON line-delimited

### runzero-import

Manage runzero import

- **`runzero-cli runzero-import <orgID>`** - Assets can be discovered, imported, and merged by runZero scan tasks, first-party integrations, and third-party
defined custom integrations. See [/account/custom-integrations](#/account/getAccountCustomIntegrations). Currently only assets for custom integrations are importable here.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
runzero-cli account create-asset-ownership-types

# JSON for scripting and agents
runzero-cli account create-asset-ownership-types --json

# Filter to specific fields
runzero-cli account create-asset-ownership-types --json --select id,name,status

# Dry run  -  show the request without sending
runzero-cli account create-asset-ownership-types --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
runzero-cli account create-asset-ownership-types --agent
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

## Health Check

```bash
runzero-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/runzero-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `RUNZERO_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `runzero-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `runzero-cli doctor` to check credentials
- Verify the environment variable is set: `echo $RUNZERO_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on /account/* but /export/* works**  -  Your token is an Export (ET) or Organization (OT) key. Account-admin endpoints need an Account key (starts with CT). Check your token's prefix letters (CT/OT/ET)  -  doctor confirms connectivity, not scope.
- **Daily quota exhausted / 429 rate limited**  -  runZero limits calls/day to your licensed-asset count. Run inventory sync once, then query the local store with the offline analysis commands (triage, diff, affected, exposure-map) instead of re-hitting the API.
- **diff or triage returns nothing**  -  Run inventory sync first  -  the offline analysis commands read the local SQLite store, not the live API. Run inventory sync at least twice for diff to have two snapshots to compare.
- **Org endpoints 404 / wrong org**  -  OT and ET tokens are org-scoped and carry their org in the token. To act on a different org, use that org's token or an Account (CT) key with the org id.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**runzero-sdk-py**](https://github.com/runZeroInc/runzero-sdk-py)  -  Python
- [**runzero-api**](https://github.com/runZeroInc/runzero-api)  -  YAML
- [**runzero-api-go**](https://github.com/runZeroInc/runzero-api-go)  -  Go
- [**runzero-custom-integrations**](https://github.com/runZeroInc/runzero-custom-integrations)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
