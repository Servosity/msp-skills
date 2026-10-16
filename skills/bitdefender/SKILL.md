---
name: bitdefender
description: "Use when the user asks about Bitdefender GravityZone across every customer: which child companies have unmanaged, outdated or unscanned endpoints, what changed since the last check, the open EDR/XDR incident queue, malware quarantined at several clients, which company a machine is in, RMM devices with no agent, or monthly seat usage for billing. Covers 140 GravityZone Public API methods as commands. Trigger phrases: `which customers have unprotected endpoints`, `gravityzone monthly usage for billing`, `open bitdefender incidents across all clients`, `find a machine across all gravityzone customers`, `same malware quarantined at several clients`, `use bitdefender gravityzone`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Bitdefender"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - bitdefender-cli
    install:
      - kind: script
        bins: [bitdefender-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.ps1
---

# Bitdefender GravityZone  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `bitdefender-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

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

The installer downloads the `bitdefender-cli` and `bitdefender-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

140 GravityZone Partners and Cloud Public API methods are typed commands with --json and --dry-run; over MCP they are reached through bitdefender_search and bitdefender_execute. On top, fleet health, fleet incident-queue and usage billing walk the whole partner tree so an MSP sees every customer at once, and fleet changes diffs saved fleet health snapshots. Not yet verified against a live tenant.

## When to Use This CLI

Use this CLI when an agent or MSP technician needs GravityZone data or actions from a script or chat: endpoints, companies, policies, quarantine and incidents; scans, isolation and quarantine tasks; licence and monthly usage. Prefer the fleet commands for any question that spans more than one child company. fleet changes needs two saved fleet health snapshots before it reports anything; fleet locate answers from the latest snapshot, so pass --data-source live before acting on an endpoint id. fleet locate and fleet rmm-gap state where their inventory came from in results.source (snapshot or live) and results.taken_at; meta.source says local when the snapshot answered and live after a live fan-out. Write commands run as soon as they are invoked: preview with --dry-run. These commands have not yet been verified against a live tenant.

## Anti-triggers

Do not use this CLI for:
- Do not use it to install or uninstall the Bitdefender agent on a machine; get the installer link with packages install-links and deploy it with your RMM
- Do not use it for Bitdefender consumer products (Total Security, Mobile Security); it only speaks the GravityZone Control Center API
- Do not use it for real-time event streaming; configure the Event Push Service (push set) and receive events in your SIEM
- Do not use it to edit full policy settings; the API only toggles policy modules (policies set-modules) and assigns policies

## Unique Capabilities

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

## Command Reference

**accounts**  -  Control Center user accounts

- `bitdefender-cli accounts create`  -  Create a user account and assign it a password.
- `bitdefender-cli accounts delete`  -  Delete a specific user account.
- `bitdefender-cli accounts get`  -  Retrieve information for a specific GravityZone account.
- `bitdefender-cli accounts list`  -  Display all the user accounts that belong to a company.
- `bitdefender-cli accounts update`  -  Updates a user account identified through the account ID.

**api_key**  -  Details of the API key in use

- `bitdefender-cli api-key`  -  Returns details about the API key used.

**aws_ec2**  -  Amazon EC2 integration and hourly usage

- `bitdefender-cli aws-ec2 company-by-account`  -  Retrieves information regarding the managed companies that have configured the Amazon EC2 integration with a specific
- `bitdefender-cli aws-ec2 configure`  -  Configures the Amazon EC2 integration using the provided Amazon Resource Name of a valid AWS Cross-Account Role.
- `bitdefender-cli aws-ec2 disable`  -  Disables a previously configured Amazon EC2 integration.
- `bitdefender-cli aws-ec2 external-id`  -  Returns the last External ID that was generated using the generateAmazonEC2ExternalIdForCrossAccountRole method.
- `bitdefender-cli aws-ec2 generate-external-id`  -  Generates an External ID.
- `bitdefender-cli aws-ec2 hourly-usage`  -  Exposes the hourly usage for each Amazon instance category (micro, medium etc.).

**blocklist**  -  Hash, path and connection blocklist rules

- `bitdefender-cli blocklist add`  -  Set up one or more rules that you can use to add items to blocklists.
- `bitdefender-cli blocklist list`  -  Lists all items present in a Blocklist.
- `bitdefender-cli blocklist remove`  -  Removes one or more items from the Blocklist, identified by their IDs.

**companies**  -  Partner company tree: list, find, inspect, create, suspend, activate, delete

- `bitdefender-cli companies activate`  -  Activates a suspended company.
- `bitdefender-cli companies create`  -  Creates a Customer or Partner company account.
- `bitdefender-cli companies delete`  -  Deletes a company.
- `bitdefender-cli companies find`  -  Searches for all managed companies containing the specified string in their name.
- `bitdefender-cli companies get`  -  Retrieves the details of a company that you manage.
- `bitdefender-cli companies get-by-user`  -  Retrieves the details of a company linked to an account identified through the given username.
- `bitdefender-cli companies list`  -  Returns the list of companies under a parent company or from a company folder.
- `bitdefender-cli companies suspend`  -  Suspends an active company account, with the following implications:
- `bitdefender-cli companies update`  -  Updates the details of a company that you manage.

**company_folders**  -  Company folders that organize child companies under a partner

- `bitdefender-cli company-folders create`  -  Creates a new folder under an existing folder or company.
- `bitdefender-cli company-folders delete`  -  Deletes a company folder.
- `bitdefender-cli company-folders list`  -  Returns the list of company folders under a company or under a company folder.
- `bitdefender-cli company-folders move`  -  Moves a company or company folder under a parent company or company folder.

**custom_fields**  -  Custom field definitions attached to companies

- `bitdefender-cli custom-fields create`  -  Creates a custom field definition.
- `bitdefender-cli custom-fields delete`  -  Deletes a custom field definition.
- `bitdefender-cli custom-fields list`  -  Retrieves the custom fields definitions.
- `bitdefender-cli custom-fields update`  -  Updates a custom field definition.

**custom_rules**  -  Custom detection and exclusion rules

- `bitdefender-cli custom-rules create`  -  Create a custom rule.
- `bitdefender-cli custom-rules delete`  -  Delete a custom rule.
- `bitdefender-cli custom-rules list`  -  Retrieves the list of custom rules for a specified company.
- `bitdefender-cli custom-rules update`  -  Edit any existing custom exclusion or detection rule by referencing its Rule ID.

**email_verification**  -  Email verification codes for account contact addresses

- `bitdefender-cli email-verification`  -  Send a verification code to a specific email address and verify that a user has access to it.

**endpoint_tags**  -  Endpoint tags

- `bitdefender-cli endpoint-tags`  -  Returns details about the endpoint tags associated with the specified companies.

**endpoints**  -  Managed and unmanaged endpoints

- `bitdefender-cli endpoints assign-policy`  -  Assign a policy to one or more endpoints. This method works on all endpoint types, including containers.
- `bitdefender-cli endpoints delete`  -  Deletes an endpoint.
- `bitdefender-cli endpoints get`  -  Retrieves comprehensive information about a specific endpoint managed in GravityZone.
- `bitdefender-cli endpoints kill-process`  -  Terminate an active process using its process ID, its path, the endpoint where it is running, and, if available
- `bitdefender-cli endpoints list`  -  Returns the list of endpoints.
- `bitdefender-cli endpoints move`  -  Moves a list of endpoints to a custom group.
- `bitdefender-cli endpoints move-to-company`  -  Moves a list of endpoints to another protected entity (move between companies)
- `bitdefender-cli endpoints set-label`  -  Sets a new label to an endpoint.

**groups**  -  Custom endpoint groups

- `bitdefender-cli groups create`  -  Creates a new custom group of endpoints.
- `bitdefender-cli groups delete`  -  Deletes a custom group.
- `bitdefender-cli groups list`  -  Retrieves the list of groups under a specified group.
- `bitdefender-cli groups move`  -  Moves a custom group to another custom group.

**incidents**  -  EDR/XDR incidents: list, inspect, triage, assign, annotate

- `bitdefender-cli incidents assign`  -  Enables bulk assignment and reassignment of EDR and XDR incidents to eligible GravityZone users.
- `bitdefender-cli incidents get`  -  Retrieve information regarding a specific incident, by referencing its ID.
- `bitdefender-cli incidents get-many`  -  Retrieve information on multiple incidents, by referencing their IDs.
- `bitdefender-cli incidents list`  -  Retrieves details about Endpoint and Organization incidents for a specified company based on the applied filters.
- `bitdefender-cli incidents note`  -  Assigns a note to an incident.
- `bitdefender-cli incidents set-priority`  -  Change the priority of an incident.
- `bitdefender-cli incidents set-status`  -  Changes the status of an incident.
- `bitdefender-cli incidents similar-emails`  -  Retrieve emails similar to a given email, scoped to the same company.

**integrations**  -  Configured third-party integrations (VMware and others)

- `bitdefender-cli integrations create`  -  Create a new integration under the Integrations hub.
- `bitdefender-cli integrations delete`  -  Delete a specific integration that was configured under the Integrations hub from a target company.
- `bitdefender-cli integrations get`  -  Request information about an integration configured under the Integrations hub.
- `bitdefender-cli integrations list`  -  Allows you to get a list of all the integrations configured on a specific company under the Integrations hub.
- `bitdefender-cli integrations manage`  -  Enable or disable an integration configured under the Integrations hub.
- `bitdefender-cli integrations update`  -  Update an integration configured under the Integrations hub.

**integrators**  -  Integrator accounts linked to the company

- `bitdefender-cli integrators add`  -  Assign an endpoint as an integrator for a specific integration.
- `bitdefender-cli integrators list`  -  Retrieve a list of integrators configured for a specific integration.
- `bitdefender-cli integrators remove`  -  Unassign a specific endpoint as an integrator.

**inventory**  -  Network inventory tree: companies, groups and endpoints

- `bitdefender-cli inventory list`  -  Returns network inventory items.
- `bitdefender-cli inventory roots`  -  Returns the root containers for a company.

**investigation**  -  Investigation packages, file retrieval and remote command execution

- `bitdefender-cli investigation collect-package`  -  Initiate a forensic collection task on an endpoint.
- `bitdefender-cli investigation file-url`  -  Check the status of a task started through the collectInvestigationPackage method.
- `bitdefender-cli investigation kill-process`  -  Initiate a task that terminates a running process on a managed endpoint.
- `bitdefender-cli investigation retrieve-file`  -  Retrieve a file from an endpoint within your company and upload it as an archive to a centralized storage location in
- `bitdefender-cli investigation run-command`  -  Remotely execute a command on an endpoint and upload the resulting output as a log file to the GravityZone centralized

**isolation**  -  Network isolation of endpoints and restore from isolation

- `bitdefender-cli isolation isolate`  -  Creates one or more tasks to isolate the specified endpoint or endpoints.
- `bitdefender-cli isolation restore`  -  Creates one or more tasks to restore the specified endpoint or endpoints from isolation.

**license**  -  License and subscription details for a company

- `bitdefender-cli license add-product-key`  -  Adds a license or add-on key to the existing list of licenses and add-on keys.
- `bitdefender-cli license get`  -  Retrieves the license information for a company.
- `bitdefender-cli license remove-product-key`  -  Removes a license key from the given company.
- `bitdefender-cli license set-key`  -  Sets a standalone or an add-on license key for a company.
- `bitdefender-cli license set-monthly-subscription`  -  Enables monthly subscription for a company.

**live_search**  -  Live Search osquery-style queries on endpoints

- `bitdefender-cli live-search`  -  Create tasks that retrieve information about events and system statistics directly from online endpoints.

**maintenance_windows**  -  Patch management maintenance windows

- `bitdefender-cli maintenance-windows approved-patches`  -  Returns a list of manually approved patches from the endpoints of a specific company.
- `bitdefender-cli maintenance-windows assign`  -  Assigns a set of maintenance windows to a policy.
- `bitdefender-cli maintenance-windows create`  -  Creates a maintenance window which you can use in Patch Management.
- `bitdefender-cli maintenance-windows delete`  -  Deletes a maintenance window.
- `bitdefender-cli maintenance-windows get`  -  Retrieves all the information related to a maintenance window.
- `bitdefender-cli maintenance-windows list`  -  Will display a list of available Maintenance Windows for a company.
- `bitdefender-cli maintenance-windows unassign`  -  Unassigns a set of maintenance windows from a policy.
- `bitdefender-cli maintenance-windows update`  -  Modifies a patch management maintenance window.

**notifications**  -  Notification settings for the API key's user account

- `bitdefender-cli notifications configure`  -  Configures the notification settings for a given user account.
- `bitdefender-cli notifications get`  -  Returns the notifications settings.

**packages**  -  Security agent installation packages and download links

- `bitdefender-cli packages create`  -  Creates an installation package.
- `bitdefender-cli packages delete`  -  Deletes a package identified through the provided package ID.
- `bitdefender-cli packages get`  -  Retrieves information about the configuration of a specific package identified through the provided ID.
- `bitdefender-cli packages install-links`  -  Returns the installation links and full kits for an installation package.
- `bitdefender-cli packages list`  -  Returns the list of available packages.
- `bitdefender-cli packages update`  -  Update an installation package.

**patches**  -  Installed and missing patches per endpoint or company (Patch Management add-on)

- `bitdefender-cli patches installed`  -  Request a list of installed patches that are associated to specific endpoints or a specific company.
- `bitdefender-cli patches missing`  -  Request a list of missing patches that are associated to specific endpoints or a specific company.

**phasr**  -  PHASR attack-surface recommendations, resources and identities

- `bitdefender-cli phasr apply`  -  Applies recommendations by ID and returns success and failure details for each one requested.
- `bitdefender-cli phasr edit-rules-access`  -  Restrict or allow access to behavioral profiles based on the recommendation generated by a specific PHASR rule.
- `bitdefender-cli phasr identities`  -  Retrieve all detected identities for a company.
- `bitdefender-cli phasr monitored-rules`  -  Displays all the monitored PHASR rules across all behavioral categories for a specific company.
- `bitdefender-cli phasr profiles`  -  Retrieve information for the behavioral profiles that a specific recommendation applies to.
- `bitdefender-cli phasr recommendations`  -  Retrieve PHASR recommendations for a company.
- `bitdefender-cli phasr request-access-action`  -  Takes action on request access recommendations by either allowing or denying them.
- `bitdefender-cli phasr resources`  -  Retrieve all detected resources for a company.
- `bitdefender-cli phasr rule-data`  -  Retrieved detailed information regarding a specific PHASR rule and the behavioral profiles it applies to.

**policies**  -  Security policies

- `bitdefender-cli policies get`  -  Retrieves all information related to a security policy.
- `bitdefender-cli policies list`  -  Retrieves the list of available policies.
- `bitdefender-cli policies set-modules`  -  Enable or disable the settings associated to a specific policy.

**push**  -  Event Push Service settings and statistics

- `bitdefender-cli push get`  -  Gets the push event settings.
- `bitdefender-cli push reset-stats`  -  Resets the push event statistics and errors.
- `bitdefender-cli push set`  -  Sets the push event settings.
- `bitdefender-cli push stats`  -  Gets the push event statistics and errors.
- `bitdefender-cli push test`  -  Sends a test event.

**quarantine**  -  Quarantined files and Exchange items

- `bitdefender-cli quarantine add-file`  -  Creates a new task to add a file to quarantine.
- `bitdefender-cli quarantine empty`  -  Creates a new task to empty the quarantine.
- `bitdefender-cli quarantine list`  -  Retrieves the list of quarantined items available for a company.
- `bitdefender-cli quarantine release-exchange`  -  Creates a new task to release items from the quarantine to their intended recipients.
- `bitdefender-cli quarantine remove`  -  Creates a new task to remove items from quarantine.
- `bitdefender-cli quarantine restore`  -  Creates a new task to restore items from the quarantine.
- `bitdefender-cli quarantine restore-exchange`  -  Creates a new task to restore items from the quarantine for Exchange Servers.

**reports**  -  Scheduled reports and their download links

- `bitdefender-cli reports create`  -  Creates a new instant or scheduled report, based on the parameters received
- `bitdefender-cli reports delete`  -  Deletes a report by its ID.
- `bitdefender-cli reports download-links`  -  Returns an Object with information regarding the report availability for download and the corresponding download links.
- `bitdefender-cli reports list`  -  Returns the list of scheduled reports, according to the parameters received.

**response_actions**  -  Incident response actions and their status

- `bitdefender-cli response-actions create`  -  Take response actions on user nodes generated in GravityZone XDR incidents or your own SOC-generated incidents.
- `bitdefender-cli response-actions status`  -  Request the status and the result of a response action.

**tasks**  -  Scan, reconfigure, gather-logs and sandbox tasks

- `bitdefender-cli tasks delete`  -  Delete a specific task, based on its ID.
- `bitdefender-cli tasks gather-logs`  -  Create a task that collects diagnostic logs from an online endpoint.
- `bitdefender-cli tasks list-scans`  -  Returns the list of scan tasks.
- `bitdefender-cli tasks reconfigure`  -  Creates a new Reconfigure Agent task. With this task you can choose which modules to install on target agents.
- `bitdefender-cli tasks sandbox-submit`  -  Create a Sandbox Analyzer task and submit up to 5 files for analysis.
- `bitdefender-cli tasks scan`  -  Creates a new scan task.
- `bitdefender-cli tasks scan-by-mac`  -  Generate a scan task for managed endpoints identified by their MAC address.
- `bitdefender-cli tasks status`  -  Retrieves information about the status of a given task identified using its ID.

**usage**  -  Monthly license usage (billing seats) per company

- `bitdefender-cli usage monthly`  -  Exposes the monthly usage for a company in a target month.
- `bitdefender-cli usage monthly-by-product`  -  Exposes the monthly usage of a company in a target month.

**yara**  -  YARA scans on endpoints

- `bitdefender-cli yara`  -  Initiate an On-demand scan on the specified targets, using a YARA rule to detect files that match the rule conditions.


**companies**


- `bitdefender-cli companies tree`  -  Your own company plus every child company and sub-partner under it as one flat list with ids, type, parent and depth.


**fleet**


- `bitdefender-cli fleet health`  -  One ranked table of every child company's unmanaged, outdated, unscanned, policy-not-applied and isolated endpoints, saved as a dated snapshot.

- `bitdefender-cli fleet incident-queue`  -  Open EDR/XDR incidents across all child companies, ranked by severity and age with unassigned and SLA-breach flags.

- `bitdefender-cli fleet rmm-gap`  -  Joins an RMM device export to GravityZone inventory to list devices with no agent, discovered-but-unmanaged machines, and agents the RMM does not know.

- `bitdefender-cli fleet changes`  -  What changed between saved fleet health snapshots (needs at least two): new and removed endpoints, newly unmanaged, outdated or isolated machines, policy flips.

- `bitdefender-cli fleet threats`  -  The same quarantined file (SHA256) seen at two or more companies in a window, and which of those companies already blocklist it.

- `bitdefender-cli fleet locate`  -  Find a machine by hostname, IP, MAC or label across every child company and get its endpoint id and company.


**usage**


- `bitdefender-cli usage billing`  -  Per-company billable usage for a month with own share after the child rollup, month-over-month seat swings, and flags wherever a needed fetch failed.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
bitdefender-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

## Auth Setup

GravityZone authenticates with HTTP Basic: the API key is the username and the password is empty. Create the key in Control Center under your user menu > My Account > API keys and enable the API areas you will use: keys are scoped per area, with no read/write split. The fleet commands need a partner-level key with the Companies and Network APIs (companies tree, fleet health, fleet locate, fleet rmm-gap), plus Licensing for usage billing, Incidents for fleet incident-queue, and Quarantine and Incidents for fleet threats. A customer-level key works but every fleet view then covers only that one company. Export the key as BITDEFENDER_API_KEY (`export BITDEFENDER_API_KEY=<key>` in bash or zsh; `$env:BITDEFENDER_API_KEY = "<key>"` in PowerShell), or pipe it to `bitdefender-cli auth set-token` to save it. The API host differs by region and on-premises install, so also set BITDEFENDER_BASE_URL to the Access URL shown under My Account > Control Center API (for example https://cloud.gravityzone.bitdefender.com/api).

Run `bitdefender-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  bitdefender-cli accounts list --agent --select email,id,profile
  ```
- **Previewable**  -  `--dry-run` prints the JSON-RPC method (after the `#` in the URL) and the parameters it would send, without sending; the wire body wraps them in a JSON-RPC envelope
- **Offline-friendly**  -  the fleet commands save dated snapshots to a local SQLite store; `fleet changes`, `fleet locate` and `fleet rmm-gap` can answer from them offline
- **Non-interactive**  -  never prompts, every input is a flag
- **Writes run immediately**  -  generated write commands (create, delete, isolate, quarantine, tasks) run as soon as they are invoked and do not prompt; preview with `--dry-run` first. `--agent` does not imply `--yes`
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success

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

- Use `--home <dir>` for one invocation, or set `BITDEFENDER_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `BITDEFENDER_CONFIG_DIR`, `BITDEFENDER_DATA_DIR`, `BITDEFENDER_STATE_DIR`, `BITDEFENDER_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `BITDEFENDER_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `bitdefender-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

  On Windows, use a Windows path with doubled backslashes in JSON, for example `"BITDEFENDER_HOME": "C:\\msp\\bitdefender"`.

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `BITDEFENDER_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `BITDEFENDER_HOME`, or `doctor` will not find credentials left under the former root.

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
bitdefender-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "bitdefender-cli learnings confirm 12"] }
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
       materially more, record the divergence via `bitdefender-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `bitdefender-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
bitdefender-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
bitdefender-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
bitdefender-cli teach-playbook \
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
bitdefender-cli playbook amend \
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

`bitdefender-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `BITDEFENDER_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
bitdefender-cli feedback "the --since flag is inclusive but docs say exclusive"
bitdefender-cli feedback --stdin < notes.txt
bitdefender-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `BITDEFENDER_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `BITDEFENDER_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
bitdefender-cli profile save briefing --json
bitdefender-cli --profile briefing accounts list
bitdefender-cli profile list --json
bitdefender-cli profile show briefing
bitdefender-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | `fleet health --fail-on` found companies with the listed issues |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `bitdefender-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add bitdefender-mcp -- bitdefender-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which bitdefender-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   bitdefender-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `bitdefender-cli <command> --help`.
