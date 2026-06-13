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
      - kind: go
        bins: [ninjaone-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/ninjaone/cmd/ninjaone-cli
---

# NinjaOne  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `ninjaone-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install ninjaone --cli-only
   ```
2. Verify: `ninjaone-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/ninjaone/cmd/ninjaone-cli@latest
```

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

These capabilities aren't available in any other tool for this API.

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
- **`software-audit`**  -  For each software title, counts the distinct versions installed across the fleet and where each version lives; flags titles fragmented across many versions.

  _Find license sprawl and update lag (the same app at 14 versions) in one query instead of scrolling per-device inventories._

  ```bash
  ninjaone-cli software-audit --min-versions 3 --json
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


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
ninjaone-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  ninjaone-cli activities --agent --select id,name,status
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
ninjaone-cli feedback "the --since flag is inclusive but docs say exclusive"
ninjaone-cli feedback --stdin < notes.txt
ninjaone-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/ninjaone-cli/feedback.jsonl`. They are never POSTed unless `NINJAONE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `NINJAONE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

1. **Empty, `help`, or `--help`** → show `ninjaone-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/ninjaone/cmd/ninjaone-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add ninjaone-mcp -- ninjaone-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which ninjaone-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   ninjaone-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `ninjaone-cli <command> --help`.
