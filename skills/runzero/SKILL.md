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
      - kind: go
        bins: [runzero-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/runzero/cmd/runzero-cli
---

# runZero  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `runzero-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install runzero --cli-only
   ```
2. Verify: `runzero-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/runzero/cmd/runzero-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

runZero's console and SDK answer one query at a time, online-only, against a license-bound daily quota. This CLI syncs your assets, services, software, certificates, and vulnerability findings into local SQLite once, then turns 'what changed on my attack surface' (diff), 'what critical thing is exposed and vulnerable' (triage), and 'who is affected by this CVE' (affected) into a single quota-free local command  -  with --json, --select, --agent output, and typed exit codes for clean scripting.

## When to Use This CLI

Reach for this CLI when you need to query, slice, or diff a runZero attack surface repeatedly without spending the license-bound daily API quota, when you need to join assets to services/software/vulnerabilities offline (which the live API cannot do in one call), or when you are scripting inventory pulls, exposure triage, scan orchestration, or fleet hygiene into your own tooling. It is the right choice for MSP/MSSP weekly exposure sweeps, incident-response CVE pivots, and asset-inventory cleanup.

## Unique Capabilities

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


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
runzero-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

runZero uses an HTTP Bearer API token. The token prefix encodes its scope: an Account key (starts with CT) reaches everything, an Organization key (starts with OT) reaches the org and export endpoints, and an Export token (starts with ET) reaches export endpoints only. Set your token in the environment and run doctor to confirm it is accepted before syncing (doctor checks connectivity, not scope  -  the prefix letters above are the scope signal).

Run `runzero-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  runzero-cli account create-asset-ownership-types --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
runzero-cli feedback "the --since flag is inclusive but docs say exclusive"
runzero-cli feedback --stdin < notes.txt
runzero-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/runzero-cli/feedback.jsonl`. They are never POSTed unless `RUNZERO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `RUNZERO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
runzero-cli profile save briefing --json
runzero-cli --profile briefing account create-asset-ownership-types
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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `runzero-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/runzero/cmd/runzero-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add runzero-mcp -- runzero-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which runzero-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   runzero-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `runzero-cli <command> --help`.
