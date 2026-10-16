# ConnectWise RMM CLI

**66 ConnectWise RMM (Asio) partner API operations, plus a local fleet database, patch-compliance rollups, and script runs you can preview, schedule and watch in one command.**

Lists companies, sites and endpoints across every client, reads patch state, logged-on users, services and resource usage, and drives the automation engine: run previews a script on named endpoints and, with --confirm, schedules it and collects per-endpoint results. Sync the fleet into SQLite once and the fleet commands roll it up across clients; scopes tells you exactly which OAuth scopes your API key was granted.

Learn more at [ConnectWise RMM](https://developer.connectwise.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `connectwise-rmm-cli` and `connectwise-rmm-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.ps1 | iex
   ```
3. Verify: `connectwise-rmm-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=connectwise-rmm). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/connectwise-rmm --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/connectwise-rmm --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `connectwise-rmm-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the connectwise-rmm skill from https://github.com/Servosity/msp-skills/tree/main/skills/connectwise-rmm. The skill defines how its required CLI (`connectwise-rmm-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download `connectwise-rmm-mcp.mcpb` from the [latest release](https://github.com/Servosity/msp-skills/releases?q=connectwise-rmm). One bundle covers every supported platform.
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `CONNECTWISE_RMM_CLIENT_ID` and `CONNECTWISE_RMM_CLIENT_SECRET` when Claude Desktop prompts you. If your API key was not granted every scope, also set `CONNECTWISE_RMM_OAUTH_SCOPE` to the line `connectwise-rmm-cli scopes` prints.

Requires Claude Desktop 1.0.0 or later. The bundle carries the MCP server and its companion CLI for macOS (one universal binary for Intel and Apple Silicon), Linux x64 and Windows x64. Linux on ARM and Windows on ARM are released as standalone binaries but are not bundled, so use the manual config below there.

> **Claude Desktop bundle:** every tool shells out to the companion `connectwise-rmm-cli`, which the bundle ships beside the MCP server; if yours lacks it, run the installer above first (or set `CONNECTWISE_RMM_CLI_PATH` to an existing binary). Details: [#331](https://github.com/Servosity/msp-skills/issues/331).

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "connectwise-rmm": {
      "command": "connectwise-rmm-mcp",
      "env": {
        "CONNECTWISE_RMM_CLIENT_ID": "<your-client-id>",
        "CONNECTWISE_RMM_CLIENT_SECRET": "<your-client-secret>",
        "CONNECTWISE_RMM_OAUTH_SCOPE": "<scopes printed by connectwise-rmm-cli scopes>"
      }
    }
  }
}
```

</details>

## Authentication

ConnectWise RMM uses OAuth2 client credentials. Create an API key under Integrations > API Access in the ConnectWise portal, then export `CONNECTWISE_RMM_CLIENT_ID` and `CONNECTWISE_RMM_CLIENT_SECRET`. The CLI mints and caches the bearer token itself by posting a JSON body to `<base_url>/v1/token`. The client ID prefix picks the region (0e30 North America, 0e31 Europe, 0e32 Australia); set `CONNECTWISE_RMM_BASE_URL` to override. Run `connectwise-rmm-cli scopes --agent` and export the `CONNECTWISE_RMM_OAUTH_SCOPE` line it prints, because one ungranted scope fails the whole token request. `connectwise-rmm-cli auth login` mints and caches a token explicitly.

## Quick Start

```bash
# Smoke-test the install without calling the API; run doctor without --dry-run to check credentials, region and reachability
connectwise-rmm-cli doctor --dry-run

# Find which scopes your API key was granted and copy the CONNECTWISE_RMM_OAUTH_SCOPE line it prints
connectwise-rmm-cli scopes --agent

# Pull the fleet into the local store; every fleet command reads this copy
connectwise-rmm-cli sync --resources companies,sites,endpoints,tickets

# Find agents that are offline or went silent across every client
connectwise-rmm-cli fleet stale --older-than 24h --agent

# Rank companies by patch risk after Patch Tuesday
connectwise-rmm-cli fleet patch-posture --min-compliance 90 --agent

# One card for a machine named in a ticket (machine names work once fleet whois has cached them)
connectwise-rmm-cli fleet brief 4a9caa07-d45a-4c14-aa36-8c1c26e7a029 --agent

```

## Unique Features

### Fleet health
- **`fleet stale`**  -  Every endpoint across all companies that is offline, missing from the heartbeat, or silent longer than a threshold, sorted by company and silence.

  _Reach for this for 'which agents are offline' across clients instead of looping heartbeat calls yourself._

  ```bash
  connectwise-rmm-cli fleet stale --older-than 24h --agent
  ```
- **`fleet whois`**  -  Which endpoints a person is logged on to now or was last logged on to, with company and site names.

  _Use this when a ticket names a person, not a machine._

  ```bash
  connectwise-rmm-cli fleet whois jsmith --company "Acme Corp" --agent
  ```
- **`fleet brief`**  -  One card for one endpoint: online or last seen, logged-on users, patch counts, and tickets linked to it or its site.

  _Use this first when a ticket mentions one machine._

  ```bash
  connectwise-rmm-cli fleet brief 4a9caa07-d45a-4c14-aa36-8c1c26e7a029 --agent
  ```

### Patch compliance
- **`fleet patch-posture`**  -  Companies ranked by patch risk: endpoints below a compliance bar, missing patches, pending reboots, out of support, stale assessments.

  _Use this for per-company patch questions; the API itself is per endpoint._

  ```bash
  connectwise-rmm-cli fleet patch-posture --min-compliance 90 --agent
  ```
- **`fleet scorecard`**  -  Per-site health card for one company: endpoint mix, offline count, patch risk and tickets by status.

  _Use this to prepare a client review._

  ```bash
  connectwise-rmm-cli fleet scorecard "Acme Corp" --agent
  ```
- **`fleet reboot-due`**  -  Endpoints with pending reboots split into reboot now, coordinate (user logged on or sessions unknown), offline, and availability unknown.

  _Use this as the patch-week reboot work queue._

  ```bash
  connectwise-rmm-cli fleet reboot-due --agent
  ```

### Automation
- **`fleet task-failures`**  -  Failed endpoints from a script run grouped by identical error text, joined to endpoint names, with IDs ready to re-run.

  _Use this after a fleet script push to see what failed and why._

  ```bash
  connectwise-rmm-cli fleet task-failures 7c9e6679-7425-40de-944b-e07fc1f90ae7 --agent
  ```

## Recipes

### Monday offline sweep

```bash
connectwise-rmm-cli fleet stale --older-than 48h --type Server --agent
```

Offline servers plus servers silent for over 48 hours or missing from the heartbeat, sorted by company and silence.

### Patch numbers for one endpoint

```bash
connectwise-rmm-cli patches get-endpoint 4a9caa07-d45a-4c14-aa36-8c1c26e7a029 --agent --select summary.compliance,summary.missingCount,summary.pendingRebootCount,summary.policyName
```

Narrow the patch payload to the four numbers a client review needs.

### Which machines is this user on

```bash
connectwise-rmm-cli fleet whois jsmith --company "Acme Corp" --agent
```

Every endpoint where a user is logged on now or was the last user, with company and site names.

### Reboot queue

```bash
connectwise-rmm-cli fleet reboot-due --agent
```

Pending-reboot endpoints split into reboot now, coordinate (user logged on or sessions unknown), offline, and availability unknown.

### Run a script safely

```bash
connectwise-rmm-cli run --script "Restart Print Spooler" --endpoint 4a9caa07-d45a-4c14-aa36-8c1c26e7a029 --agent
```

Preview only: resolves the script and targets and prints the exact request. Add --confirm --wait to schedule it and collect per-endpoint results.

### Why did the script fail

```bash
connectwise-rmm-cli fleet task-failures 7c9e6679-7425-40de-944b-e07fc1f90ae7 --agent
```

Failed endpoints grouped by identical error text, with endpoint IDs ready to pass back to run.

## Usage

Run `connectwise-rmm-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CONNECTWISE_RMM_CONFIG_DIR`, `CONNECTWISE_RMM_DATA_DIR`, `CONNECTWISE_RMM_STATE_DIR`, or `CONNECTWISE_RMM_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CONNECTWISE_RMM_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CONNECTWISE_RMM_HOME=/srv/connectwise-rmm
connectwise-rmm-cli doctor
```

On Windows (PowerShell):

```powershell
$env:CONNECTWISE_RMM_HOME = "C:\ProgramData\connectwise-rmm"
connectwise-rmm-cli doctor
```

Under `CONNECTWISE_RMM_HOME=/srv/connectwise-rmm`, the four dirs resolve to `/srv/connectwise-rmm/config`, `/srv/connectwise-rmm/data`, `/srv/connectwise-rmm/state`, and `/srv/connectwise-rmm/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "connectwise-rmm": {
      "command": "connectwise-rmm-mcp",
      "env": {
        "CONNECTWISE_RMM_HOME": "/srv/connectwise-rmm"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CONNECTWISE_RMM_DATA_DIR` overrides an explicit `--home` for that kind. Use `CONNECTWISE_RMM_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CONNECTWISE_RMM_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `connectwise-rmm-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### companies

Manage companies

- **`connectwise-rmm-cli companies get-company`** - Get a company by ID.

Scope: platform.companies.read.
- **`connectwise-rmm-cli companies list`** - List all companies (clients).

Scope: platform.companies.read.

### custom_fields

Manage custom fields

- **`connectwise-rmm-cli custom-fields get-company`** - Get custom field values of a company.

Scope: platform.custom_fields_values.read.
- **`connectwise-rmm-cli custom-fields get-definition`** - Get one custom field definition.

Scope: platform.custom_fields_definitions.read.
- **`connectwise-rmm-cli custom-fields get-endpoint`** - Get custom field values of an endpoint.

Scope: platform.custom_fields_values.read.
- **`connectwise-rmm-cli custom-fields get-site`** - Get custom field values of a site.

Scope: platform.custom_fields_values.read.
- **`connectwise-rmm-cli custom-fields list-definitions`** - List custom field definitions.

Scope: platform.custom_fields_definitions.read.
- **`connectwise-rmm-cli custom-fields set-company`** - Set custom field values on a company.

Scope: platform.custom_fields_values.write.
- **`connectwise-rmm-cli custom-fields set-endpoint`** - Set custom field values on an endpoint.

Scope: platform.custom_fields_values.write.
- **`connectwise-rmm-cli custom-fields set-site`** - Set custom field values on a site.

Scope: platform.custom_fields_values.write.

### device_groups

Manage device groups

- **`connectwise-rmm-cli device-groups`** - List all device groups (static, dynamic, predefined).

Scope: platform.deviceGroups.read.

### endpoints

Manage endpoints

- **`connectwise-rmm-cli endpoints create-agent-install-token`** - Get the agent install token for a company and site.

Exposed API for Generate Token. Body {clientID, siteID}; returns {token}. The token maps a newly installed agent to that company and site. Scope: platform.agent-token.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli endpoints get`** - Get full details (hardware, OS, agent) of one endpoint.

Scope: platform.asset.read.
- **`connectwise-rmm-cli endpoints get-cpu-usage`** - Get the latest CPU usage of an endpoint.

Latest CPU usage. Required scope is not stated in the public sources.
- **`connectwise-rmm-cli endpoints get-disk-usage`** - Get the latest disk usage of an endpoint.

Latest disk usage. Required scope is not stated in the public sources.
- **`connectwise-rmm-cli endpoints get-heartbeat`** - Check agent availability (online/offline, last seen) for up to 400 endpoints.

Agent Last Seen API. Query resourceType=endpoints and resources=<comma-separated IDs> (max 400 endpoints). Returns {status: success|partial_success, successfulRecords: [{clientID, siteID, endpoints: [{endpointID, DcDateTimeUTC, Availability}]}], failedRecords: [{resourceType, resource, error}]}. Rate limit 40 requests per 5 minutes. Scope: platform.devices.read.
- **`connectwise-rmm-cli endpoints get-memory-usage`** - Get the latest memory usage of an endpoint.

Latest memory usage; minute sets how many minutes of history to return. Required scope is not stated in the public sources.
- **`connectwise-rmm-cli endpoints get-services`** - List Windows services or daemons on one endpoint.

Service details for an endpoint; filter by serviceName. Required scope is not stated in the public sources.
- **`connectwise-rmm-cli endpoints get-summary`** - Fetch projected endpoint inventory for companies, sites, or endpoints (Device Summary API).

Device Summary API. Body {resourceType: company|site|endpoint, resources: [ids]} (max 4 companies, 10 sites, 500 endpoints). Query field= is a comma-separated projection (max 15), e.g. endpointID,deviceName,friendlyName,os,system; without it only endpointID, deviceName, friendlyName, resourceType and endpointType come back. Up to 500 records per page; page with limit and cursor. Only the platform category is supported today. Rate limit 100 per 10 minutes. ConnectWise University lists Origin and X-Request-ID as required headers; the CLI sends both on every request. Scope: platform.devices.read.
- **`connectwise-rmm-cli endpoints get-system-state`** - Get last boot time and logged-on users of an endpoint.

Scope: platform.devices.read.
- **`connectwise-rmm-cli endpoints list`** - List every managed endpoint for the partner.

Scope: platform.asset.read.
- **`connectwise-rmm-cli endpoints list-applications`** - List installed applications on a set of endpoints.

Software inventory. Body {resourceType: endpoints, resources: [endpoint IDs]}; filter by name and version; page with limit and cursor. Required scope is not stated in the public sources.
- **`connectwise-rmm-cli endpoints list-company`** - List the endpoints of one company.

Scope: platform.asset.read.
- **`connectwise-rmm-cli endpoints list-services`** - List endpoint services for the partner.

Scope: platform.asset.read.
- **`connectwise-rmm-cli endpoints list-site`** - List the endpoints of one site.

Scope: platform.asset.read.
- **`connectwise-rmm-cli endpoints uninstall`** - Uninstall (offboard) the agent from one or more endpoints.

Scope: platform.agent.delete.

### hypervisors

Manage hypervisors

- **`connectwise-rmm-cli hypervisors list-guests`** - List guest VMs of a hypervisor host.

Scope: platform.devices.read.
- **`connectwise-rmm-cli hypervisors list-hosts`** - List monitored hypervisor hosts.

Fetch hypervisors Hosts.

Scope: platform.devices.read.

### patches

Manage patches

- **`connectwise-rmm-cli patches get-endpoint`** - Get patch compliance summary and patch list for an endpoint.

Scope: platform.patching.read.
- **`connectwise-rmm-cli patches get-os-patch-compliance`** - Get OS patch compliance summary for a set of endpoints.

Bulk OS patch compliance. Body {resourceType: endpoints, resources: [ids]}. Page with limit and cursor. Scope: platform.ospatching.management.read.
- **`connectwise-rmm-cli patches get-os-patch-details`** - Get OS patch details for a set of endpoints.

Bulk OS patch details. Body {resourceType: endpoints, resources: [ids]}. Scope: platform.ospatching.management.read.
- **`connectwise-rmm-cli patches get-os-patch-inventory`** - Get the partner-wide OS patch inventory.

Every OS patch known across the partner. Scope: platform.ospatching.management.read.
- **`connectwise-rmm-cli patches get-third-party-patch-compliance`** - Get third-party (non-OS) patch compliance for a set of endpoints.

Bulk third-party application patch compliance. Body {resourceType: endpoints, resources: [ids]}. Page with limit and cursor. Scope: platform.tppatching.management.read.
- **`connectwise-rmm-cli patches get-third-party-patch-details`** - Get third-party (non-OS) patch details for a set of endpoints.

Bulk third-party application patch details. Body {resourceType: endpoints, resources: [ids]}. Page with limit and cursor. Scope: platform.tppatching.management.read.

### policies

Manage policies

- **`connectwise-rmm-cli policies get-effective-policy`** - Get the effective policy applied to an endpoint.

Scope: platform.policies.read.
- **`connectwise-rmm-cli policies get-effective-policy-override`** - Get policy overrides for an endpoint.

Scope: platform.policies.read.
- **`connectwise-rmm-cli policies list-policy-categories`** - List policy categories and subcategories.

Scope: platform.policies.read.

### rate_limits

Manage rate limits

- **`connectwise-rmm-cli rate-limits`** - Get current rate limit.

### scripts

Manage scripts

- **`connectwise-rmm-cli scripts list`** - List scripts created in the script editor (templateType fusionscript).

Scope: platform.automation.read.
- **`connectwise-rmm-cli scripts list-custom-templates`** - List custom task templates (templateType custom).

Scope: platform.automation.read.
- **`connectwise-rmm-cli scripts list-shell`** - List out-of-the-box global script templates (templateType script).

Scope: platform.automation.read.

### sites

Manage sites

- **`connectwise-rmm-cli sites create`** - Create a site under a company.

Scope: platform.sites.write.
- **`connectwise-rmm-cli sites get`** - Get a site by ID.

Scope: platform.sites.read.
- **`connectwise-rmm-cli sites list`** - List all sites across every company.

Scope: platform.sites.read.
- **`connectwise-rmm-cli sites list-company`** - List the sites of one company.

Scope: platform.sites.read.
- **`connectwise-rmm-cli sites update`** - Partially update a site.

Scope: platform.sites.write.

### tasks

Manage tasks

- **`connectwise-rmm-cli tasks get-instance-results`** - Get per-endpoint results of one task run.

Scope: platform.automation.read.
- **`connectwise-rmm-cli tasks get-instance-summary`** - Get the run history (instances) of a task.

Scope: platform.automation.read.
- **`connectwise-rmm-cli tasks list-endpoint-summaries`** - List tasks that ran on one endpoint.

Scope: platform.automation.read.
- **`connectwise-rmm-cli tasks list-summaries`** - List all scheduled tasks (the Scheduled Tasks page).

Scope: platform.automation.read.
- **`connectwise-rmm-cli tasks schedule`** - Schedule a script or template to run on target endpoints.

Scope: platform.automation.create.

### ticket_notes

Manage ticket notes

- **`connectwise-rmm-cli ticket-notes create`** - Add a note to a ticket.

This API creates a new note for the specified ticket in the system. The 'id' parameter value in the request body is ignored, if provided. A new 'id' is created  for the newly created note and provided in response body

Scope: platform.tickets.update (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli ticket-notes list`** - List notes on a ticket.

This API fetches the selected notes for a specific ticket from system and returns it.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).

### ticket_settings

Manage ticket settings

- **`connectwise-rmm-cli ticket-settings list-service-board-teams`** - List teams of a service board.

This API fetches all the team associations for a specific service-board.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli ticket-settings list-service-boards`** - List service boards.

This API returns all the available service-boards in the system, for partner with provided partnerId. Pagination support is not available for this API yet, so this API may return large amount of data depending upon serviceBoards present in the system.

The API accepts query parameters which should be provided to identify available service boards to assigns tickets to. 
As the CW Expert Service only support entitled companies and sites, providing companyId and siteId allows service to return only entitled service-boards.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli ticket-settings list-ticket-categories`** - List ticket categories.

This API returns all the ticket categories for a partner. Pagination support is not available for this API yet, so this API may return large amount of data depending upon categories present for a partner in the system.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli ticket-settings list-ticket-priorities`** - List ticket priorities.

This API returns all the available priorities in the system. Pagination support is not available for this API yet, so this API may return large amount of data depending upon priorities present in the system.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli ticket-settings list-ticket-sources`** - List ticket sources.

This API returns all the ticket sources for a partner. Pagination support is not available for this API yet, so this API may return large amount of data depending upon sources present for a partner in the system.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli ticket-settings list-ticket-statuses`** - List ticket statuses.

This API returns all the available statuses in the system. Pagination support is not available for this API yet, so this API may return large amount of data depending upon statuses present in the system.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli ticket-settings list-ticket-tags`** - List ticket tags.

This API returns all the available tags in the system, for partner with provided partnerId. Pagination support is not available for this API yet, so this API may return large amount of data depending upon tags present in the system.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli ticket-settings list-ticket-types`** - List ticket types.

This API returns all the available types in the system. Pagination support is not available for this API yet, so this API may return large amount of data depending upon types present in the system.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).

### tickets

Manage tickets

- **`connectwise-rmm-cli tickets create`** - Create a service ticket.

This API creates a new ticket in the system.
The 'read-only' parameters in the request body are ignored, if provided. See request body schema for details.

Scope: platform.tickets.create (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli tickets get`** - Get a service ticket.

Fetches the ticket stored in the system against the provided id.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli tickets get-counts`** - Get ticket counts.

This API returns ticket counts

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli tickets list`** - List service tickets (filterable; page by passing --page-size and --page-num together).

This API returns selected tickets in the service. Pagination and filtering support is available, see details below.

For support for Filtering, please note below strategy. 
In general, you can add as many filter criteria as required by adding those many query parameters. Each query parameter becomes one filter criterion 
and these are ultimately *AND*ed together (tickets matching all criteria are filtered). 
However there is a concept of group of criteria. Each filter query parameter supports (as indicated in description of parameter) an optional numeric suffix i.e 1, 2, etc.
The query parameters which shares a common suffix become part of same group.
This means ?number=123&statusIds=3a8e2efd-5b8a-4911-9824-8d897cf828d3&number1=3445&number2=55666 forms three groups of criteria as below

  - group 1(with no suffix): number = 123 AND statusId = 3a8e2efd-5b8a-4911-9824-8d897cf828d3
  - group 2(with suffix 1): number = 3445
  - group 3(with suffix 2): number = 55666

All the criteria in same group are *AND*ed and then groups themselves are *OR*ed. This allows almost any type of query formation while fitering the tickets.

Scope: platform.tickets.read (inferred from the published scope list; the operation does not state it).
- **`connectwise-rmm-cli tickets update`** - Update a service ticket with JSON Patch operations.

This API updates the existing ticket based on JSON Patch request received.
This API is suitable for changing specific fields of ticket without replacing the entire ticket object.
For more details on JSON Patch, please check http://jsonpatch.com/
The 'read-only' fields of ticket body cannot be updated using this API. For getting 'read-only' fields info, see ticket
schema details

Scope: platform.tickets.update (inferred from the published scope list; the operation does not state it).


### fleet


- **`connectwise-rmm-cli fleet stale`** - Every endpoint across all companies that is offline, missing from the heartbeat, or silent longer than a threshold, sorted by company and silence.

- **`connectwise-rmm-cli fleet patch-posture`** - Companies ranked by patch risk: endpoints below a compliance bar, missing patches, pending reboots, out of support, stale assessments.

- **`connectwise-rmm-cli fleet whois`** - Which endpoints a person is logged on to now or was last logged on to, with company and site names.

- **`connectwise-rmm-cli fleet brief`** - One card for one endpoint: online or last seen, logged-on users, patch counts, and tickets linked to it or its site.

- **`connectwise-rmm-cli fleet scorecard`** - Per-site health card for one company: endpoint mix, offline count, patch risk and tickets by status.

- **`connectwise-rmm-cli fleet task-failures`** - Failed endpoints from a script run grouped by identical error text, joined to endpoint names, with IDs ready to re-run.

- **`connectwise-rmm-cli fleet reboot-due`** - Endpoints with pending reboots split into reboot now, coordinate (user logged on or sessions unknown), offline, and availability unknown.




### run and scopes

- **`connectwise-rmm-cli run`** - Resolve a script and endpoints by name or ID and print the exact schedule request; `--confirm` schedules it, `--wait` polls and returns per-endpoint results.
- **`connectwise-rmm-cli scopes`** - Mint a token per candidate scope, report granted and denied scopes, and print the CONNECTWISE_RMM_OAUTH_SCOPE line to export.

### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`connectwise-rmm-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`connectwise-rmm-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`connectwise-rmm-cli learnings list`** - Inspect taught rows
- **`connectwise-rmm-cli learnings forget <query>`** - Undo a teach
- **`connectwise-rmm-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`connectwise-rmm-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`connectwise-rmm-cli teach-pattern`** - Install a query/resource template up front
- **`connectwise-rmm-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CONNECTWISE_RMM_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `connectwise-rmm-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
connectwise-rmm-cli companies list

# JSON for scripting and agents
connectwise-rmm-cli companies list --json
# Filter to specific fields
connectwise-rmm-cli companies list --json --select account,acquiredDate,activityStatus

# Dry run  -  show the request without sending
connectwise-rmm-cli companies list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
connectwise-rmm-cli companies list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply confirmation. Generated write commands (sites create/update, tickets create/update, ticket-notes create, custom-fields set-*, tasks schedule, endpoints uninstall, endpoints create-agent-install-token, import) run immediately, so preview them with `--dry-run`; `run` only schedules when `--confirm` is passed
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
connectwise-rmm-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `connectwise-rmm-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/connectwise-rmm-cli/config.toml` (`%USERPROFILE%\.config\connectwise-rmm-cli\config.toml` on Windows); `--home`, `CONNECTWISE_RMM_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CONNECTWISE_RMM_CLIENT_ID` | per_call | Yes | API client ID from Integrations > API Access; its prefix picks the region (0e30 NA, 0e31 EU, 0e32 AU). |
| `CONNECTWISE_RMM_CLIENT_SECRET` | per_call | Yes | API client secret paired with the client ID. |
| `CONNECTWISE_RMM_OAUTH_SCOPE` | per_call | No | Space-separated scopes to request; defaults to every scope this CLI uses. Get the exact value from `connectwise-rmm-cli scopes`. |
| `CONNECTWISE_RMM_BASE_URL` | per_call | No | Regional API host; overrides the client-ID region. |
| `CONNECTWISE_RMM_TOKEN_URL` | per_call | No | Token endpoint override; defaults to `<base_url>/v1/token`. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `connectwise-rmm-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `connectwise-rmm-cli doctor` to check credentials, and `connectwise-rmm-cli auth status` for the token source
- Verify both variables are set: `echo $CONNECTWISE_RMM_CLIENT_ID; test -n "$CONNECTWISE_RMM_CLIENT_SECRET" && echo secret set`
- Run `connectwise-rmm-cli scopes --agent` to find the scopes your key was granted
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **Token request fails with HTTP 400 or 401 right after creating the API key**  -  Run connectwise-rmm-cli scopes --agent and export the CONNECTWISE_RMM_OAUTH_SCOPE line it prints; one ungranted scope fails the whole token request
- **oauth app does not exist**  -  Check the client ID and secret; for EU or AU tenants whose client ID does not start with 0e31 or 0e32, export CONNECTWISE_RMM_BASE_URL=https://openapi.service.euplatform.connectwise.com (EU) or https://openapi.service.auplatform.connectwise.com (AU)
- **A fleet command returns an empty list**  -  Check scanned_endpoints and note in the output; run connectwise-rmm-cli sync --resources companies,sites,endpoints first
- **fleet results look partial on a large fleet (scan_cap_hit: true)**  -  Raise --max-endpoints (0 means no cap) or scope with --company
- **fleet brief says the machine name is not found**  -  Pass the endpoint ID, or run connectwise-rmm-cli fleet whois <any user> --company <company> once to cache machine names
- **Ticket counts look low**  -  sync stores the tickets the API returns without paging; page explicitly with tickets list --page-size 100 --page-num 2
- **HTTP 429 Too Many Requests**  -  Limits are per token and resource family; check the budget with connectwise-rmm-cli rate-limits and slow down
- **HTTP 403 on one command while others work**  -  The key lacks that command's scope (shown in its help as Scope:); grant it under Integrations > API Access, then rerun scopes

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**connectwise-asio-mcp**](https://github.com/MSPbotsAI/connectwise-asio-mcp)  -  Python
- [**asio-cli**](https://github.com/tfournet/asio-cli)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
