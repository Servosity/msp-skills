# Jira Service Management CLI

**Every Jira Service Management Cloud request-API (servicedeskapi) and Assets endpoint in one binary, plus SLA breach radar, internal notes, ticket timelines, and CMDB-to-ticket joins.**

Work queues, SLAs, internal notes, customers, organizations and Assets (CMDB) objects from the terminal or any MCP agent. 'breaches' ranks open tickets across every queue by SLA time remaining, 'request note' always posts agent-only notes (public=false) and errors if JSM stores one as public, and 'cmdb tickets' joins Assets objects to the tickets that touch them. Pagination walks past JSM's known paging bugs, and a local store powers offline search, 'sla-report' and per-client views.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `jira-service-management-cli` and `jira-service-management-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex
   ```
3. Verify: `jira-service-management-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=jira-service-management). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/jira-service-management --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/jira-service-management --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `jira-service-management-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the jira-service-management skill from https://github.com/Servosity/msp-skills/tree/main/skills/jira-service-management. The skill defines how its required CLI (`jira-service-management-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=jira-service-management).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in your site subdomain, Atlassian email and API token when Claude Desktop prompts you. Cloud ID and workspace ID are only needed for Assets commands; `jira-service-management-cli cmdb discover` prints them.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS (Apple Silicon and Intel), Linux (`amd64`) and Windows (`amd64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "jira-service-management": {
      "command": "jira-service-management-mcp",
      "env": {
        "JIRA_SERVICE_MANAGEMENT_SITE": "<site-subdomain>",
        "JIRA_SERVICE_MANAGEMENT_EMAIL": "<you@example.com>",
        "JIRA_SERVICE_MANAGEMENT_API_TOKEN": "<api-token>",
        "JIRA_SERVICE_MANAGEMENT_CLOUD_ID": "<cloudId (Assets only)>",
        "JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID": "<workspaceId (Assets only)>"
      }
    }
  }
}
```

</details>

## Authentication

Jira Service Management Cloud uses HTTP Basic auth with your Atlassian account email and an API token (create one at id.atlassian.com/manage-profile/security/api-tokens). Set JIRA_SERVICE_MANAGEMENT_SITE to your site subdomain (acme for acme.atlassian.net), JIRA_SERVICE_MANAGEMENT_EMAIL and JIRA_SERVICE_MANAGEMENT_API_TOKEN. Assets (CMDB) commands call api.atlassian.com, need a JSM Premium or Enterprise site, and also need JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID: run 'cmdb discover' to print them or 'cmdb discover --save' to store them in the config file. The same credential works for both APIs. What you can see follows your Jira permissions: agents see their desks' requests and internal notes, customers see only their own requests. Set the site rather than JIRA_SERVICE_MANAGEMENT_BASE_URL: a base-URL override also redirects Assets calls.

## Quick Start

```bash
# Health check: lists missing EMAIL or API_TOKEN, flags an unset SITE as a placeholder base_url, then probes the API.
jira-service-management-cli doctor

# List the service desks your account can see; note the desk id.
jira-service-management-cli servicedesk list --agent

# Rank open tickets across that desk's queues by SLA time remaining.
jira-service-management-cli breaches --desk 3 --within 2h --agent

# Read a ticket's full history, internal notes labeled, before you act on it.
jira-service-management-cli request timeline SD-123 --agent

# Preview an agent-only internal note; drop --dry-run to post it.
jira-service-management-cli request note SD-123 "Escalated to network team" --dry-run

# Resolve the cloudId and Assets workspaceId that Assets commands need.
jira-service-management-cli cmdb discover --agent

```

## Unique Features

### SLA and queue triage
- **`breaches`**  -  Rank open tickets that have a running SLA across your queues by time remaining, breached first, each ticket counted once (paused-only SLAs skipped unless --include-paused).

  _Reach for this when asked what is about to breach right now; one call replaces a queue walk plus one SLA call per ticket._

  ```bash
  jira-service-management-cli breaches --desk 3 --within 2h --agent
  ```
- **`sla-report`**  -  Breach rate, breached count, and median elapsed time vs median goal per desk, request type or SLA metric over a period. Reads the local store: run 'sync --resources request,servicedesk,requesttype' first; results are as of the last sync.

  _Use this for weekly SLA reporting; it answers in one local query what would otherwise be one API call per ticket._

  ```bash
  jira-service-management-cli sla-report --since 30d --group-by requesttype --agent
  ```

### MSP client hygiene
- **`roster`**  -  Get the complete customer list for a desk and spot customers with no organization, customers in two organizations, empty organizations, and unattached organizations that hold this desk's customers.

  _Use this for MSP customer onboarding/offboarding hygiene; the customers endpoint returns at most 50 people a page, and roster reads every page and de-duplicates._

  ```bash
  jira-service-management-cli roster 3 --agent
  ```
- **`client`**  -  One client organization's open tickets by status and request type, oldest open age, and how many are breaching. Reads the local store: run 'sync --resources organization,request,requesttype' first; counts are as of the last sync.

  _Use this before a client call to see how that client's tickets are doing._

  ```bash
  jira-service-management-cli client "Acme Corp" --agent
  ```

### Working a ticket
- **`request timeline`**  -  Read one ticket's whole story in order: comments labeled public or internal, status changes, approvals, participants and SLA state.

  _Use this before replying on a ticket so internal notes are never missed and never mistaken for customer-visible replies._

  ```bash
  jira-service-management-cli request timeline SD-123 --agent
  ```
- **`request note`**  -  Add an agent-only internal note to a request: it always posts public=false and errors if JSM stores the comment as public.

  _Pick this over a generic comment call whenever the note is for other agents, not the customer._

  ```bash
  jira-service-management-cli request note SD-123 "Escalated to network team" --dry-run
  ```

### Assets (CMDB)
- **`cmdb tickets`**  -  Go from a hostname, Assets key or AQL query to the matching Assets objects and every service desk ticket connected to them, with each ticket's SLA breach state. Needs Assets (JSM Premium or Enterprise) plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID; run 'cmdb discover --save' once.

  _Use this when a ticket names a device and you need its history across tickets._

  ```bash
  jira-service-management-cli cmdb tickets srv-01 --agent
  ```
- **`cmdb reconcile`**  -  Diff an Assets object type against an RMM or PSA CSV export: missing from Assets, missing from inventory, and attribute mismatches. Needs Assets (JSM Premium or Enterprise) plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID; run 'cmdb discover --save' once.

  _Use this for the weekly CMDB-vs-inventory check instead of a spreadsheet VLOOKUP._

  ```bash
  jira-service-management-cli cmdb reconcile --type Laptop --file rmm.csv --key Name --compare Serial,OS --agent
  ```
- **`cmdb discover`**  -  Find your site's cloudId and Assets workspaceId (Assets requires JSM Premium or Enterprise) and optionally save them with --save so Assets commands work.

  _Run this once when Assets commands report a missing cloudId or workspaceId._

  ```bash
  jira-service-management-cli cmdb discover --agent
  ```

## Recipes

### What breaches next

```bash
jira-service-management-cli breaches --desk 3 --within 4h --agent
```

Walks every queue on desk 3 once, de-duplicates tickets, keeps only tickets already breached or breaching within 4 hours, and ranks them by the smallest SLA time remaining.

### Read a ticket with its SLA, narrowed for an agent

```bash
jira-service-management-cli request get SD-123 --expand sla --agent --select issueKey,currentStatus.status,sla.values.name,sla.values.ongoingCycle.remainingTime.friendly
```

Fetches one request with SLA expanded and keeps only the status and each SLA's remaining time.

### Work one queue, narrowed for an agent

```bash
jira-service-management-cli servicedesk queue list-issues 3 10 --agent --select key,fields.summary
```

Lists the tickets in queue 10 of desk 3 with only their keys and summaries; use 'request timeline <key>' next, and 'request note <key> "text"' for an agent-only note.

### Reply to the customer, then move the ticket

```bash
jira-service-management-cli request comment add SD-123 --body "We replaced the toner; please confirm it prints." --public --dry-run
```

Previews a customer-visible reply; then run 'request transition list SD-123' to find the transition id and 'request transition perform SD-123 --id <id>' to move it.

### Weekly SLA rollup

```bash
jira-service-management-cli sla-report --since 7d --group-by requesttype --agent
```

Breach rate per request type over the last week from the local store; run 'sync --resources request,servicedesk,requesttype' first so the numbers are current.

## Usage

Run `jira-service-management-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `JIRA_SERVICE_MANAGEMENT_CONFIG_DIR`, `JIRA_SERVICE_MANAGEMENT_DATA_DIR`, `JIRA_SERVICE_MANAGEMENT_STATE_DIR`, or `JIRA_SERVICE_MANAGEMENT_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `JIRA_SERVICE_MANAGEMENT_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export JIRA_SERVICE_MANAGEMENT_HOME=/srv/jira-service-management
jira-service-management-cli doctor
```

On Windows (PowerShell):

```powershell
$env:JIRA_SERVICE_MANAGEMENT_HOME = "C:\ProgramData\jira-service-management"
jira-service-management-cli doctor
```

Under `JIRA_SERVICE_MANAGEMENT_HOME=/srv/jira-service-management`, the four dirs resolve to `/srv/jira-service-management/config`, `/srv/jira-service-management/data`, `/srv/jira-service-management/state`, and `/srv/jira-service-management/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "jira-service-management": {
      "command": "jira-service-management-mcp",
      "env": {
        "JIRA_SERVICE_MANAGEMENT_HOME": "/srv/jira-service-management"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `JIRA_SERVICE_MANAGEMENT_DATA_DIR` overrides an explicit `--home` for that kind. Use `JIRA_SERVICE_MANAGEMENT_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `JIRA_SERVICE_MANAGEMENT_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `jira-service-management-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### api-import

Manage api import

- **`jira-service-management-cli api-import <id>`** - Start configured imports. To see an ongoing import see the Progress resource

### aql

Manage aql

- **`jira-service-management-cli aql`** - Deprecated by Atlassian (removal date passed); use 'object find-by-aql'. This endpoint is deprecated and will be removed on 18th of September 2024. Please use POST `/object/aql` instead. Find objects based on Assets Query Language (AQL)

### assets

Manage assets

- **`jira-service-management-cli assets`** - Returns a list of Assets workspace IDs. Include a workspace ID in the path to access the [Assets REST APIs](https://developer.atlassian.com/cloud/assets/rest).

**Permissions required**: Any

### config

Manage config

- **`jira-service-management-cli config create-referencetype`** - Create a reference type (Assets).
- **`jira-service-management-cli config create-statustype`** - Create a new status
- **`jira-service-management-cli config delete-statustype`** - Delete an existing status
- **`jira-service-management-cli config get-statustype`** - Find a status by id
- **`jira-service-management-cli config list-referencetypes`** - Get reference type
- **`jira-service-management-cli config list-statustypes`** - Find all status
- **`jira-service-management-cli config update-statustype`** - Update an existing status

### customer

Manage customer

- **`jira-service-management-cli customer create`** - This method adds a customer to the Jira Service Management instance by passing a JSON file including an email address and display name. The display name does not need to be unique. The record's identifiers, `name` and `key`, are automatically generated from the request details.

**Permissions required**: Jira Administrator Global permission
- **`jira-service-management-cli customer create-skippermissioncheck`** - Atlassian-internal endpoint (ASAP auth only); not callable with an API token. Creates a customer account on behalf of jsd-nutmeg.

This endpoint is restricted to jsd-nutmeg via ASAP authentication. It provides the same capability as the public `POST /servicedeskapi/customer` endpoint, but does not require a User Context Token (UCT) or Connect app user \ -  authorization is enforced entirely via the ASAP token.

No user permission checks are performed; `null` is passed as the acting user to bypass the permission check in the underlying service.
- **`jira-service-management-cli customer revoke-portal-only-access-for-user`** - This method revokes portal-only access for a particular user, removing their ability to log in to the Jira Service Management customer portal as a portal-only user. After revocation, the user cannot submit or view requests through the portal.

**Permissions required:** Site administration (that is, member of the *site-admin* [group](https://confluence.atlassian.com/x/24xjL)).

### global

Manage global

- **`jira-service-management-cli global <id>`** - Update general configuration for object schema

### icon

Manage icon

- **`jira-service-management-cli icon get`** - Load a single icon by id
- **`jira-service-management-cli icon list-global`** - Return all global icons i.e. icons not associated with a particular object schema

### importsource

Manage importsource

- **`jira-service-management-cli importsource <id>`** - Retrieves a specific import source configuration by its ID. If scheduled imports are enabled, the response includes scheduling information.

### insight

Manage insight

- **`jira-service-management-cli insight`** - This endpoint is deprecated, please use /assets/workspace/.

### instance

Manage instance

- **`jira-service-management-cli instance`** - This method retrieves information about the Jira Service Management instance such as software version, builds, and related links.

**Permissions required**: None, the user does not need to be logged in.

### iql

Manage iql

- **`jira-service-management-cli iql`** - Deprecated by Atlassian (removal date passed); use 'object find-by-aql'. This endpoint is deprecated and will be removed on 18th of September 2024. Please use POST `/object/aql` instead.

### knowledgebase

Manage knowledgebase

- **`jira-service-management-cli knowledgebase list-articles`** - Returns articles which match the given query string across all service desks.

**Permissions required**: Permission to access the [customer portal](https://confluence.atlassian.com/servicedeskcloud/configuring-the-customer-portal-732528918.html).
- **`jira-service-management-cli knowledgebase view-article`** - View knowledge base article

### object

Manage object

- **`jira-service-management-cli object count-by-aql`** - This API provides the total count of objects that match a specified AQL query. Please note that this operation may incur performance latency.
- **`jira-service-management-cli object create`** - Create a new object in Assets
- **`jira-service-management-cli object delete`** - Delete the referenced object
- **`jira-service-management-cli object find-by-aql`** - Fetch Objects by AQL
- **`jira-service-management-cli object get`** - Load one object
- **`jira-service-management-cli object list-connected-tickets`** - Relation between Jira issues and Assets objects
- **`jira-service-management-cli object navlist-aql`** - Retrieve a list of objects based on an AQL. Deprecated from 30 September 2024. Please use POST /object/aql instead. For more information please see https://developer.atlassian.com/changelog/#CHANGE-1661.
- **`jira-service-management-cli object navlist-iql`** - Deprecated by Atlassian (removal date passed); use 'object navlist-aql'. Deprecated. Use `/object/navlist/aql` instead.
- **`jira-service-management-cli object update`** - Update an existing object in Assets

### objectschema

Manage objectschema

- **`jira-service-management-cli objectschema create`** - Create a new object schema
- **`jira-service-management-cli objectschema delete`** - Delete a schema
- **`jira-service-management-cli objectschema get`** - Find a schema by id
- **`jira-service-management-cli objectschema list`** - Resource to find object schemas in Assets
- **`jira-service-management-cli objectschema update`** - Update an object schema

### objecttype

Manage objecttype

- **`jira-service-management-cli objecttype create`** - Create a new object type
- **`jira-service-management-cli objecttype delete`** - Delete an object type
- **`jira-service-management-cli objecttype get`** - Find an object type by id
- **`jira-service-management-cli objecttype update`** - Update an existing object type

### objecttypeattribute

Manage objecttypeattribute

- **`jira-service-management-cli objecttypeattribute create`** - Create a new attribute on the given object type
- **`jira-service-management-cli objecttypeattribute delete`** - Delete an existing object type attribute
- **`jira-service-management-cli objecttypeattribute update`** - Update an existing object type attribute

### organization

Manage organization

- **`jira-service-management-cli organization create`** - This method creates an organization by passing the name of the organization.

**Permissions required**: Service desk administrator or agent. Note: Permission to create organizations can be switched to users with the Jira administrator permission, using the **[Organization management](https://confluence.atlassian.com/servicedeskcloud/setting-up-service-desk-users-732528877.html#Settingupservicedeskusers-manageorgsManageorganizations)** feature.
- **`jira-service-management-cli organization delete`** - This method deletes an organization. Note that the organization is deleted regardless of other associations it may have. For example, associations with service desks.

**Permissions required**: Jira administrator.
- **`jira-service-management-cli organization get`** - This method returns details of an organization. Use this method to get organization details whenever your application component is passed an organization ID but needs to display other organization details.

To get organization detail field values which are visible in Jira Service Management, see the [Customer Service Management REST API](https://developer.atlassian.com/cloud/customer-service-management/rest/v1/api-group-organization/#api-group-organization).

**Permissions required**: Any

**Response limitations**: Customers can only retrieve organization of which they are members.
- **`jira-service-management-cli organization list`** - This method returns a list of organizations in the Jira Service Management instance. Use this method when you want to present a list of organizations or want to locate an organization by name.

**Permissions required**: Any. However, to fetch organizations based on `accountId` the user must have a Service Desk agent license.

**Response limitations**: If the user is a customer, only those organizations of which the customer is a member are listed.

### progress

Manage progress

- **`jira-service-management-cli progress <id>`** - Show ongoing import process

### request

Manage request

- **`jira-service-management-cli request create`** - This method creates a customer request in a service desk.

The JSON request must include the service desk and customer request type, as well as any fields that are required for the request type. A list of the fields required by a customer request type can be obtained using servicedesk/\{serviceDeskId\}/requesttype/\{requestTypeId\}/field.

The fields required for a customer request type depend on the user's permissions:

 *  `raiseOnBehalfOf` is not available to Users who have the customer permission only.
 *  `requestParticipants` is not available to Users who have the customer permission only or if the feature is turned off for customers.

`requestFieldValues` is a map of Jira field IDs and their values. See Field input formats, for details of each field's JSON semantics and the values they can take.

**Permissions required**: Permission to create requests in the specified service desk.
- **`jira-service-management-cli request get`** - This method returns a customer request.

**Permissions required**: Permission to access the specified service desk.

**Response limitations**: For customers, only a request they created, was created on their behalf, or they are participating in will be returned.

**Note:** `requestFieldValues` does not include hidden fields. To get a list of request type fields that includes hidden fields, see [/rest/servicedeskapi/servicedesk/\{serviceDeskId\}/requesttype/\{requestTypeId\}/field](https://developer.atlassian.com/cloud/jira/service-desk/rest/api-group-servicedesk/#api-rest-servicedeskapi-servicedesk-servicedeskid-requesttype-requesttypeid-field-get)
- **`jira-service-management-cli request list`** - This method returns all customer requests for the user executing the query.

The returned customer requests are ordered chronologically by the latest activity on each request. For example, the latest status transition or comment.

**Permissions required**: Permission to access the specified service desk.

**Response limitations**: For customers, the list returned will include request they created (or were created on their behalf) or are participating in only.
- **`jira-service-management-cli request validate`** - Validates a customer request payload without creating (persisting) a request.

This endpoint runs exactly the same structural and semantic validations as Create customer request \ -  including ProForma form validation \ -  but performs **no mutation**: no issue is created and no side effects (attachments, comments, analytics) run.

The response is intentionally verbose and structured so that it can be consumed by automated agents (for example an LLM repairing an invalid payload): every failure carries a machine-readable location (field id / form entity) and a human-readable reason. A valid payload returns HTTP 200 with `valid: true`; an invalid payload returns HTTP 400 with `valid: false` together with the field, form and general validation errors.

**Permissions required**: Permission to create requests in the specified service desk.

### requesttype

Manage requesttype

- **`jira-service-management-cli requesttype`** - This method returns all customer request types used in the Jira Service Management instance, optionally filtered by a query string.

Use servicedeskapi/servicedesk/\{serviceDeskId\}/requesttype to find the customer request types supported by a specific service desk.

The returned list of customer request types can be filtered using the `searchQuery` parameter. The parameter is matched against the customer request types' `name` or `description`. For example, searching for "Install", "Inst", "Equi", or "Equipment" will match a customer request type with the *name* "Equipment Installation Request".

**Note:** This API by default will filter out request types hidden in the portal (i.e. request types without groups and request types where a user doesn't have permission) when `searchQuery` is provided, unless `includeHiddenRequestTypesInSearch` is set to true. Restricted request types will not be returned for those who aren't admins.

**Permissions required**: Any

### servicedesk

Manage servicedesk

- **`jira-service-management-cli servicedesk get`** - This method returns a service desk. Use this method to get service desk details whenever your application component is passed a service desk ID but needs to display other service desk details.

**Permissions required**: Permission to access the Service Desk. For example, being the Service Desk's Administrator or one of its Agents or Users.
- **`jira-service-management-cli servicedesk list`** - This method returns all the service desks in the Jira Service Management instance that the user has permission to access. Use this method where you need a list of service desks or need to locate a service desk by name or keyword.

**Note:** This method will be slow if the instance has hundreds of service desks. If you want to fetch a single service desk by its ID, use [/rest/servicedeskapi/servicedesk/\{serviceDeskId\}](./#api-rest-servicedeskapi-servicedesk-servicedeskid-get) instead.

**Permissions required**: Any

### usage

Manage usage

- **`jira-service-management-cli usage`** - Assets object counts for the tenant, total and per schema.


### breaches


- **`jira-service-management-cli breaches`** - Rank open tickets that have a running SLA across your queues by time remaining, breached first, each ticket counted once (paused-only SLAs skipped unless --include-paused).


### client


- **`jira-service-management-cli client`** - One client organization's open tickets by status and request type, oldest open age, and how many are breaching. Reads the local store: run 'sync --resources organization,request,requesttype' first; counts are as of the last sync.


### cmdb


- **`jira-service-management-cli cmdb tickets`** - Go from a hostname, Assets key or AQL query to the matching Assets objects and every service desk ticket connected to them, with each ticket's SLA breach state. Needs Assets (JSM Premium or Enterprise) plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID; run 'cmdb discover --save' once.

- **`jira-service-management-cli cmdb reconcile`** - Diff an Assets object type against an RMM or PSA CSV export: missing from Assets, missing from inventory, and attribute mismatches. Needs Assets (JSM Premium or Enterprise) plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID; run 'cmdb discover --save' once.

- **`jira-service-management-cli cmdb discover`** - Find your site's cloudId and Assets workspaceId (Assets requires JSM Premium or Enterprise) and optionally save them with --save so Assets commands work.


### request


- **`jira-service-management-cli request timeline`** - Read one ticket's whole story in order: comments labeled public or internal, status changes, approvals, participants and SLA state.

- **`jira-service-management-cli request note`** - Add an agent-only internal note to a request: it always posts public=false and errors if JSM stores the comment as public.


### roster


- **`jira-service-management-cli roster`** - Get the complete customer list for a desk and spot customers with no organization, customers in two organizations, empty organizations, and unattached organizations that hold this desk's customers.


### sla-report


- **`jira-service-management-cli sla-report`** - Breach rate, breached count, and median elapsed time vs median goal per desk, request type or SLA metric over a period. Reads the local store: run 'sync --resources request,servicedesk,requesttype' first; results are as of the last sync.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`jira-service-management-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`jira-service-management-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`jira-service-management-cli learnings list`** - Inspect taught rows
- **`jira-service-management-cli learnings forget <query>`** - Undo a teach
- **`jira-service-management-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`jira-service-management-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`jira-service-management-cli teach-pattern`** - Install a query/resource template up front
- **`jira-service-management-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `JIRA_SERVICE_MANAGEMENT_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `jira-service-management-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
jira-service-management-cli servicedesk list

# JSON for scripting and agents
jira-service-management-cli servicedesk list --json
# Filter to specific fields
jira-service-management-cli servicedesk list --json --select id,projectKey,projectName

# Dry run  -  show the request without sending
jira-service-management-cli servicedesk list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
jira-service-management-cli servicedesk list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` other error (for example `request note` refusing a note JSM stored as public), `2` usage error, `3` not found, `4` auth error, `5` API error (also a missing SITE, CLOUD_ID or WORKSPACE_ID variable), `6` partial failure, `7` rate limited, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `JIRA_SERVICE_MANAGEMENT_SITE` resolves `{site}`
- `JIRA_SERVICE_MANAGEMENT_CLOUD_ID` resolves `{cloudId}`
- `JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID` resolves `{workspaceId}`

Base URL: `https://{site}.atlassian.net/rest/servicedeskapi`

Assets base URL: `https://api.atlassian.com/ex/jira/{cloudId}/jsm/assets/workspace/{workspaceId}/v1`

## Health Check

```bash
jira-service-management-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `jira-service-management-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/jira-service-management-cli/config.toml` (`%USERPROFILE%\.config\jira-service-management-cli\config.toml` on Windows); `--home`, `JIRA_SERVICE_MANAGEMENT_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `JIRA_SERVICE_MANAGEMENT_SITE` | endpoint | Yes | Site subdomain: `acme` for acme.atlassian.net. |
| `JIRA_SERVICE_MANAGEMENT_CLOUD_ID` | endpoint | Assets only | Site cloudId; `cmdb discover --save` stores it. |
| `JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID` | endpoint | Assets only | Assets workspaceId; `cmdb discover --save` stores it. |
| `JIRA_SERVICE_MANAGEMENT_BASE_URL` | override | No | Replaces the JSM base URL; also redirects Assets calls, so prefer SITE. |
| `JIRA_SERVICE_MANAGEMENT_CONFIG` | override | No | Path to an alternate config.toml. |
| `JIRA_SERVICE_MANAGEMENT_EMAIL` | per_call | Yes | Atlassian account email (Basic auth username). |
| `JIRA_SERVICE_MANAGEMENT_API_TOKEN` | per_call | Yes | Atlassian API token (Basic auth password), from id.atlassian.com/manage-profile/security/api-tokens. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `jira-service-management-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `jira-service-management-cli doctor` to check credentials
- Run `jira-service-management-cli doctor`: it lists a missing EMAIL or API_TOKEN, and an unset SITE shows as a placeholder base_url
- A 403 also exits 4: the account lacks permission (for example a customer-only account)
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `jira-service-management-cli servicedesk list --agent` (or the matching `list` subcommand) to see valid ids

### API-specific
- **HTTP 401 Unauthorized on every command**  -  Set JIRA_SERVICE_MANAGEMENT_EMAIL to your Atlassian account email and JIRA_SERVICE_MANAGEMENT_API_TOKEN to an API token (not your password), then run 'jira-service-management-cli doctor'.
- **'JIRA_SERVICE_MANAGEMENT_SITE not set', or DNS errors for a host like acme.atlassian.net.atlassian.net**  -  Set the subdomain only, acme for acme.atlassian.net: 'export JIRA_SERVICE_MANAGEMENT_SITE=acme' on macOS or Linux, '$env:JIRA_SERVICE_MANAGEMENT_SITE = "acme"' in Windows PowerShell.
- **Assets commands fail with a missing cloudId or workspaceId, or 404**  -  Run 'jira-service-management-cli cmdb discover --save' to store both ids in the config file, or run 'cmdb discover' and set JIRA_SERVICE_MANAGEMENT_CLOUD_ID and JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID to the values it prints.
- **A desk's customer list stops at 50 people**  -  The customers endpoint returns at most 50 people a page. Add --all to 'servicedesk customer list', or use 'jira-service-management-cli roster <desk-id>', which pages until an empty or short page and de-duplicates.
- **sync stores few or no requests for an agent account**  -  The request list is the customer view; sync asks for requestOwnership=ALL_REQUESTS. Use 'breaches --desk <id>' or 'servicedesk queue list-issues' for the agent's queue view.
- **sla-report or client returns all zeros or requests_scanned 0**  -  The local store is empty or stale; run 'jira-service-management-cli sync --resources request,servicedesk,requesttype,organization' (a bare sync also walks Assets, which fails on non-Premium sites).

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**atlassian-python-api**](https://github.com/atlassian-api/atlassian-python-api)  -  Python (1679 stars)
- [**atlassian-mcp-server**](https://github.com/atlassian/atlassian-mcp-server)  -  JavaScript (1092 stars)
- [**go-atlassian**](https://github.com/ctreminiom/go-atlassian)  -  Go (217 stars)
- [**jira-cli (jr)**](https://github.com/Zious11/jira-cli)  -  Rust (5 stars)
- [**jira-insights-mcp**](https://github.com/aaronsb/jira-insights-mcp)  -  TypeScript (3 stars)
- [**jira-service-desk-mcp**](https://github.com/CzSadykov/jira-service-desk-mcp)  -  Python (1 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
