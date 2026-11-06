---
name: runzero
description: "Every runZero query, plus a local SQLite copy of your whole attack surface that diffs over time, joins assets to vulnerabilities offline, and costs zero API quota to re-slice. Trigger phrases: `list runzero assets`, `triage my runzero exposure`, `what changed on my attack surface`, `which assets are affected by this CVE`, `find stale assets in runzero`, `use runzero`, `run runzero-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "runZero"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - runzero-cli
    install:
      - kind: script
        bins: [runzero-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.ps1
---

# runZero - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `runzero-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.ps1 | iex
   ```
3. Verify: `runzero-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `runzero-cli` and `runzero-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

runZero's console and SDK answer one query at a time, online-only, against a license-bound daily quota. This CLI syncs your assets, services, software, certificates, and vulnerability findings into local SQLite once, then turns 'what changed on my attack surface' (diff), 'what critical thing is exposed and vulnerable' (triage), and 'who is affected by this CVE' (affected) into a single quota-free local command  -  with --json, --select, --agent output, and typed exit codes for clean scripting.

## When to Use This CLI

Reach for this CLI when you need to query, slice, or diff a runZero attack surface repeatedly without spending the license-bound daily API quota, when you need to join assets to services/software/vulnerabilities offline (which the live API cannot do in one call), or when you are scripting inventory pulls, exposure triage, scan orchestration, or fleet hygiene into your own tooling. It is the right choice for MSP/MSSP weekly exposure sweeps, incident-response CVE pivots, and asset-inventory cleanup.

## Unique Capabilities

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
  runzero-cli software rollup openssl --agent
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

## Command Reference

**account**  -  Requires Account key (starts with CT), or OAuth

- `runzero-cli account create-asset-ownership-types`  -  Create new asset ownership types
- `runzero-cli account create-credential`  -  Create a new credential
- `runzero-cli account create-custom-integration`  -  Create a new custom integration
- `runzero-cli account create-custom-integration-and-id`  -  Replace custom integration at provided ID
- `runzero-cli account create-group`  -  Create a new group
- `runzero-cli account create-group-mapping`  -  Create a new SSO group mapping
- `runzero-cli account create-key`  -  Create a new key
- `runzero-cli account create-organization`  -  Create a new organization
- `runzero-cli account create-organization-export-token`  -  Create a new export token for an organization
- `runzero-cli account create-scan-template`  -  Create a new scan template
- `runzero-cli account create-user`  -  Create a new user account
- `runzero-cli account create-user-invite`  -  Create a new user account and send an email invite
- `runzero-cli account delete-asset-ownership-type`  -  Delete a single asset ownership type
- `runzero-cli account delete-asset-ownership-types`  -  Delete asset ownership types
- `runzero-cli account delete-custom-integration`  -  Delete an custom integration
- `runzero-cli account delete-organization-export-token`  -  Removes the export token from the specified organization
- `runzero-cli account delete-organization-export-token-deprecated`  -  This API has been deprecated. Please use `DELETE /account/orgs/{org_id}/exportTokens/{key_id}` instead.
- `runzero-cli account export-events-json`  -  System event log as JSON
- `runzero-cli account export-events-jsonl`  -  System event log as JSON line-delimited
- `runzero-cli account get-agents`  -  Get all agents across all organizations
- `runzero-cli account get-apitoken`  -  Generate an access token using an API client
- `runzero-cli account get-asset-ownership-types`  -  Get all asset ownership types
- `runzero-cli account get-credential`  -  Get credential details
- `runzero-cli account get-credentials`  -  Get all account credentials
- `runzero-cli account get-custom-integration`  -  Get single custom integration
- `runzero-cli account get-custom-integrations`  -  Get all custom integrations
- `runzero-cli account get-group`  -  Get group details
- `runzero-cli account get-group-mapping`  -  Get SSO group mapping details
- `runzero-cli account get-group-mappings`  -  Get all SSO group mappings
- `runzero-cli account get-groups`  -  Get all groups
- `runzero-cli account get-key`  -  Get key details
- `runzero-cli account get-keys`  -  Get all active API keys
- `runzero-cli account get-license`  -  Get license details
- `runzero-cli account get-organization`  -  Get organization details
- `runzero-cli account get-organization-export-token`  -  Get export token details
- `runzero-cli account get-organization-export-tokens`  -  Get all active export tokens for an organization
- `runzero-cli account get-organizations`  -  Get all organization details
- `runzero-cli account get-scan-template`  -  Get scan template details
- `runzero-cli account get-scan-templates`  -  Get all scan templates across all organizations (up to 1000)
- `runzero-cli account get-sites`  -  Get all sites details across all organizations
- `runzero-cli account get-tasks`  -  Get all task details across all organizations (up to 1000)
- `runzero-cli account get-user`  -  Get user details
- `runzero-cli account get-users`  -  Get all users
- `runzero-cli account remove-credential`  -  Remove this credential
- `runzero-cli account remove-group`  -  Remove this group
- `runzero-cli account remove-group-mapping`  -  Remove this SSO group mapping
- `runzero-cli account remove-key`  -  Remove this key
- `runzero-cli account remove-organization`  -  Remove this organization
- `runzero-cli account remove-scan-template`  -  Remove scan template
- `runzero-cli account remove-user`  -  Remove this user
- `runzero-cli account reset-user-lockout`  -  Resets the user's lockout status
- `runzero-cli account reset-user-mfa`  -  Resets the user's MFA tokens
- `runzero-cli account reset-user-password`  -  Sends the user a password reset email
- `runzero-cli account rotate-apitoken`  -  Rotate the API client secret
- `runzero-cli account rotate-key`  -  Rotates the key secret
- `runzero-cli account rotate-organization-export-token`  -  Rotates an organization export token and returns the updated token
- `runzero-cli account rotate-organization-export-token-deprecated`  -  This API has been deprecated. Please use `PATCH /account/orgs/{org_id}/exportTokens/{key_id}/rotate` instead.
- `runzero-cli account update-asset-ownership-type`  -  Update a single asset ownership type
- `runzero-cli account update-asset-ownership-types`  -  Update asset ownership types
- `runzero-cli account update-custom-integration`  -  Update a single custom integration
- `runzero-cli account update-group`  -  Update an existing group
- `runzero-cli account update-group-mapping`  -  Update an existing SSO group mapping
- `runzero-cli account update-organization`  -  Update organization details
- `runzero-cli account update-scan-template`  -  Update scan template
- `runzero-cli account update-user`  -  Update a user's details

**health**  -  Manage health

- `runzero-cli health`  -  Returns a health check status (cloud and self-hosted)

**org**  -  Manage org

- `runzero-cli org bulk-remove-custom-integration`  -  Remove custom integration from a list of assets
- `runzero-cli org clear-bulk-asset-owners`  -  Clear all owners across multiple assets based on a search query
- `runzero-cli org clear-bulk-asset-tags`  -  Clear all tags across multiple assets based on a search query
- `runzero-cli org create-sample`  -  Create a traffic sampling task for a given site
- `runzero-cli org create-scan`  -  Create a scan task for a given site
- `runzero-cli org create-site`  -  Create a new site
- `runzero-cli org export-asset-metrics-json`  -  Export asset metrics
- `runzero-cli org export-asset-top-hwcsv`  -  Top asset hardware products as CSV
- `runzero-cli org export-asset-top-oscsv`  -  Top asset operating systems as CSV
- `runzero-cli org export-asset-top-tags-csv`  -  Top asset tags as CSV
- `runzero-cli org export-asset-top-types-csv`  -  Top asset types as CSV
- `runzero-cli org export-services-top-products-csv`  -  Top service products as CSV
- `runzero-cli org export-services-top-protocols-csv`  -  Top service protocols as CSV
- `runzero-cli org export-services-top-tcpcsv`  -  Top TCP services as CSV
- `runzero-cli org export-services-top-udpcsv`  -  Top UDP services as CSV
- `runzero-cli org get-agent`  -  Get details for a single agent. Legacy path for /org/explorers/{explorer_id}
- `runzero-cli org get-agents`  -  Get all agents. Legacy path for /org/explorers
- `runzero-cli org get-asset`  -  Get asset details
- `runzero-cli org get-assets`  -  Get all assets
- `runzero-cli org get-custom-integration`  -  Get single custom integration
- `runzero-cli org get-custom-integrations`  -  Get all custom integrations
- `runzero-cli org get-explorer`  -  Get details for a single explorer. This is the same call as legacy path /org/agents/{agent_id}
- `runzero-cli org get-explorers`  -  Get all explorers. This is the same call as legacy path /org/agents
- `runzero-cli org get-hosted-zone`  -  Get details for a single Hosted Zone. Hosted Zones are only available to Enterprise licensed customers.
- `runzero-cli org get-hosted-zones`  -  Get all hosted zones. Hosted Zones are only available to Enterprise licensed customers.
- `runzero-cli org get-key`  -  Get API key details
- `runzero-cli org get-organization`  -  Get organization details
- `runzero-cli org get-service`  -  Get service details
- `runzero-cli org get-services`  -  Get all services
- `runzero-cli org get-site`  -  Get site details
- `runzero-cli org get-sites`  -  Get all sites
- `runzero-cli org get-task`  -  Get task details
- `runzero-cli org get-task-change-report`  -  Returns a temporary task change report data url
- `runzero-cli org get-task-log`  -  Returns a temporary task log data url
- `runzero-cli org get-task-scan-data`  -  Returns a temporary task scan data url
- `runzero-cli org get-tasks`  -  Get all tasks (last 1000)
- `runzero-cli org get-wireless-lan`  -  Get wireless LAN details
- `runzero-cli org get-wireless-lans`  -  Get all wireless LANs
- `runzero-cli org hide-task`  -  Signal that a completed task should be hidden
- `runzero-cli org import-nessus-scan-data`  -  Import a Nessus scan data file into a site
- `runzero-cli org import-packet-data`  -  Import a packet capture file into a site
- `runzero-cli org import-scan-data`  -  Import a scan data file into a site
- `runzero-cli org merge-assets`  -  Merge multiple assets
- `runzero-cli org remove-agent`  -  Remove and uninstall an agent. Legacy path for /org/explorers/{explorer_id}
- `runzero-cli org remove-asset`  -  Remove an asset
- `runzero-cli org remove-asset-source`  -  Remove single source from asset
- `runzero-cli org remove-bulk-assets`  -  Removes multiple assets by ID
- `runzero-cli org remove-custom-integration`  -  Remove single custom integration from asset
- `runzero-cli org remove-explorer`  -  Remove and uninstall an explorer. This is the same call as legacy path /org/agents/{agent_id}
- `runzero-cli org remove-key`  -  Remove the current API key
- `runzero-cli org remove-service`  -  Remove a service
- `runzero-cli org remove-site`  -  Remove a site and associated assets
- `runzero-cli org remove-wireless-lan`  -  Remove a wireless LAN
- `runzero-cli org rotate-key`  -  Rotate the API key secret and return the updated key
- `runzero-cli org stop-task`  -  Signal that a task should be stopped or canceled.This will also remove recurring and scheduled tasks
- `runzero-cli org update-agent-settings`  -  Update the settings associated with the agent. Legacy path for /org/explorers/{explorer_id}
- `runzero-cli org update-asset-comments`  -  Update asset comments
- `runzero-cli org update-asset-criticality`  -  Update asset criticality
- `runzero-cli org update-asset-owners`  -  Update asset owners
- `runzero-cli org update-asset-tags`  -  Update asset tags
- `runzero-cli org update-bulk-asset-criticality`  -  Update criticality across multiple assets based on a search query
- `runzero-cli org update-bulk-asset-owners`  -  Update asset owners across multiple assets based on a search query
- `runzero-cli org update-bulk-asset-tags`  -  Update tags across multiple assets based on a search query
- `runzero-cli org update-explorer-settings`  -  Update the settings associated with the Explorer. This is the same call as legacy path /org/agents/{agent_id}
- `runzero-cli org update-organization`  -  Update organization details
- `runzero-cli org update-site`  -  Update a site definition
- `runzero-cli org update-task`  -  Update task parameters
- `runzero-cli org upgrade-agent`  -  Force an agent to update and restart. Legacy path for /org/explorers/{explorer_id}/update
- `runzero-cli org upgrade-explorer`  -  Force an explorer to update and restart. This is the same call as legacy path /org/agents/{agent_id}/update

**releases**  -  Manage releases

- `runzero-cli releases get-latest-agent-version`  -  Returns latest agent version
- `runzero-cli releases get-latest-platform-version`  -  Returns latest platform version
- `runzero-cli releases get-latest-scanner-version`  -  Returns latest scanner version

**runzero-export**  -  Manage runzero export

- `runzero-cli runzero-export assets-cisco-csv`  -  Cisco serial number and model name export for Cisco Smart Net Total Care Service.
- `runzero-cli runzero-export assets-csv`  -  Asset inventory as CSV
- `runzero-cli runzero-export assets-json`  -  Exports the asset inventory
- `runzero-cli runzero-export assets-jsonl`  -  Asset inventory as JSON line-delimited
- `runzero-cli runzero-export assets-nmap-xml`  -  Asset inventory as Nmap-style XML
- `runzero-cli runzero-export certificates-csv`  -  Export the certificate inventory as CSV
- `runzero-cli runzero-export certificates-json`  -  Export the certificate inventory as JSON
- `runzero-cli runzero-export certificates-jsonl`  -  Export the certificate inventory as JSONL line-delimited
- `runzero-cli runzero-export directory-groups-csv`  -  Group inventory as CSV
- `runzero-cli runzero-export directory-groups-json`  -  Exports the group inventory
- `runzero-cli runzero-export directory-groups-jsonl`  -  Group inventory as JSON line-delimited
- `runzero-cli runzero-export directory-users-csv`  -  User inventory as CSV
- `runzero-cli runzero-export directory-users-json`  -  Exports the user inventory
- `runzero-cli runzero-export directory-users-jsonl`  -  User inventory as JSON line-delimited
- `runzero-cli runzero-export findings-csv`  -  Export findings as CSV
- `runzero-cli runzero-export findings-json`  -  Export findings as JSON
- `runzero-cli runzero-export findings-jsonl`  -  Export findings as JSON line-delimited
- `runzero-cli runzero-export services-csv`  -  Service inventory as CSV
- `runzero-cli runzero-export services-json`  -  Service inventory as JSON
- `runzero-cli runzero-export services-jsonl`  -  Service inventory as JSON line-delimited
- `runzero-cli runzero-export sites-csv`  -  Site list as CSV
- `runzero-cli runzero-export sites-json`  -  Export all sites
- `runzero-cli runzero-export sites-jsonl`  -  Site list as JSON line-delimited
- `runzero-cli runzero-export snmparpcache-csv`  -  SNMP ARP cache data as CSV
- `runzero-cli runzero-export snow-assets-csv`  -  Export an asset inventory as CSV for ServiceNow integration
- `runzero-cli runzero-export snow-assets-json`  -  Exports the asset inventory as JSON
- `runzero-cli runzero-export snow-service-graph-assets-json`  -  Exports the asset inventory as JSON
- `runzero-cli runzero-export snow-services-csv`  -  Export a service inventory as CSV for ServiceNow integration
- `runzero-cli runzero-export software-csv`  -  Software inventory as CSV
- `runzero-cli runzero-export software-json`  -  Exports the software inventory
- `runzero-cli runzero-export software-jsonl`  -  Software inventory as JSON line-delimited
- `runzero-cli runzero-export splunk-asset-sync-created-json`  -  Exports the asset inventory in a sync-friendly manner using created_at as a checkpoint. Requires the Splunk entitlement.
- `runzero-cli runzero-export splunk-asset-sync-updated-json`  -  Exports the asset inventory in a sync-friendly manner using updated_at as a checkpoint. Requires the Splunk entitlement.
- `runzero-cli runzero-export subnet-utilization-stats-csv`  -  Subnet utilization statistics as as CSV
- `runzero-cli runzero-export tasks-json`  -  Exports organization tasks
- `runzero-cli runzero-export tasks-jsonl`  -  Organization tasks as JSON line-delimited
- `runzero-cli runzero-export vulnerabilities-csv`  -  Export the vulnerability inventory as CSV
- `runzero-cli runzero-export vulnerabilities-json`  -  Export the vulnerability inventory as JSON
- `runzero-cli runzero-export vulnerabilities-jsonl`  -  Export the vulnerability inventory as JSON line-delimited
- `runzero-cli runzero-export wireless-csv`  -  Wireless inventory as CSV
- `runzero-cli runzero-export wireless-json`  -  Wireless inventory as JSON
- `runzero-cli runzero-export wireless-jsonl`  -  Wireless inventory as JSON line-delimited

**runzero-import**  -  Manage runzero import

- `runzero-cli runzero-import <orgID>`  -  Assets can be discovered, imported, and merged by runZero scan tasks, first-party integrations


**affected**


- `runzero-cli affected`  -  Given a CVE, list every affected asset with its services and criticality.


**certs-expiring**


- `runzero-cli certs-expiring`  -  List TLS certificates expiring within a window or using weak crypto, joined to the asset and service presenting them.


**diff**


- `runzero-cli diff`  -  Show which assets, services, ports, and software appeared or disappeared between two local syncs.


**exposure-delta**


- `runzero-cli exposure-delta`  -  List services and ports that became newly exposed or newly vulnerable since the last sync, ranked by asset criticality.


**exposure-map**


- `runzero-cli exposure-map`  -  Roll up which protocols and ports are exposed across a CIDR, asset-counted per service.


**scan-watch**


- `runzero-cli scan-watch`  -  Start a scan on a site and follow the task to completion in one command, with a typed exit code on the result.


**software**


- `runzero-cli software rollup`  -  Group installed software by product and version with a count of assets running each.


**stale**


- `runzero-cli stale`  -  Bucket assets by last-seen age, end-of-life OS, and missing tag/owner  -  emitting IDs ready to pipe into the bulk commands.


**triage**


- `runzero-cli triage`  -  Rank assets by criticality joined to their exposed services and high-severity vulnerability findings.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
runzero-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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
runzero-cli software rollup openssl --agent
```

Group every installed OpenSSL build by version with the count of assets running each, so you can target the outdated ones.

## Auth Setup

runZero uses an HTTP Bearer API token. The token prefix encodes its scope: an Account key (starts with CT) reaches everything, an Organization key (starts with OT) reaches the org and export endpoints, and an Export token (starts with ET) reaches export endpoints only. Set your token in the environment and run doctor to confirm it is accepted before syncing (doctor checks connectivity, not scope  -  the prefix letters above are the scope signal).

Run `runzero-cli doctor` to verify setup.

## Credential safety

- The API key (`RUNZERO_API_KEY`: an account, organization or export token) goes only as `Authorization: Bearer <key>` to the scheme, host and port of the configured base URL: `https://console.runzero.com/api/v1.0` by default, or `RUNZERO_BASE_URL` (a self-hosted console). A redirect to any other host is refused, so the key never reaches it.
- A credential a response carries (an API key's token from `account get-keys`, an organization's export and download tokens, a minted access token, a client secret) prints as `[credential withheld]`, and the rest of the record prints; the local store keeps the same masked record. Any other output, file or MCP result that would repeat the key, its header or one of those credentials, in any encoding, is replaced whole by `[credkit: withheld from <sink>: it carried a credential this program holds]`. Responses are never cached on disk.
- Only `credentials.toml` and `config.toml` (both mode 0600) hold a credential: `config.toml` keeps the base URL as you configured it, a password or token in it included. `auth set-token` reads the key from stdin and holds it before anything prints.
- `--dry-run` sends nothing. Over MCP, `runzero_execute`, `import`, `scan_watch` and every other tool that changes runZero data return a preview and send only with `confirm: true`; the exports and the offline analysis tools are read-only; and a tool argument can never name a file to read or write.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  runzero-cli account export-events-json --agent
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `RUNZERO_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `RUNZERO_CONFIG_DIR`, `RUNZERO_DATA_DIR`, `RUNZERO_STATE_DIR`, `RUNZERO_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `RUNZERO_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `runzero-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "runzero": {
        "command": "runzero-mcp",
        "env": {
          "RUNZERO_HOME": "/srv/runzero"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `RUNZERO_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `RUNZERO_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `RUNZERO_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `RUNZERO_DATA_SOURCE` sets the `--data-source` default.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
runzero-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "runzero-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `runzero-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `runzero-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `runzero-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
runzero-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
runzero-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
runzero-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
runzero-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`runzero-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `RUNZERO_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
runzero-cli feedback "the --since flag is inclusive but docs say exclusive"
runzero-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `RUNZERO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `RUNZERO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
runzero-cli profile save briefing --json
runzero-cli --profile briefing account export-events-json
runzero-cli profile list --json
runzero-cli profile show briefing
runzero-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `runzero-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add runzero-mcp -- runzero-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `runzero-mcp --transport http` requires `RUNZERO_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `RUNZERO_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `RUNZERO_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which runzero-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   runzero-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `runzero-cli <command> --help`.
