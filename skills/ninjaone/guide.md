# NinjaOne CLI

**Every NinjaOne report, plus a local store that answers fleet-wide questions no single API call can: patch compliance, backup gaps, AV blast-radius, health, drift.**

Existing NinjaOne tools are 1:1 API mirrors or Python libraries you script yourself. This CLI syncs your whole estate into local SQLite, then answers the questions MSPs actually ask across clients with offline FTS, analytics rollups, --json/--select/--csv, and typed exit codes. Commands like patch-compliance, backup-coverage, av-sweep, fleet-health, and drift are cross-fleet joins the API never returns in one call, and every command is agent-native through the MCP Cobra-tree mirror.

## Install

The recommended path installs both the `ninjaone-cli` binary and the `pp-ninjaone` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install ninjaone
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install ninjaone --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install ninjaone --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install ninjaone --agent claude-code
npx -y @mvanhorn/printing-press-library install ninjaone --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/ninjaone/cmd/ninjaone-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/ninjaone-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install ninjaone --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-ninjaone --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-ninjaone --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install ninjaone --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/ninjaone-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `NINJAONE_CLIENT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/ninjaone/cmd/ninjaone-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "ninjaone": {
      "command": "ninjaone-mcp",
      "env": {
        "NINJAONE_CLIENT_ID": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

NinjaOne uses OAuth2 client-credentials. Create an API app under Administration > Apps > API, then set NINJAONE_CLIENT_ID and NINJAONE_CLIENT_SECRET. The default base URL is https://app.ninjarmm.com; for non-US tenants set NINJAONE_BASE_URL (e.g. https://eu.ninjarmm.com) and NINJAONE_TOKEN_URL (e.g. https://eu.ninjarmm.com/ws/oauth/token). Run 'ninjaone-cli doctor' to confirm auth and reachability.

## Quick Start

```bash
# confirm OAuth credentials, instance/region, and API reachability first
ninjaone-cli doctor

# pull devices, orgs, patches, software, AV, and backup reports into the local store
ninjaone-cli sync

# per-org compliance rollup from the synced data
ninjaone-cli patch-compliance --min-pct 95

# find unprotected devices across the fleet
ninjaone-cli backup-coverage --agent

# offline full-text search across synced devices and software
ninjaone-cli search 'finance-srv'

```

## Unique Features

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

## Usage

Run `ninjaone-cli --help` for the full command reference and flag list.

## Commands

### activities

Manage activities

- **`ninjaone-cli activities`** - Returns activity log in reverse chronological order

### alert

Manage alert

- **`ninjaone-cli alert <uid>`** - Resets alert/condition by UID

### alerts

Manage alerts

- **`ninjaone-cli alerts`** - Returns list of active alerts/triggered conditions

### attachments

Manage attachments

- **`ninjaone-cli attachments`** - Upload temporary attachments

### automation

Manage automation

- **`ninjaone-cli automation`** - Returns list of all available automation scripts

### backup

Backup

- **`ninjaone-cli backup get-integrity-check-jobs`** - Returns a list of integrity check jobs.
- **`ninjaone-cli backup get-jobs`** - Returns list of backup jobs
- **`ninjaone-cli backup submit-integrity-check-job`** - Creates an integrity check job

### checklist

Manage checklist

- **`ninjaone-cli checklist archive-template`** - Archive a checklist template by id
- **`ninjaone-cli checklist create-templates`** - Creates multiple checklist templates
- **`ninjaone-cli checklist delete-template`** - Delete a checklist template by id
- **`ninjaone-cli checklist delete-templates`** - Deletes checklist templates by id
- **`ninjaone-cli checklist get-templates`** - List checklists templates with given criteria
- **`ninjaone-cli checklist restore-template`** - Restore a checklist template by id
- **`ninjaone-cli checklist update-templates`** - Updates multiple checklist templates

### contact

Manage contact

- **`ninjaone-cli contact delete`** - Delete a contact by their ID
- **`ninjaone-cli contact get-by-id`** - Get a contact by their ID
- **`ninjaone-cli contact update`** - Update a contact by their ID

### contacts

Manage contacts

- **`ninjaone-cli contacts create`** - Create a new contact
- **`ninjaone-cli contacts get`** - Get all contacts

### custom-fields

Custom Fields

- **`ninjaone-cli custom-fields <entityId>`** - Get custom field signed urls

### device

Devices

- **`ninjaone-cli device get`** - Returns device details
- **`ninjaone-cli device update`** - Change device friendly name, user data, etc.

### device-custom-fields

Manage device custom fields

- **`ninjaone-cli device-custom-fields`** - Returns list of all custom fields

### devices

Devices

- **`ninjaone-cli devices get`** - Returns list of devices (basic node information)
- **`ninjaone-cli devices node-approval-operation`** - Approve or reject devices that are waiting for approval
- **`ninjaone-cli devices search`** - Returns list of entities matching search term

### devices-detailed

Manage devices detailed

- **`ninjaone-cli devices-detailed`** - Returns list of devices with additional information

### document-templates

Document Templates

- **`ninjaone-cli document-templates archive`** - Archives multiple document template by ids
- **`ninjaone-cli document-templates create`** - Create document template
- **`ninjaone-cli document-templates delete`** - Deletes a document template by id
- **`ninjaone-cli document-templates get`** - Get document template
- **`ninjaone-cli document-templates get-with-attributes`** - List document templates with fields
- **`ninjaone-cli document-templates restore`** - Restores a document template by id
- **`ninjaone-cli document-templates update`** - Updates a document template by id

### group

Groups/Search


### groups

Groups/Search

- **`ninjaone-cli groups`** - List groups (saved searches)

### itam

Manage itam

- **`ninjaone-cli itam create-unmanaged-device-public-api`** - Create an Unmanaged Device with the provided details
- **`ninjaone-cli itam delete-unmanaged-device-public-api`** - Delete an Unmanaged Device with the provided id
- **`ninjaone-cli itam update-unmanaged-device-public-api`** - Update an Unmanaged Device with the provided details

### knowledgebase

Manage knowledgebase

- **`ninjaone-cli knowledgebase archive-knowledge-base-articles`** - Archive knowledge base articles
- **`ninjaone-cli knowledgebase archive-knowledge-base-folders`** - Archive knowledge base folders
- **`ninjaone-cli knowledgebase create-knowledge-base-articles`** - Create knowledge base articles
- **`ninjaone-cli knowledgebase delete-knowledge-base-articles`** - Delete knowledge base articles
- **`ninjaone-cli knowledgebase delete-knowledge-base-folders`** - Delete knowledge base folders
- **`ninjaone-cli knowledgebase download-knowledge-base-article`** - Download knowledge base article
- **`ninjaone-cli knowledgebase get-client-knowledge-base-articles`** - Lists organization knowledge base articles
- **`ninjaone-cli knowledgebase get-global-knowledge-base-articles`** - Lists global knowledge base articles
- **`ninjaone-cli knowledgebase get-knowledge-base-article-signed-urls`** - Get knowledge base article signed urls
- **`ninjaone-cli knowledgebase get-knowledge-base-folder-content`** - Returns knowledge base folder and its content
- **`ninjaone-cli knowledgebase get-knowledge-base-folder-path-content`** - Returns knowledge base folder and its content
- **`ninjaone-cli knowledgebase move`** - Move knowledge base folders and documents to another knowledge base folder
- **`ninjaone-cli knowledgebase restore-knowledge-base-articles`** - Restore archived knowledge base articles
- **`ninjaone-cli knowledgebase restore-knowledge-base-folders`** - Restore archived knowledge base folders
- **`ninjaone-cli knowledgebase update-knowledge-base-articles`** - Update knowledge base articles
- **`ninjaone-cli knowledgebase upload-knowledge-base-articles`** - Upload knowledge base articles

### locations

Location

- **`ninjaone-cli locations`** - Returns flat list of all locations for all organizations

### ninjaone-public-jobs

Manage ninjaone public jobs

- **`ninjaone-cli ninjaone-public-jobs`** - Returns list of running jobs

### notification-channels

Manage notification channels

- **`ninjaone-cli notification-channels get`** - Returns list of notification channels
- **`ninjaone-cli notification-channels get-enabled`** - Returns list of enabled notification channels

### organization

Organizations

- **`ninjaone-cli organization archive-checklists`** - Archive multiple organization checklists
- **`ninjaone-cli organization archive-client-document`** - Archives an organization document by id
- **`ninjaone-cli organization archive-multi-page-client-documents`** - Archives multiple organization documents by id
- **`ninjaone-cli organization create-checklists`** - Creates multiple organization checklists
- **`ninjaone-cli organization create-documents`** - Creates organization documents and returns the documents created
- **`ninjaone-cli organization delete-client-checklist`** - Deletes an organization checklist by id
- **`ninjaone-cli organization delete-client-checklists`** - Deletes organization checklists by id
- **`ninjaone-cli organization delete-client-document`** - Deletes an archived organization document by id
- **`ninjaone-cli organization get`** - Returns organization details (policy mappings, locations)
- **`ninjaone-cli organization get-client-checklist`** - Get a client checklist by id
- **`ninjaone-cli organization get-client-checklist-signed-urls`** - Get organization checklist signed urls
- **`ninjaone-cli organization get-client-checklists`** - List client checklists with given criteria
- **`ninjaone-cli organization get-client-document-signed-urls`** - Get organization document signed urls
- **`ninjaone-cli organization get-client-documents-with-attribute-values`** - List all organization documents with field values
- **`ninjaone-cli organization get-installer`** - Generates and returns URL for installer with specified settings
- **`ninjaone-cli organization promote-client-checklists`** - Promote organization checklists by id
- **`ninjaone-cli organization promote-client-checklists-1`** - Promote organization checklists by id
- **`ninjaone-cli organization restore-checklists`** - Restore multiple organization checklists
- **`ninjaone-cli organization restore-client-document`** - Restores an organization document by id
- **`ninjaone-cli organization restore-multi-page-client-documents`** - Restore multiple multi page organization documents
- **`ninjaone-cli organization update`** - Change organization name, description and policy mappings
- **`ninjaone-cli organization update-checklists`** - Updates multiple organization checklists
- **`ninjaone-cli organization update-documents`** - Updates organization documents and returns the documents updated

### organizations

Organizations

- **`ninjaone-cli organizations create`** - Creates new organization with optional list of locations and policy mappings.
Template organization ID can be specified to copy various settings
- **`ninjaone-cli organizations get`** - Returns list of organizations (Brief mode)

### organizations-detailed

Manage organizations detailed

- **`ninjaone-cli organizations-detailed`** - Returns list of organizations with locations and policy mappings

### policies

Manage policies

- **`ninjaone-cli policies create-policy`** - Creates new policy using (New Root, Child, Copy)
- **`ninjaone-cli policies get`** - Returns list of policies

### queries

Queries

- **`ninjaone-cli queries get-antivirus-status-report`** - Returns list of statues of antivirus software installed on devices
- **`ninjaone-cli queries get-antivirus-threats`** - Returns list of antivirus threats
- **`ninjaone-cli queries get-computer-systems`** - Returns computer systems information for devices
- **`ninjaone-cli queries get-custom-fields-detailed-report`** - Returns Custom Fields report with additional information about each field
- **`ninjaone-cli queries get-custom-fields-report`** - Returns Custom Fields report
- **`ninjaone-cli queries get-device-health-report`** - Returns list of device health summary records
- **`ninjaone-cli queries get-device-usage`** - Returns the backup usage by device
- **`ninjaone-cli queries get-disk-drives`** - Returns list of physical disks
- **`ninjaone-cli queries get-installed-ospatches`** - Returns patch installation history records (successful and failed)
- **`ninjaone-cli queries get-installed-software-patches`** - Returns 3rd party software patch installation history records (successful and failed)
- **`ninjaone-cli queries get-last-logged-on-users-report`** - Returns usernames and logon times
- **`ninjaone-cli queries get-network-interfaces`** - Returns list of Network Interfaces for each device
- **`ninjaone-cli queries get-operating-systems`** - Returns operating systems' for devices
- **`ninjaone-cli queries get-pending-failed-rejected-ospatches`** - Returns list of OS patches for which there were no installation attempts
- **`ninjaone-cli queries get-pending-failed-rejected-software-patches`** - Returns list of 3rd party Software patches for which there were no installation attempts
- **`ninjaone-cli queries get-policy-overrides-1`** - Returns list of overridden policy sections for each device
- **`ninjaone-cli queries get-processors`** - Returns list of processors
- **`ninjaone-cli queries get-raidcontroller-report`** - Returns list of RAID controllers
- **`ninjaone-cli queries get-raiddrive-report`** - Returns list of drives connected to RAID controllers
- **`ninjaone-cli queries get-scoped-custom-fields-detailed-report`** - Returns report for Custom Fields defined at different scopes (device, location, organization) with additional information about each field
- **`ninjaone-cli queries get-scoped-custom-fields-report`** - Returns report for Custom Fields defined at different scopes (device, location, organization)
- **`ninjaone-cli queries get-software`** - Returns list software installed on devices
- **`ninjaone-cli queries get-volumes`** - Returns list of disk volumes
- **`ninjaone-cli queries get-windows-services-report`** - Returns list of Windows Services and their statuses

### related-items

Related Items

- **`ninjaone-cli related-items create`** - Relate an attachment to an entity
- **`ninjaone-cli related-items create-for-entity`** - Create a relation between two entities
- **`ninjaone-cli related-items create-for-entity-1`** - Create multiple relations between two entities
- **`ninjaone-cli related-items create-secure-for-entity`** - Create a relation to a secure value
- **`ninjaone-cli related-items delete`** - Deletes related item
- **`ninjaone-cli related-items delete-relateditems`** - Deletes related items associated with an entity
- **`ninjaone-cli related-items get-all`** - List all related items
- **`ninjaone-cli related-items get-attachments-signed-urls`** - Get related item attachments signed urls for an entity
- **`ninjaone-cli related-items get-for-host-entity`** - List related items for a specific host entity filterable by scope
- **`ninjaone-cli related-items get-with-entity`** - List related items for a specific related entity
- **`ninjaone-cli related-items get-with-entity-type`** - List related entities for a related entity type
- **`ninjaone-cli related-items get-with-host-entity-type`** - List relations and references for a host entity type

### roles

Manage roles

- **`ninjaone-cli roles`** - Returns list of device roles

### software-products

Manage software products

- **`ninjaone-cli software-products`** - Returns available software products (3rd party patching)

### tab

Manage tab

- **`ninjaone-cli tab create-custom-public-api`** - Create a Custom Tab with the provided details
- **`ninjaone-cli tab delete-unmanaged-device-public-api-1`** - Delete a Custom Tab
- **`ninjaone-cli tab get-custom-public-api`** - Gets a custom tab. NOTE: This will _not_ fetch tab extensions. You must use the GET tab/{tabId}/role/{roleId} for that
- **`ninjaone-cli tab get-summary-for-end-user`** - Retrieve all of the custom tabs available to end user views
- **`ninjaone-cli tab get-summary-for-organization`** - Retrieve all of the custom tabs available to organizations and locations
- **`ninjaone-cli tab get-summary-for-role`** - Retrieve all of the custom tabs that would appear for the given role
- **`ninjaone-cli tab rename-custom-public-api`** - Renames a Custom Tab
- **`ninjaone-cli tab update-custom-display`** - Using this API it is possible to configure tabs to be hidden for roles and their children
- **`ninjaone-cli tab update-custom-public-api`** - Update a Custom Tab. This API can be used to either update existing tabs, or create tab 'role extensions' for existing tabs
- **`ninjaone-cli tab update-end-user-custom-order`** - Update the order of custom tabs for end-user tabs. NOTE: All tabs defined for end-users must be specified in the payload
- **`ninjaone-cli tab update-organization-custom-order`** - Update the order of custom tabs for organizations and locations. NOTE: All tabs defined for organizations must be specified in the payload
- **`ninjaone-cli tab update-role-custom-order`** - Update the order of custom tabs for a specific role. NOTE: Only tabs created on this role can be ordered. All tabs defined on the role must be specified in the payload

### tag

Manage tag

- **`ninjaone-cli tag batch-update`** - Update tags for the supplied assetIds. Tags will be added and removed as specified
- **`ninjaone-cli tag create`** - Create an Asset Tag with the provided name and description
- **`ninjaone-cli tag delete`** - Delete Asset Tags having the provided ids
- **`ninjaone-cli tag delete-tagid`** - Delete the Asset Tag with the provided id
- **`ninjaone-cli tag get`** - Get a list of created Asset Tags
- **`ninjaone-cli tag merge`** - Merges tags. Can merge into an existing or new tag depending on the input parameters
- **`ninjaone-cli tag set-for-asset`** - Set the tags for an asset to exactly the supplied values
- **`ninjaone-cli tag update`** - Update an Asset Tag with the provided metadata

### tasks

Manage tasks

- **`ninjaone-cli tasks`** - Returns list of registered scheduled tasks

### ticketing

ticketing

- **`ninjaone-cli ticketing create`** - Create a new ticket, does not accept files
- **`ninjaone-cli ticketing create-comment`** - Add a new comment to a ticket, allows files
- **`ninjaone-cli ticketing get-all-statuses`** - Get list of ticket status
- **`ninjaone-cli ticketing get-all-user-and-contacts`** - Returns list of users (contacts, end-user, technician)
- **`ninjaone-cli ticketing get-boards`** - Returns list of ticketing boards
- **`ninjaone-cli ticketing get-contacts-1`** - Returns list of contacts
- **`ninjaone-cli ticketing get-ticket-attributes`** - Returns list of the ticket attributes
- **`ninjaone-cli ticketing get-ticket-by-id`** - Returns a ticket
- **`ninjaone-cli ticketing get-ticket-form-by-id`** - Returns a ticket form with fields
- **`ninjaone-cli ticketing get-ticket-forms`** - Returns list of ticket forms with their fields
- **`ninjaone-cli ticketing get-ticket-log-entries-by-ticket-id`** - Returns list of the ticket log entries for a ticket
- **`ninjaone-cli ticketing get-tickets-by-board`** - Run a board. Returns list of tickets matching the board condition and filters. Allows pagination
- **`ninjaone-cli ticketing update`** - Change ticket fields. Does not accept comments

### user

Users

- **`ninjaone-cli user add-role-members`** - Add members to user role
- **`ninjaone-cli user create-end`** - Create an end user
- **`ninjaone-cli user create-technician`** - Create a new technician
- **`ninjaone-cli user delete-end`** - Delete an end user
- **`ninjaone-cli user delete-technician`** - Delete a technician by their ID
- **`ninjaone-cli user get-end`** - Get details for a specific end user identifier
- **`ninjaone-cli user get-end-1`** - Get all end users
- **`ninjaone-cli user get-node-custom-fields-3`** - Returns list of end user custom fields
- **`ninjaone-cli user get-roles`** - Get list of user roles
- **`ninjaone-cli user get-technician`** - Get details for a specific technician identifier
- **`ninjaone-cli user get-technicians`** - Get all technicians
- **`ninjaone-cli user patch-end`** - Update a specific end user by their ID
- **`ninjaone-cli user remove-role-members`** - Remove users from user role
- **`ninjaone-cli user update-node-attribute-values-3`** - Update end user custom field values
- **`ninjaone-cli user update-technician`** - Update technician by their ID

### users

Users

- **`ninjaone-cli users`** - Returns list of users

### vulnerability

Manage vulnerability

- **`ninjaone-cli vulnerability fetch-all-scan-groups`** - Fetches all Scan Groups.
- **`ninjaone-cli vulnerability fetch-scan-group-by-id`** - Fetches a single Scan Group by ID.
- **`ninjaone-cli vulnerability update-scan-group`** - Upload CSV to an existing scan group. The uploaded CSV must contain the columns defined in the 
scan group which map to the Ninja machine identifier (hostname, IP address, or MAC address) and the CVE ID.

### webhook

Webhook Endpoints

- **`ninjaone-cli webhook configure`** - Creates or updates Webhook configuration for current application/client
- **`ninjaone-cli webhook disable`** - Disables Webhook configuration for current application/client


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
ninjaone-cli activities

# JSON for scripting and agents
ninjaone-cli activities --json

# Filter to specific fields
ninjaone-cli activities --json --select id,name,status

# Dry run  -  show the request without sending
ninjaone-cli activities --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
ninjaone-cli activities --agent
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
ninjaone-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/ninjaone-public-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `NINJAONE_CLIENT_ID` | auth_flow_input | Yes | Set during initial auth setup. |
| `NINJAONE_CLIENT_SECRET` | auth_flow_input | Yes | Set during initial auth setup. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `ninjaone-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `ninjaone-cli doctor` to check credentials
- Verify the environment variable is set: `echo $NINJAONE_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Verify NINJAONE_CLIENT_ID / NINJAONE_CLIENT_SECRET and that the API app has the monitoring/management scopes; re-run 'ninjaone-cli doctor'.
- **404 or wrong-region errors**  -  Set NINJAONE_BASE_URL (and NINJAONE_TOKEN_URL) to your region host, e.g. https://eu.ninjarmm.com, so calls hit the right tenant.
- **Transcendence commands return empty**  -  Run 'ninjaone-cli sync' first  -  patch-compliance, backup-coverage, av-sweep, and drift read the local store, not the live API.
- **drift shows nothing**  -  drift needs at least two syncs to compare; run sync now and again next week.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Lungshot/NinjaOneMCP**](https://github.com/Lungshot/NinjaOneMCP)  -  TypeScript
- [**marsacom/NinjaOneToolKit**](https://github.com/marsacom/NinjaOneToolKit)  -  Python
- [**jstrn/ninjapy**](https://github.com/jstrn/ninjapy)  -  Python
- [**ak9999/ninjaonepy**](https://github.com/ak9999/ninjaonepy)  -  Python
- [**wyre-technology/ninjaone-mcp**](https://github.com/wyre-technology/ninjaone-mcp)  -  TypeScript
- [**fredriksknese/mcp-ninjaone**](https://github.com/fredriksknese/mcp-ninjaone)  -  TypeScript
- [**craysiii/ninjarmm**](https://github.com/craysiii/ninjarmm)  -  Ruby

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
