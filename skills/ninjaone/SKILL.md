---
name: ninjaone
description: "Every NinjaOne report, plus a local store that answers fleet-wide questions no single API call can: patch compliance, backup gaps, AV blast-radius, health, drift. Trigger phrases: `check patch compliance in ninjaone`, `which ninjaone devices have no backup`, `ninjaone av threat sweep`, `ninjaone fleet health for an org`, `show stale ninjaone devices`, `use ninjaone`, `run ninjaone`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "NinjaOne"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - ninjaone-cli
    install:
      - kind: script
        bins: [ninjaone-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.ps1
---

# NinjaOne - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `ninjaone-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.ps1 | iex
   ```
3. Verify: `ninjaone-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `ninjaone-cli` and `ninjaone-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Existing NinjaOne tools are 1:1 API mirrors or Python libraries you script yourself. This CLI syncs your whole estate into local SQLite, then answers the questions MSPs actually ask across clients with offline FTS, analytics rollups, --json/--select/--csv, and typed exit codes. Commands like patch-compliance, backup-coverage, av-sweep, fleet-health, and drift are cross-fleet joins the API never returns in one call, and every command is agent-native through the MCP Cobra-tree mirror.

## When to Use This CLI

Use this CLI when an agent or technician needs fleet-wide answers across many NinjaOne organizations  -  patch compliance, unprotected-device detection, AV threat blast-radius, OS end-of-life exposure, software sprawl, or week-over-week drift  -  rather than a single device or report lookup. It is also the right tool for scripted automation: every command speaks --json/--select with typed exit codes, and the local store keeps queries fast and offline.

## Anti-triggers

Do not use this CLI for:
- Inspecting a single device's backup bytes or job history  -  use the generated backup / queries backup-usage commands, not backup-coverage
- Trend-over-time questions  -  use drift, not fleet-health; point-in-time scoring  -  use fleet-health, not drift
- Real-time alert paging, remote control sessions, or console-only workflows  -  this CLI covers the NinjaOne public API surface only
- Other RMM platforms (Datto RMM, N-able N-central, Atera)  -  this CLI is NinjaOne-only

## Unique Capabilities

### Cross-fleet rollups (local store only)
- **`patch-compliance`**  -  One row per organization showing percent OS-patched, percent software-patched, failed counts, and the worst-offender device across your whole fleet.

  _Reach for this when an agent or tech needs the single 'who is non-compliant' answer across many clients instead of pivoting two reports by hand._

  ```bash
  ninjaone-cli patch-compliance --min-pct 95 --agent
  ```
- **`backup-coverage`**  -  Lists every device with no backup usage (or zero bytes / stale job), grouped by organization, so unprotected endpoints surface immediately.

  _The fastest DR-oversight answer: which managed endpoints are silently unprotected right now._

  ```bash
  ninjaone-cli backup-coverage --agent --select deviceName,org,reason
  ```
- **`software-audit`**  -  For each software title, reports its install count across the fleet, the number of distinct versions and the version list; flags titles fragmented across many versions.

  _Find license sprawl and update lag (the same app at 14 versions) in one query instead of scrolling per-device inventories._

  ```bash
  ninjaone-cli software-audit --min-versions 3 --json
  ```
- **`low-disk`**  -  Lists every synced volume under a free-space floor (percent or GiB), worst first, with device and organization, so disk-full patch and backup failures are caught before they happen.

  _Full disks are a top cause of failed patches and backups; this is the weekly sweep that finds them across every client at once._

  ```bash
  ninjaone-cli low-disk --min-free-pct 10 --agent
  ```
- **`bitlocker-gaps`**  -  Lists every synced volume whose BitLocker protection is not PROTECTED (unprotected, pending, unknown or not reported), grouped by organization, for encryption audits and cyber-insurance questionnaires.

  _Insurers and auditors ask whether every laptop is encrypted; this answers it per client from one sync._

  ```bash
  ninjaone-cli bitlocker-gaps --org 2 --agent
  ```

### Risk and exposure scoring
- **`fleet-health`**  -  A transparent 0-100 score per organization from patch, backup, AV, and stale-device signals, with the contributing deductions itemized.

  _One artifact for a quarterly business review: an agent can read the itemized deductions and explain why a client is trending down._

  ```bash
  ninjaone-cli fleet-health --org 42 --agent
  ```
- **`av-sweep`**  -  Given a threat name or a stale-definition window, lists every device fleet-wide carrying it, grouped by organization and location.

  _Turns one threat detection into a blast-radius answer during triage without walking the API device by device._

  ```bash
  ninjaone-cli av-sweep --definition-stale-days 7 --agent
  ```
- **`stale-devices`**  -  Lists devices whose last contact exceeds a threshold, with days-since and owning organization and location.

  _Surfaces endpoints that quietly stopped checking in, which are invisible in per-org device lists._

  ```bash
  ninjaone-cli stale-devices --days 14 --csv
  ```
- **`os-eol`**  -  Lists devices running end-of-life operating systems, grouped by organization, against a curated EOL reference table.

  _A security-and-compliance answer for QBRs: which clients are running OS versions past their support date._

  ```bash
  ninjaone-cli os-eol --agent
  ```

### Time-series (local snapshots)
- **`drift`**  -  Diffs the current sync against the previous stored snapshot to show which organizations got better or worse on patch, backup, or stale metrics.

  _The only way to answer 'which clients are drifting' without rebuilding a spreadsheet from scratch each week._

  ```bash
  ninjaone-cli drift --metric patch --agent
  ```

## Command Reference

**activities**  -  Manage activities

- `ninjaone-cli activities`  -  Returns activity log in reverse chronological order

**alert**  -  Manage alert

- `ninjaone-cli alert <uid>`  -  Resets alert/condition by UID

**alerts**  -  Manage alerts

- `ninjaone-cli alerts`  -  Returns list of active alerts/triggered conditions

**attachments**  -  Manage attachments

- `ninjaone-cli attachments`  -  Upload temporary attachments

**automation**  -  Manage automation

- `ninjaone-cli automation`  -  Returns list of all available automation scripts

**backup**  -  Backup

- `ninjaone-cli backup get-integrity-check-jobs`  -  Returns a list of integrity check jobs.
- `ninjaone-cli backup get-jobs`  -  Returns list of backup jobs
- `ninjaone-cli backup submit-integrity-check-job`  -  Creates an integrity check job

**checklist**  -  Manage checklist

- `ninjaone-cli checklist archive-template`  -  Archive a checklist template by id
- `ninjaone-cli checklist create-templates`  -  Creates multiple checklist templates
- `ninjaone-cli checklist delete-template`  -  Delete a checklist template by id
- `ninjaone-cli checklist delete-templates`  -  Deletes checklist templates by id
- `ninjaone-cli checklist get-templates`  -  List checklists templates with given criteria
- `ninjaone-cli checklist restore-template`  -  Restore a checklist template by id
- `ninjaone-cli checklist update-templates`  -  Updates multiple checklist templates

**contact**  -  Manage contact

- `ninjaone-cli contact delete`  -  Delete a contact by their ID
- `ninjaone-cli contact get-by-id`  -  Get a contact by their ID
- `ninjaone-cli contact update`  -  Update a contact by their ID

**contacts**  -  Manage contacts

- `ninjaone-cli contacts create`  -  Create a new contact
- `ninjaone-cli contacts get`  -  Get all contacts

**custom-fields**  -  Custom Fields

- `ninjaone-cli custom-fields <entityId>`  -  Get custom field signed urls

**device**  -  Devices

- `ninjaone-cli device get`  -  Returns device details
- `ninjaone-cli device update`  -  Change device friendly name, user data, etc.

**device-custom-fields**  -  Manage device custom fields

- `ninjaone-cli device-custom-fields`  -  Returns list of all custom fields

**devices**  -  Devices

- `ninjaone-cli devices get`  -  Returns list of devices (basic node information)
- `ninjaone-cli devices node-approval-operation`  -  Approve or reject devices that are waiting for approval
- `ninjaone-cli devices search`  -  Returns list of entities matching search term

**devices-detailed**  -  Manage devices detailed

- `ninjaone-cli devices-detailed`  -  Returns list of devices with additional information

**document-templates**  -  Document Templates

- `ninjaone-cli document-templates archive`  -  Archives multiple document template by ids
- `ninjaone-cli document-templates create`  -  Create document template
- `ninjaone-cli document-templates delete`  -  Deletes a document template by id
- `ninjaone-cli document-templates get`  -  Get document template
- `ninjaone-cli document-templates get-with-attributes`  -  List document templates with fields
- `ninjaone-cli document-templates restore`  -  Restores a document template by id
- `ninjaone-cli document-templates update`  -  Updates a document template by id

**group**  -  Groups/Search


**groups**  -  Groups/Search

- `ninjaone-cli groups`  -  List groups (saved searches)

**itam**  -  Manage itam

- `ninjaone-cli itam create-unmanaged-device-public-api`  -  Create an Unmanaged Device with the provided details
- `ninjaone-cli itam delete-unmanaged-device-public-api`  -  Delete an Unmanaged Device with the provided id
- `ninjaone-cli itam update-unmanaged-device-public-api`  -  Update an Unmanaged Device with the provided details

**knowledgebase**  -  Manage knowledgebase

- `ninjaone-cli knowledgebase archive-knowledge-base-articles`  -  Archive knowledge base articles
- `ninjaone-cli knowledgebase archive-knowledge-base-folders`  -  Archive knowledge base folders
- `ninjaone-cli knowledgebase create-knowledge-base-articles`  -  Create knowledge base articles
- `ninjaone-cli knowledgebase delete-knowledge-base-articles`  -  Delete knowledge base articles
- `ninjaone-cli knowledgebase delete-knowledge-base-folders`  -  Delete knowledge base folders
- `ninjaone-cli knowledgebase download-knowledge-base-article`  -  Download knowledge base article
- `ninjaone-cli knowledgebase get-client-knowledge-base-articles`  -  Lists organization knowledge base articles
- `ninjaone-cli knowledgebase get-global-knowledge-base-articles`  -  Lists global knowledge base articles
- `ninjaone-cli knowledgebase get-knowledge-base-article-signed-urls`  -  Get knowledge base article signed urls
- `ninjaone-cli knowledgebase get-knowledge-base-folder-content`  -  Returns knowledge base folder and its content
- `ninjaone-cli knowledgebase get-knowledge-base-folder-path-content`  -  Returns knowledge base folder and its content
- `ninjaone-cli knowledgebase move`  -  Move knowledge base folders and documents to another knowledge base folder
- `ninjaone-cli knowledgebase restore-knowledge-base-articles`  -  Restore archived knowledge base articles
- `ninjaone-cli knowledgebase restore-knowledge-base-folders`  -  Restore archived knowledge base folders
- `ninjaone-cli knowledgebase update-knowledge-base-articles`  -  Update knowledge base articles
- `ninjaone-cli knowledgebase upload-knowledge-base-articles`  -  Upload knowledge base articles

**locations**  -  Location

- `ninjaone-cli locations`  -  Returns flat list of all locations for all organizations

**ninjaone-public-jobs**  -  Manage ninjaone public jobs

- `ninjaone-cli ninjaone-public-jobs`  -  Returns list of running jobs

**notification-channels**  -  Manage notification channels

- `ninjaone-cli notification-channels get`  -  Returns list of notification channels
- `ninjaone-cli notification-channels get-enabled`  -  Returns list of enabled notification channels

**organization**  -  Organizations

- `ninjaone-cli organization archive-checklists`  -  Archive multiple organization checklists
- `ninjaone-cli organization archive-client-document`  -  Archives an organization document by id
- `ninjaone-cli organization archive-multi-page-client-documents`  -  Archives multiple organization documents by id
- `ninjaone-cli organization create-checklists`  -  Creates multiple organization checklists
- `ninjaone-cli organization create-documents`  -  Creates organization documents and returns the documents created
- `ninjaone-cli organization delete-client-checklist`  -  Deletes an organization checklist by id
- `ninjaone-cli organization delete-client-checklists`  -  Deletes organization checklists by id
- `ninjaone-cli organization delete-client-document`  -  Deletes an archived organization document by id
- `ninjaone-cli organization get`  -  Returns organization details (policy mappings, locations)
- `ninjaone-cli organization get-client-checklist`  -  Get a client checklist by id
- `ninjaone-cli organization get-client-checklist-signed-urls`  -  Get organization checklist signed urls
- `ninjaone-cli organization get-client-checklists`  -  List client checklists with given criteria
- `ninjaone-cli organization get-client-document-signed-urls`  -  Get organization document signed urls
- `ninjaone-cli organization get-client-documents-with-attribute-values`  -  List all organization documents with field values
- `ninjaone-cli organization get-installer`  -  Generates and returns URL for installer with specified settings
- `ninjaone-cli organization promote-client-checklists`  -  Promote organization checklists by id
- `ninjaone-cli organization promote-client-checklists-1`  -  Promote organization checklists by id
- `ninjaone-cli organization restore-checklists`  -  Restore multiple organization checklists
- `ninjaone-cli organization restore-client-document`  -  Restores an organization document by id
- `ninjaone-cli organization restore-multi-page-client-documents`  -  Restore multiple multi page organization documents
- `ninjaone-cli organization update`  -  Change organization name, description and policy mappings
- `ninjaone-cli organization update-checklists`  -  Updates multiple organization checklists
- `ninjaone-cli organization update-documents`  -  Updates organization documents and returns the documents updated

**organizations**  -  Organizations

- `ninjaone-cli organizations create`  -  Creates new organization with optional list of locations and policy mappings.
- `ninjaone-cli organizations get`  -  Returns list of organizations (Brief mode)

**organizations-detailed**  -  Manage organizations detailed

- `ninjaone-cli organizations-detailed`  -  Returns list of organizations with locations and policy mappings

**policies**  -  Manage policies

- `ninjaone-cli policies create-policy`  -  Creates new policy using (New Root, Child, Copy)
- `ninjaone-cli policies get`  -  Returns list of policies

**queries**  -  Queries

- `ninjaone-cli queries get-antivirus-status-report`  -  Returns list of statues of antivirus software installed on devices
- `ninjaone-cli queries get-antivirus-threats`  -  Returns list of antivirus threats
- `ninjaone-cli queries get-computer-systems`  -  Returns computer systems information for devices
- `ninjaone-cli queries get-custom-fields-detailed-report`  -  Returns Custom Fields report with additional information about each field
- `ninjaone-cli queries get-custom-fields-report`  -  Returns Custom Fields report
- `ninjaone-cli queries get-device-health-report`  -  Returns list of device health summary records
- `ninjaone-cli queries get-device-usage`  -  Returns the backup usage by device
- `ninjaone-cli queries get-disk-drives`  -  Returns list of physical disks
- `ninjaone-cli queries get-installed-ospatches`  -  Returns patch installation history records (successful and failed)
- `ninjaone-cli queries get-installed-software-patches`  -  Returns 3rd party software patch installation history records (successful and failed)
- `ninjaone-cli queries get-last-logged-on-users-report`  -  Returns usernames and logon times
- `ninjaone-cli queries get-network-interfaces`  -  Returns list of Network Interfaces for each device
- `ninjaone-cli queries get-operating-systems`  -  Returns operating systems' for devices
- `ninjaone-cli queries get-pending-failed-rejected-ospatches`  -  Returns list of OS patches for which there were no installation attempts
- `ninjaone-cli queries get-pending-failed-rejected-software-patches`  -  Returns list of 3rd party Software patches for which there were no installation attempts
- `ninjaone-cli queries get-policy-overrides-1`  -  Returns list of overridden policy sections for each device
- `ninjaone-cli queries get-processors`  -  Returns list of processors
- `ninjaone-cli queries get-raidcontroller-report`  -  Returns list of RAID controllers
- `ninjaone-cli queries get-raiddrive-report`  -  Returns list of drives connected to RAID controllers
- `ninjaone-cli queries get-scoped-custom-fields-detailed-report`  -  Returns report for Custom Fields defined at different scopes (device, location, organization)
- `ninjaone-cli queries get-scoped-custom-fields-report`  -  Returns report for Custom Fields defined at different scopes (device, location, organization)
- `ninjaone-cli queries get-software`  -  Returns list software installed on devices
- `ninjaone-cli queries get-volumes`  -  Returns list of disk volumes
- `ninjaone-cli queries get-windows-services-report`  -  Returns list of Windows Services and their statuses

**related-items**  -  Related Items

- `ninjaone-cli related-items create`  -  Relate an attachment to an entity
- `ninjaone-cli related-items create-for-entity`  -  Create a relation between two entities
- `ninjaone-cli related-items create-for-entity-1`  -  Create multiple relations between two entities
- `ninjaone-cli related-items create-secure-for-entity`  -  Create a relation to a secure value
- `ninjaone-cli related-items delete`  -  Deletes related item
- `ninjaone-cli related-items delete-relateditems`  -  Deletes related items associated with an entity
- `ninjaone-cli related-items get-all`  -  List all related items
- `ninjaone-cli related-items get-attachments-signed-urls`  -  Get related item attachments signed urls for an entity
- `ninjaone-cli related-items get-for-host-entity`  -  List related items for a specific host entity filterable by scope
- `ninjaone-cli related-items get-with-entity`  -  List related items for a specific related entity
- `ninjaone-cli related-items get-with-entity-type`  -  List related entities for a related entity type
- `ninjaone-cli related-items get-with-host-entity-type`  -  List relations and references for a host entity type

**roles**  -  Manage roles

- `ninjaone-cli roles`  -  Returns list of device roles

**software-products**  -  Manage software products

- `ninjaone-cli software-products`  -  Returns available software products (3rd party patching)

**tab**  -  Manage tab

- `ninjaone-cli tab create-custom-public-api`  -  Create a Custom Tab with the provided details
- `ninjaone-cli tab delete-unmanaged-device-public-api-1`  -  Delete a Custom Tab
- `ninjaone-cli tab get-custom-public-api`  -  Gets a custom tab. NOTE: This will _not_ fetch tab extensions. You must use the GET tab/{tabId}/role/{roleId} for that
- `ninjaone-cli tab get-summary-for-end-user`  -  Retrieve all of the custom tabs available to end user views
- `ninjaone-cli tab get-summary-for-organization`  -  Retrieve all of the custom tabs available to organizations and locations
- `ninjaone-cli tab get-summary-for-role`  -  Retrieve all of the custom tabs that would appear for the given role
- `ninjaone-cli tab rename-custom-public-api`  -  Renames a Custom Tab
- `ninjaone-cli tab update-custom-display`  -  Using this API it is possible to configure tabs to be hidden for roles and their children
- `ninjaone-cli tab update-custom-public-api`  -  Update a Custom Tab.
- `ninjaone-cli tab update-end-user-custom-order`  -  Update the order of custom tabs for end-user tabs. NOTE: All tabs defined for end-users must be specified in the payload
- `ninjaone-cli tab update-organization-custom-order`  -  Update the order of custom tabs for organizations and locations.
- `ninjaone-cli tab update-role-custom-order`  -  Update the order of custom tabs for a specific role. NOTE: Only tabs created on this role can be ordered.

**tag**  -  Manage tag

- `ninjaone-cli tag batch-update`  -  Update tags for the supplied assetIds. Tags will be added and removed as specified
- `ninjaone-cli tag create`  -  Create an Asset Tag with the provided name and description
- `ninjaone-cli tag delete`  -  Delete Asset Tags having the provided ids
- `ninjaone-cli tag delete-tagid`  -  Delete the Asset Tag with the provided id
- `ninjaone-cli tag get`  -  Get a list of created Asset Tags
- `ninjaone-cli tag merge`  -  Merges tags. Can merge into an existing or new tag depending on the input parameters
- `ninjaone-cli tag set-for-asset`  -  Set the tags for an asset to exactly the supplied values
- `ninjaone-cli tag update`  -  Update an Asset Tag with the provided metadata

**tasks**  -  Manage tasks

- `ninjaone-cli tasks`  -  Returns list of registered scheduled tasks

**ticketing**  -  ticketing

- `ninjaone-cli ticketing create`  -  Create a new ticket, does not accept files
- `ninjaone-cli ticketing create-comment`  -  Add a new comment to a ticket, allows files
- `ninjaone-cli ticketing get-all-statuses`  -  Get list of ticket status
- `ninjaone-cli ticketing get-all-user-and-contacts`  -  Returns list of users (contacts, end-user, technician)
- `ninjaone-cli ticketing get-boards`  -  Returns list of ticketing boards
- `ninjaone-cli ticketing get-contacts-1`  -  Returns list of contacts
- `ninjaone-cli ticketing get-ticket-attributes`  -  Returns list of the ticket attributes
- `ninjaone-cli ticketing get-ticket-by-id`  -  Returns a ticket
- `ninjaone-cli ticketing get-ticket-form-by-id`  -  Returns a ticket form with fields
- `ninjaone-cli ticketing get-ticket-forms`  -  Returns list of ticket forms with their fields
- `ninjaone-cli ticketing get-ticket-log-entries-by-ticket-id`  -  Returns list of the ticket log entries for a ticket
- `ninjaone-cli ticketing get-tickets-by-board`  -  Run a board. Returns list of tickets matching the board condition and filters. Allows pagination
- `ninjaone-cli ticketing update`  -  Change ticket fields. Does not accept comments

**user**  -  Users

- `ninjaone-cli user add-role-members`  -  Add members to user role
- `ninjaone-cli user create-end`  -  Create an end user
- `ninjaone-cli user create-technician`  -  Create a new technician
- `ninjaone-cli user delete-end`  -  Delete an end user
- `ninjaone-cli user delete-technician`  -  Delete a technician by their ID
- `ninjaone-cli user get-end`  -  Get details for a specific end user identifier
- `ninjaone-cli user get-end-1`  -  Get all end users
- `ninjaone-cli user get-node-custom-fields-3`  -  Returns list of end user custom fields
- `ninjaone-cli user get-roles`  -  Get list of user roles
- `ninjaone-cli user get-technician`  -  Get details for a specific technician identifier
- `ninjaone-cli user get-technicians`  -  Get all technicians
- `ninjaone-cli user patch-end`  -  Update a specific end user by their ID
- `ninjaone-cli user remove-role-members`  -  Remove users from user role
- `ninjaone-cli user update-node-attribute-values-3`  -  Update end user custom field values
- `ninjaone-cli user update-technician`  -  Update technician by their ID

**users**  -  Users

- `ninjaone-cli users`  -  Returns list of users

**vulnerability**  -  Manage vulnerability

- `ninjaone-cli vulnerability fetch-all-scan-groups`  -  Fetches all Scan Groups.
- `ninjaone-cli vulnerability fetch-scan-group-by-id`  -  Fetches a single Scan Group by ID.
- `ninjaone-cli vulnerability update-scan-group`  -  Upload CSV to an existing scan group.

**webhook**  -  Webhook Endpoints

- `ninjaone-cli webhook configure`  -  Creates or updates Webhook configuration for current application/client
- `ninjaone-cli webhook disable`  -  Disables Webhook configuration for current application/client


**av-sweep**


- `ninjaone-cli av-sweep`  -  Given a threat name or a stale-definition window, lists every device fleet-wide carrying it, grouped by organization and location.


**backup-coverage**


- `ninjaone-cli backup-coverage`  -  Lists every device with no backup usage (or zero bytes / stale job), grouped by organization, so unprotected endpoints surface immediately.


**bitlocker-gaps**


- `ninjaone-cli bitlocker-gaps`  -  Lists every synced volume whose BitLocker protection is not PROTECTED (unprotected, pending, unknown or not reported), grouped by organization, for encryption audits and cyber-insurance questionnaires.


**drift**


- `ninjaone-cli drift`  -  Diffs the current sync against the previous stored snapshot to show which organizations got better or worse on patch, backup, or stale metrics.


**fleet-health**


- `ninjaone-cli fleet-health`  -  A transparent 0-100 score per organization from patch, backup, AV, and stale-device signals, with the contributing deductions itemized.


**low-disk**


- `ninjaone-cli low-disk`  -  Lists every synced volume under a free-space floor (percent or GiB), worst first, with device and organization, so disk-full patch and backup failures are caught before they happen.


**os-eol**


- `ninjaone-cli os-eol`  -  Lists devices running end-of-life operating systems, grouped by organization, against a curated EOL reference table.


**patch-compliance**


- `ninjaone-cli patch-compliance`  -  One row per organization showing percent OS-patched, percent software-patched, failed counts, and the worst-offender device across your whole fleet.


**software-audit**


- `ninjaone-cli software-audit`  -  For each software title, reports its install count across the fleet, the number of distinct versions and the version list; flags titles fragmented across many versions.


**stale-devices**


- `ninjaone-cli stale-devices`  -  Lists devices whose last contact exceeds a threshold, with days-since and owning organization and location.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
ninjaone-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Monday patch-compliance review

```bash
ninjaone-cli sync && ninjaone-cli patch-compliance --min-pct 95 --agent
```

Sync the fleet, then get a per-org compliance table ready to pipe into a report or an agent.

### Find unprotected endpoints for a DR audit

```bash
ninjaone-cli backup-coverage --agent --select deviceName,org,reason
```

List every device with no/stale backup, narrowing the agent-shaped output to the fields that matter.

### AV threat blast-radius during triage

```bash
ninjaone-cli av-sweep --threat 'Trojan.Generic' --json
```

Find every device fleet-wide carrying a named threat, grouped by org and location.

### QBR health snapshot for one client

```bash
ninjaone-cli fleet-health --org 42 --agent
```

Get an itemized 0-100 health score an agent can narrate in a quarterly review.

### Narrow a verbose report for an agent

```bash
ninjaone-cli queries get-device-health-report --agent --select results.deviceId,results.healthStatus
```

Use --select with dotted paths to keep only the fields you need from a large nested report.

## Auth Setup

NinjaOne uses OAuth2 client-credentials. Create an API app under Administration > Apps > API, then set NINJAONE_CLIENT_ID and NINJAONE_CLIENT_SECRET. The default base URL is https://app.ninjarmm.com; for non-US tenants set NINJAONE_BASE_URL (e.g. https://eu.ninjarmm.com) and NINJAONE_TOKEN_URL (e.g. https://eu.ninjarmm.com/ws/oauth/token). Run 'ninjaone-cli doctor' to confirm auth and reachability.

Run `ninjaone-cli doctor` to verify setup.

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
  ninjaone-cli activities --agent
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

- Use `--home <dir>` for one invocation, or set `NINJAONE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `NINJAONE_CONFIG_DIR`, `NINJAONE_DATA_DIR`, `NINJAONE_STATE_DIR`, `NINJAONE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `NINJAONE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `ninjaone-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "ninjaone": {
        "command": "ninjaone-mcp",
        "env": {
          "NINJAONE_HOME": "/srv/ninjaone"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `NINJAONE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `NINJAONE_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `NINJAONE_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `NINJAONE_DATA_SOURCE` sets the `--data-source` default.

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
ninjaone-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "ninjaone-cli learnings confirm 12"] }
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
       materially more, record the divergence via `ninjaone-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `ninjaone-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `ninjaone-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
ninjaone-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
ninjaone-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
ninjaone-cli teach-playbook \
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
ninjaone-cli playbook amend \
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

`ninjaone-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `NINJAONE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
ninjaone-cli feedback "the --since flag is inclusive but docs say exclusive"
ninjaone-cli feedback --stdin < notes.txt
ninjaone-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `NINJAONE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `NINJAONE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
ninjaone-cli profile save briefing --json
ninjaone-cli --profile briefing activities
ninjaone-cli profile list --json
ninjaone-cli profile show briefing
ninjaone-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Async Jobs

For endpoints that submit long-running work, the generator detects the submit-then-poll pattern (a `job_id`/`task_id`/`operation_id` field in the response plus a sibling status endpoint) and wires up three extra flags on the submitting command:

| Flag | Purpose |
|------|---------|
| `--wait` | Block until the job reaches a terminal status instead of returning the job ID immediately |
| `--wait-timeout` | Maximum wait duration (default 10m, 0 means no timeout) |
| `--wait-interval` | Initial poll interval (default 2s; grows with exponential backoff up to 30s) |

Use async submission without `--wait` when you want to fire-and-forget; use `--wait` when you want one command to return the finished artifact.

If `--wait` times out or polling fails after the job was accepted, the command exits `8` and prints the job ID plus a recovery command that fetches the result. The job may still be running and may already be billed, so run the recovery command instead of resubmitting.

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
| 8 | Async job submitted but not finished (run the printed recovery command; do not resubmit) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `ninjaone-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add ninjaone-mcp -- ninjaone-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `ninjaone-mcp --transport http` requires `NINJAONE_MCP_HTTP_TOKEN` (callers send `Authorization: Bearer <token>`), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. `NINJAONE_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which ninjaone-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   ninjaone-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `ninjaone-cli <command> --help`.
