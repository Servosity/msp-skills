# Action1 CLI

**Every Action1 endpoint, plus the fleet-wide patch and vulnerability views the org-siloed API cannot give you.**

Action1's REST API is organization-siloed  -  every call is scoped to one org and nothing is kept over time. This CLI mirrors the full API, then fans out across all your organizations into a local SQLite store so you can rank the worst-patched endpoints fleet-wide (fleet patch-posture), triage CVEs by blast radius and CISA KEV status (fleet vuln-triage), find dark agents (fleet stale), and diff patch drift week over week (fleet patch-drift). Agent-native output, typed exit codes, and offline search throughout.

Learn more at [Action1](https://app.action1.com/support/).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `action1-cli` binary and the `pp-action1` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install action1
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install action1 --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install action1 --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install action1 --agent claude-code
npx -y @mvanhorn/printing-press-library install action1 --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/action1/cmd/action1-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/action1-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install action1 --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-action1 --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-action1 --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install action1 --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/action1-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ACTION1_OAUTH2` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/action1/cmd/action1-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "action1": {
      "command": "action1-mcp",
      "env": {
        "ACTION1_OAUTH2": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Action1 uses OAuth2 token-mint. Generate a Client ID and Client Secret on the API Credentials page in the Action1 console, then set ACTION1_CLIENT_ID and ACTION1_CLIENT_SECRET. The CLI POSTs them to /oauth2/token (JSON body) to obtain a bearer token automatically. Set ACTION1_ORG_ID to scope the fleet commands to one organization by default, and ACTION1_REGION (us/eu/au) to pick your data center. Per-organization API commands like 'endpoints managed <orgId>' take the organization id as their first argument.

## Quick Start

```bash
# confirm credentials mint a token and the API is reachable
action1-cli doctor

# list your organizations  -  the orgId most commands take
action1-cli organizations get --json

# pull endpoints, updates, vulnerabilities, and software into the local store, fanning out across orgs
action1-cli sync --full

# the worst-patched endpoints across the whole fleet
action1-cli fleet patch-posture --limit 25

# CVEs ranked by blast radius, known-exploited first
action1-cli fleet vuln-triage --kev-only

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-org views the API can't give you
- **`fleet patch-posture`**  -  See every endpoint across all your client organizations ranked by how many updates it is missing  -  one fleet-wide view.

  _Reach for this when an agent needs the worst-patched machines across the whole book of business, not one client at a time._

  ```bash
  action1-cli fleet patch-posture --agent --limit 25
  ```
- **`fleet vuln-triage`**  -  Rank CVEs across every organization by how many endpoints they hit, weighted by CVSS and the CISA Known-Exploited flag.

  _Pick this to answer 'what should we patch first across all clients' with exploit-aware prioritization._

  ```bash
  action1-cli fleet vuln-triage --kev-only --agent
  ```
- **`fleet software-rollup`**  -  Deduplicate installed software across the whole fleet into app name x version spread x install count.

  _Use this for license counting and version-spread audits across all clients at once._

  ```bash
  action1-cli fleet software-rollup --name "Google Chrome" --agent
  ```
- **`fleet automation-health`**  -  Success and failure rates across automation instances and their per-endpoint results, aggregated across organizations.

  _Use this to spot automations that are quietly failing on a subset of endpoints across clients._

  ```bash
  action1-cli fleet automation-health --agent
  ```
- **`fleet org-scorecard`**  -  One row per client organization: endpoint count, missing updates, open CVEs, KEV exposure, and stale agents  -  the per-client posture number MSPs report to account managers.

  _Reach for this when an agent needs a per-client posture summary across every organization, not per-endpoint detail._

  ```bash
  action1-cli fleet org-scorecard --agent
  ```
- **`fleet reboot-pending`**  -  Every endpoint fleet-wide where an installed update is waiting on a reboot to finish  -  the action queue that closes out a patch cycle.

  _Reach for this when an agent needs the concrete list of machines blocking patch-cycle completion, not a composite health score._

  ```bash
  action1-cli fleet reboot-pending --agent
  ```

### Fleet health & drift
- **`fleet stale`**  -  Surface endpoints that have not checked in for N days, or are offline, across all organizations.

  _Use this to find agents that silently stopped reporting before they become a coverage gap._

  ```bash
  action1-cli fleet stale --days 14 --agent
  ```
- **`fleet patch-drift`**  -  Diff two synced snapshots to show which updates were remediated and which newly appeared since last sync.

  _Reach for this to prove remediation progress week over week, or catch regressions._

  ```bash
  action1-cli fleet patch-drift --agent
  ```
- **`fleet health-score`**  -  A single composite score per endpoint from missing updates, open vulnerabilities, reboot-required, and staleness  -  fleet-ranked.

  _Pick this when you need a triage-ranked worst-to-best list of machines across the fleet._

  ```bash
  action1-cli fleet health-score --agent --limit 50
  ```

## Recipes


### Worst-patched endpoints, fleet-wide

```bash
action1-cli fleet patch-posture --agent --limit 25
```

Cross-org rollup of endpoints ranked by missing-update count.

### Known-exploited CVE triage

```bash
action1-cli fleet vuln-triage --kev-only --agent
```

CISA KEV CVEs across all orgs, ranked by affected endpoints and CVSS.

### Find dark agents

```bash
action1-cli fleet stale --days 14 --agent
```

Endpoints that stopped checking in over two weeks ago across the fleet.

### Narrow a verbose endpoint list

```bash
action1-cli endpoints managed 00000000-0000-0000-0000-000000000000 --agent --select items.name,items.OS,items.last_seen,items.online_status
```

orgId is positional (00000000-... = all organizations); dotted --select trims the large managed-endpoint payload to just the fields an agent needs.

### Prove remediation progress

```bash
action1-cli fleet patch-drift --agent
```

What got patched and what newly appeared since the previous sync.

## Usage

Run `action1-cli --help` for the full command reference and flag list.

## Commands

### audit

Manage audit

- **`action1-cli audit events-get`** - **Requires permission: `view_audit`**

'The Audit Trail contains event records associated with user actions, such as logins, remote access, changes to configuration, and API usage.'
- **`action1-cli audit events-id-get`** - **Requires permission: `view_audit`**

Get a specific audit record by its ID.
- **`action1-cli audit export-get`** - **Requires permission: `view_audit`**

Exports audit data. Use parameters to filter out exported data.

The only currently supported format is 'csv'.

### automations

Manage automations

- **`action1-cli automations actions-templates-get`** - Gets a list of existing action templates.
- **`action1-cli automations actions-templates-template-id-get`** - Gets details for an action template specified by its ID.
- **`action1-cli automations policies-instances-org-id-get`** - **Requires permission: `view_automations`**

Gets a list of running and completed automations for the specified organization.
- **`action1-cli automations policies-instances-org-id-id-get`** - **Requires permission: `view_automations`**

Gets details about a automation instance specified by its ID.
- **`action1-cli automations policies-instances-org-id-instance-id-endpoint-results-endpoint-id-details-get`** - **Requires permission: `view_automations`**

Gets details about the automation instance applied to an endpoint specified by its ID.
- **`action1-cli automations policies-instances-org-id-instance-id-endpoint-results-get`** - **Requires permission: `view_automations`**

Gets a list of endpoints where the automation instance is being applied or has been executed.
- **`action1-cli automations policies-instances-org-id-instance-id-stop-post`** - **Requires permission: `manage_automations`**

Stops applying a automation instance and aborts actions running on a remote endpoint.
- **`action1-cli automations policies-instances-org-id-post`** - **Requires permission: `run_automations`**

Immediately runs an instance of the automation on the specified endpoints in the specified organization. This call will fail if 'orgId' is set to 'all'.
- **`action1-cli automations policies-schedules-org-id-get`** - **Requires permission: `view_automations`**

Lists existing scheduled automations. Use parameters to filter out automations in the returned results.
- **`action1-cli automations policies-schedules-org-id-id-actions-action-id-delete`** - **Requires permission: `manage_automations`**

Deletes a specified action from a scheduled automation.
- **`action1-cli automations policies-schedules-org-id-id-delete`** - **Requires permission: `manage_automations`**

Deletes a scheduled automation specified by its ID.
- **`action1-cli automations policies-schedules-org-id-id-deployment-statuses-get`** - **Requires permission: `view_automations`**

Gets the deployment statuses of a automation specified by its ID.
- **`action1-cli automations policies-schedules-org-id-id-get`** - **Requires permission: `view_automations`**

Gets a scheduled automation specified by its ID.
- **`action1-cli automations policies-schedules-org-id-id-patch`** - **Requires permission: `manage_automations`**

Modifies the automation schedule settings and parameters of one of the actions included in the automation.
- **`action1-cli automations policies-schedules-org-id-post`** - **Requires permission: `manage_automations`**

Schedules a new automation. The automation will be executed against a specified list of endpoints in the specified organization.

### cve-descriptions

Manage cve descriptions

- **`action1-cli cve-descriptions <cveId>`** - **Requires permission: `view_vulnerabilities`**

Retrieves detailed information about a specific vulnerability in general, not within a context of any organization. 

This includes the Common Vulnerabilities and Exposures (CVE) details such as the CVSS score, attack vector, severity, and other relevant attributes. 

This information provides a comprehensive overview of the vulnerability, aiding in understanding its nature and potential impact.

### data-sources

A data source is a scripting template that queries certain endpoint data, such as disk volume or local group members, and presents it in a structured way consumed by reports or alerts. Data sources can be built-in or custom (defined by 'builtin' parameter). Built-in data sources are maintained by Action1 and cannot be modified. Custom data sources can be created and maintained by the user.

- **`action1-cli data-sources all-post`** - **Requires permission: `manage_data_sources`**

Creates a new custom data source. At this time, all data sources are enterprise-wide.
- **`action1-cli data-sources org-id-get`** - **Requires one of the following permissions: `manage_data_sources`, `manage_reports`**

Gets a list of existing data sources. To filter out built-in data sources, set the 'builtin' parameter to 'yes'. For custom, set 'builtin' to 'no'.
- **`action1-cli data-sources org-id-id-delete`** - **Requires permission: `manage_data_sources`**

Deletes an existing custom data source.

Note that you cannot remove built-in data sources.

At this time, all data sources are enterprise-wide. The 'orgId' has to be set to 'all'.
- **`action1-cli data-sources org-id-id-get`** - **Requires one of the following permissions: `manage_data_sources`, `manage_reports`**

Gets details about a specific data source. At this time, all data sources are enterprise-wide. The 'orgId' has to be set to 'all'.
- **`action1-cli data-sources org-id-id-patch`** - **Requires permission: `manage_data_sources`**

Updates a custom data source specified by its ID. At this time, all data sources are enterprise-wide. The 'orgId' has to be set to 'all'.

### endpoints

The endpoint object represents a server, workstation, or other device managed by Action1 within a specific organization. Endpoints can only belong to one Action1 organization at one time, but can be moved between organizations.

- **`action1-cli endpoints agent-installation`** - **Requires permission: `manage_endpoints`**

In order to manage an endpoint, the Action1 agent needs to be installed on it first. Agent binary files are specific to each organization. Use this call to obtain a URL to download the agent installation file for a specific organization and agent type.
- **`action1-cli endpoints connector-installation-org-id-windows-exe-get`** - **Requires permission: `manage_endpoints`**

Obtains an URL to download the Deployer installation file. The Action1 Deployer installation files are specific to each organization. The Deployer services currently run on Windows only.
- **`action1-cli endpoints connectors-org-id-deployer-id-delete`** - **Requires permission: `manage_endpoints`**

In rare cases it is impossible to uninstall the Action1 Deployer (a computer it was deployed on was removed from the organization, OS reinstalled, etc.).

Use this call to delete the Deployer object without uninstalling the service.

Fails if the Deployer status is not 'Pending Install' (you must try to uninstall the Deployer first via POST '/API/endpoints/deployers/{org-id}/{deployer-id}/uninstall)''.
- **`action1-cli endpoints connectors-org-id-deployer-id-get`** - **Requires permission: `manage_endpoints`**

Obtains the current information about the specified Action1 Deployer service.
- **`action1-cli endpoints connectors-org-id-get`** - **Requires permission: `manage_endpoints`**

Lists all Action1 Deployer services in the specified organization. Use parameters to filter returned results.
- **`action1-cli endpoints discovery-org-id-get`** - **Requires permission: `manage_endpoints`**

Obtains the current Agent Deployment settings in a specified organization. If 'orgId' is to 'all', the call will fail.
- **`action1-cli endpoints discovery-org-id-patch`** - **Requires permission: `manage_endpoints`**

Updates the Agent Deployment configuration for a specific organization.
- **`action1-cli endpoints groups`** - **Requires permission: `manage_endpoints`**

Creates a new endpoint group within the specified organization. ('include_filter':[…] and 'exclude_filter':[…] -- can be omitted upon creation.)
- **`action1-cli endpoints groups-group-id`** - **Requires permission: `view_endpoints`**

Gets a specific endpoint group by its ID.
- **`action1-cli endpoints groups-group-id-contents-get`** - **Requires permission: `view_endpoints`**

Lists all endpoints included in the specified group. Use parameters to filter the returned results.
- **`action1-cli endpoints groups-group-id-contents-post`** - **Requires permission: `manage_endpoints`**

Joins endpoints with specified IDs or names to the endpoint group.

Removes endpoints with specific IDs.

If the specified endpoint is already in the group via filters, its 'added_via' attribute will be updated to 'manual' (from 'criteria') and the call will succeed.
- **`action1-cli endpoints groups-group-id-delete`** - **Requires permission: `manage_endpoints`**

Deletes an existing group in the specified organization.
- **`action1-cli endpoints groups-group-id-patch`** - **Requires permission: `manage_endpoints`**

Changes settings for an existing endpoint group in the specified organization. If
certain fields are not specified in the object you submit, they won't get changed. I.e., you can just change the group name without changing the group filters.
- **`action1-cli endpoints groups-org-id`** - **Requires permission: `view_endpoints`**

Lists existing endpoint groups. Use filters and narrow down and sort the returned results.
- **`action1-cli endpoints managed`** - **Requires permission: `view_endpoints`**

Lists all endpoints within the organization. Use filters to narrow down returned results.
- **`action1-cli endpoints managed-id`** - **Requires permission: `view_endpoints`**

Obtains current information about the endpoint with the specified ID.
- **`action1-cli endpoints managed-id-delete`** - **Requires permission: `manage_endpoints`**

Removes a specified endpoint and attempts to uninstall its agent.
- **`action1-cli endpoints managed-id-missing-updates`** - **Requires permission: `view_endpoints`**

Obtains a list of missing software updates for a specific endpoint. Use filters to narrow down the returned results.
- **`action1-cli endpoints managed-id-move`** - **Requires permission: `manage_endpoints`**

Moves the endpoint to another organization.
- **`action1-cli endpoints managed-id-patch`** - **Requires permission: `manage_endpoint_attributes`**

Changes the user-defined 'comment', 'name' and custom attributes for the specified endpoint.
- **`action1-cli endpoints managed-id-remote-sessions-post`** - **Requires permission: `remote_connect`**

Sends a request to the endpoint to start a new remote session.

After requesting to open a remote session, use 'GET /endpoints/managed/{orgId}/{endpointId}/remote_sessions/{sessionId}' again until 'connected' = 'yes'.
- **`action1-cli endpoints managed-id-remote-sessions-session-id-get`** - **Requires permission: `remote_connect`**

Gets details for an existing remote session specified by ID.
- **`action1-cli endpoints managed-id-remote-sessions-session-id-patch`** - **Requires permission: `remote_connect`**

Changes the 'current_monitor' parameter for a specific remote session.
- **`action1-cli endpoints status`** - Retrieves information if any endpoints were added to an organization.

### enterprise

Manage enterprise

- **`action1-cli enterprise get`** - Gets settings for an enterprise. You can query data for your enterprise only.
- **`action1-cli enterprise patch`** - **Requires permission: `manage_enterprise`**

Updates settings for a current enterprise. You can only access your enterprise.
- **`action1-cli enterprise request-closure`** - **Requires permission: `manage_enterprise`**

The request is available only for a free Action1 account.

If need to close an enterprise with a paid subscription, please contact your Action1 representative.

The Action1 enterprise associated with your account will enter a 30 day period of pending closure. Following the 30-day period, we will attempt to uninstall any remaining agents in your Action1 enterprise, and all your data will be permanently erased from Action1 cloud servers.
- **`action1-cli enterprise revoke-closure`** - **Requires permission: `manage_enterprise`**

The Action1 enterprise will be immediately reactivated and all your data will be retained.

### installed-software

Manage installed software


### logs

Manage logs

- **`action1-cli logs <orgId>`** - **Requires permission: `manage_endpoints`**

Gets diagnostic logs. Use parameters to pre-filter returned results. For example, get the first 50 log records of the level "normal" and higher, sorted by time in descending order.

### me

Manage me

- **`action1-cli me me`** - Gets settings for the currently authenticated user.
- **`action1-cli me patch`** - Updates settings for the currently authenticated user.
Only 'timezone' and 'session_timeout' settings are currently updatable.
- **`action1-cli me subscriptions-get`** - Gets a list of report subscriptions.
- **`action1-cli me subscriptions-post`** - **Requires permission: `view_reports`**

Creates a new report subscription.
- **`action1-cli me subscriptions-subscription-id-delete`** - Removes the report subscription.
- **`action1-cli me subscriptions-subscription-id-patch`** - **Requires permission: `view_reports`**

Updates the report subscription.

### oauth2

Manage oauth2

- **`action1-cli oauth2`** - Generates a token. Provide valid API credentials (Client ID and Client Secret) which are configured in API credentials page in Action1 console.

### organizations

Manage organizations

- **`action1-cli organizations get`** - Gets a list of organizations within the current Action1 enterprise.
- **`action1-cli organizations org-id-delete`** - **Requires permission: `manage_organizations`**

Removes an organization from the enterprise.

This method will fail if a specified organization still contains endpoints or if it is the last organization in the enterprise.
- **`action1-cli organizations org-id-patch`** - **Requires permission: `manage_organizations`**

Updates settings for an organization specified by its ID.
- **`action1-cli organizations post`** - **Requires permission: `manage_organizations`**

Creates a new organization. When a new organization is created, all the default roles are automatically created for it.

### permissions

Manage permissions

- **`action1-cli permissions`** - Gets a list of available permission templates.

### remote_search

Manage remote search

- **`action1-cli remote-search <orgId>`** - **Requires permissions: `view_endpoints`, `view_software_repository`, `view_reports`**

Searches for Action1 objects that contain a phrase specified in the 'query' parameter in one of their fields. The search returns up to 10 objects within the specified organization.

### reportdata

Manage reportdata


### reports

Manage reports

- **`action1-cli reports all-get`** - Gets a list of existing reports.

At this time all reports are enterprise-wide.
- **`action1-cli reports org-id-category-id-get`** - Gets a list of reports and categories.

At this time all reports are enterprise-wide.
- **`action1-cli reports org-id-custom-id-delete`** - **Requires permission: `manage_reports`**

Deletes a custom report specified by its ID.

You cannot delete built-in reports.
- **`action1-cli reports org-id-custom-id-patch`** - **Requires permission: `manage_reports`**

Updates a custom report. You cannot changes a custom report's category.

Custom reports are enterprise-wide.
- **`action1-cli reports org-id-custom-post`** - **Requires permission: `manage_reports`**

Creates a custom report in the predefined Custom report category. You cannot create custom reports in other categories.

Custom reports are based on the data source. To learn more about data sources, see '/data-sources/*' calls.

Custom reports are enterprise-wide.

### roles

Manage roles

- **`action1-cli roles get`** - **Requires permission: `manage_roles`**

Gets a list of available roles.
- **`action1-cli roles id-delete`** - **Requires permission: `manage_roles`**

Deletes a role specified by its ID.
- **`action1-cli roles id-get`** - **Requires permission: `manage_roles`**

Gets details about a role specified by its ID.
- **`action1-cli roles id-patch`** - **Requires permission: `manage_roles`**

Updates a role specified by its ID.
- **`action1-cli roles post`** - **Requires permission: `manage_roles`**

Creates a new role.

### scripts

Manage scripts

- **`action1-cli scripts org-id-get`** - **Requires permission: `use_scripts`**

Gets a list of existing scripts from the Script Library. To filter out built-in scripts, set the 'builtin' parameter to 'yes'. For custom, set 'builtin' to 'no'.

At this time, all scripts are enterprise-wide. The 'orgId' has to be set to 'all'.
- **`action1-cli scripts org-id-id-delete`** - **Requires permission: `manage_scripts`**

Deletes an existing custom script specified by its ID.

At this time, all scripts are enterprise-wide.
- **`action1-cli scripts org-id-id-get`** - **Requires permission: `use_scripts`**

Gets details for a script specified by its ID.

At this time, all scripts are enterprise-wide.
- **`action1-cli scripts org-id-id-patch`** - **Requires permission: `manage_scripts`**

Updates details for an existing custom script specified by its ID.

At this time, all scripts are enterprise-wide.

The 'params' array (script parameters) included in the returned object is a read-only collection that is derived from the script text.
- **`action1-cli scripts org-id-post`** - **Requires permission: `manage_scripts`**

Creates a new custom script and adds it to the Script Library.

At this time, all scripts are enterprise-wide, The 'orgId' shall be set to 'all'. In the future, the organization-specific scripts will be supported.

The 'params' array (script parameters) included in the returned object is a read-only collection that is derived from the script text.

### setting-templates

Manage setting templates

- **`action1-cli setting-templates org-id-get`** - Gets a list of existing setting templates. Setting templates are maintained by Action1 and cannot be modified.

At this time, all setting templates are enterprise-wide, even when applied to a specific organization only.
- **`action1-cli setting-templates org-id-template-id-get`** - Gets details about a setting template specified by its ID. Setting templates are maintained by Action1 and cannot be modified.

At this time, all setting templates are enterprise-wide.

### settings

Manage settings

- **`action1-cli settings org-id-get`** - **Requires permission: `manage_advanced_settings`**

Lists all existing settings.

At this time, all settings are enterprise-wide.
- **`action1-cli settings org-id-id-delete`** - **Requires permission: `manage_advanced_settings`**

Deletes an existing setting specified by its ID.

At this time, all settings are enterprise-wide.
- **`action1-cli settings org-id-id-get`** - **Requires permission: `manage_advanced_settings`**

Gets details about the setting configuration.

At this time, all settings are enterprise-wide, but the scope of application can be limited to per organization or group.
- **`action1-cli settings org-id-id-patch`** - **Requires permission: `manage_advanced_settings`**

Updates an existing setting specified by its ID.

At this time, all settings are enterprise-wide.
- **`action1-cli settings org-id-post`** - **Requires permission: `manage_advanced_settings`**

Creates a new setting.

At this time, all setting templates are enterprise-wide.

### software-repository

Action1 Software Repository is a continuously updated private application repository that hosts the latest versions of all supported applications to keep your endpoints secure and patched. Action1 maintains built-in software repository packages that are available to all customers as well as enables users to create private software packages to be shared within their organizations only. The package object is a container of software versions and initially, it should contain the first available version of the software (child object called "version"). New versions are added continuously as the respective vendor releases them. Each version object contains the software binary setup files in MSI, EXE or ZIP format (up to 32Gb in size) and deployment settings, such as silent install switches. It may also include the additional actions to be executed before or after a software installation or uninstallation (e.g., reboot or run a script).

- **`action1-cli software-repository packages-all-get`** - **Requires permission: `view_software_repository`**

Gets a list of Software Repository packages for the entire enterprise. Use parameters to filter out packages in the returned results.
- **`action1-cli software-repository packages-all-package-id-delete`** - **Requires permission: `manage_software_repository`**

Deletes a custom Software Repository package specified by its ID.

Built-in Software Repository packages cannot be deleted. This API will return an error if you attempt to send a DELETE request for a built-in package.
- **`action1-cli software-repository packages-all-package-id-get`** - **Requires permission: `view_software_repository`**

Gets details for a Software Repository package specified by its ID. Set the "fields" parameter to "*" or "versions" to include the non-default "versions" container of children objects. Example: /software-repository/{orgId}/{Adobe_Adobe_Acrobat_Reader_DC_1570155380192_builtin}?fields=*
- **`action1-cli software-repository packages-all-package-id-patch`** - **Requires permission: `manage_software_repository`**

Modifies one or more properties of a custom Software Repository package, including contents of the child "version" objects.

Built-in packages cannot be modified except for 'EULA_accepted' setting. This method will return an error if you attempt to send a PATCH request for a built-in package and specify any properties except for 'EULA_accepted'.

The 'version.additional_actions.name' property shall always be present if the parameters of an additional action are to be modified. To change the 'name' parameter of an additional action, the action needs to be deleted (DELETE method) and then added back with the new name (PATCH request).
- **`action1-cli software-repository packages-all-post`** - **Requires permission: `manage_software_repository`**

Create a new custom Software Repository package object (with no versions) and set its initial basic properties. To create package versions, use POST /software-repository/{orgId}/{packageId}/versions method.

### subscription

View Action licenses, start or extend a free trial, or request a price quote.

- **`action1-cli subscription license-enterprise-get`** - **Requires permission: `manage_enterprise`**

Gets details about the enterprise license. You can get only get information about your enterprise.
- **`action1-cli subscription license-enterprise-quote-post`** - **Requires permission: `manage_enterprise`**

Sends a quote request to the Action1 Sales department. The actual quote request will be reviewed by someone in Sales and a quote will be sent via email.
- **`action1-cli subscription license-enterprise-trial-post`** - **Requires permission: `manage_enterprise`**

Sends a request to start a free trial for a current user or requests a trial extension for an expired or exceeded trial.
- **`action1-cli subscription license-usage-enterprise-get`** - **Requires permission: `manage_enterprise`**

Gets details about license usage for the entire enterprise.
- **`action1-cli subscription license-usage-organizations-get`** - **Requires permission: `manage_organizations`**

Gets details about license usage with statistics for each organization individually.
- **`action1-cli subscription license-usage-organizations-org-id-get`** - **Requires permission: `manage_organizations`**

Gets details about license usage for an organization specified by its ID.

### updates

Manage updates

- **`action1-cli updates org-id-get`** - **Requires one of the following permissions: `approve_updates`, `view_dashboards`, `manage_automations`**

Gets a list of all missing updates. Use parameters to filter out updates in the returned results.
- **`action1-cli updates org-id-package-id-get`** - **Requires one of the following permissions: `approve_updates`, `view_dashboards`, `manage_automations`**

Gets a list of updates available for a package specified by its ID. Use parameters to filter out updates in the returned results.

### users

Manage users

- **`action1-cli users get`** - **Requires permission: `view_users`**

Gets a list of users within the current Action1 enterprise.
- **`action1-cli users id-delete`** - **Requires permission: `manage_users`**

Deletes an existing user.
- **`action1-cli users id-get`** - **Requires permission: `view_users`**

Gets an existing user.
- **`action1-cli users id-patch`** - **Requires permission: `manage_users`**

Updates an existing user.
- **`action1-cli users post`** - **Requires permission: `manage_users`**

Creates a new user.

To create an SSO user, please first review the following documentation:
- EntraId: https://www.action1.com/documentation/sso-authentication-with-entra-id/
- Duo: https://www.action1.com/documentation/sso-authentication-with-duo/
- Google: https://www.action1.com/documentation/sso-authentication-with-google/
- Okta: https://www.action1.com/documentation/sso-authentication-with-google/

### vulnerabilities

Manage vulnerabilities

- **`action1-cli vulnerabilities org-id-cve-id-get`** - **Requires permission: `view_vulnerabilities`**

Retrieves detailed information about a specific vulnerability as it exists in the context of a specific organization, including the Common Vulnerabilities and Exposures (CVE) details such as the CVSS score, attack vector, severity, and other relevant attributes. 

This information provides a comprehensive overview of the vulnerability, aiding in understanding its nature and potential impact.
- **`action1-cli vulnerabilities org-id-get`** - **Requires permission: `view_vulnerabilities`**

Retrieves a comprehensive list of all vulnerable software installed on the organization's managed endpoints. 

This includes information about the specific vulnerabilities, affected versions, and affected endpoints. The data provides insights into potential risks, enabling targeted remediation efforts.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
action1-cli enterprise get

# JSON for scripting and agents
action1-cli enterprise get --json

# Filter to specific fields
action1-cli enterprise get --json --select id,name,status

# Dry run  -  show the request without sending
action1-cli enterprise get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
action1-cli enterprise get --agent
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
action1-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/action1-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ACTION1_OAUTH2` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `action1-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `action1-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ACTION1_OAUTH2`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 or token mint fails**  -  Check ACTION1_CLIENT_ID and ACTION1_CLIENT_SECRET come from the API Credentials page (not your login); run action1-cli doctor.
- **Empty results from an org-scoped command**  -  Per-org commands take the orgId as the first argument (from organizations get); fleet views read the local store, so run sync first.
- **fleet commands return []**  -  Run action1-cli sync --full first  -  fleet views read the local store.
- **Wrong data center**  -  Set ACTION1_REGION to us, eu, or au (or ACTION1_BASE_URL) to match your console URL.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**PSAction1**](https://github.com/Action1Corp/PSAction1)  -  PowerShell
- [**Action1MCP**](https://github.com/ghively/Action1MCP)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
