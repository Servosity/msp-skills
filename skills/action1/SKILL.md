---
name: action1
description: "Every Action1 endpoint, plus the fleet-wide patch and vulnerability views the org-siloed API cannot give you. Trigger phrases: `action1 patch posture`, `which endpoints are missing patches`, `triage action1 vulnerabilities`, `find stale action1 agents`, `action1 fleet view across organizations`, `use action1`, `run action1-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Action1"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - action1-cli
    install:
      - kind: script
        bins: [action1-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.ps1
---

# Action1 - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `action1-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.ps1 | iex
   ```
3. Verify: `action1-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `action1-cli` and `action1-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Action1's REST API is organization-siloed  -  every call is scoped to one org and nothing is kept over time. This CLI mirrors the full API, then fans out across all your organizations into a local SQLite store so you can rank the worst-patched endpoints fleet-wide (fleet patch-posture), triage CVEs by blast radius and CISA KEV status (fleet vuln-triage), find dark agents (fleet stale), and diff patch drift week over week (fleet patch-drift). Agent-native output, typed exit codes, and offline search throughout.

## When to Use This CLI

Use this CLI when an agent or technician needs to query Action1 patch, vulnerability, endpoint, software, automation, or report data  -  especially across multiple client organizations at once, offline, or over time. It is the right tool for fleet-wide patch posture, exploit-aware CVE triage, dark-agent detection, and patch-drift reporting that the org-scoped Action1 API cannot answer directly. All fleet commands read the local SQLite store; run sync --full first or they return empty results.

## Anti-triggers

Do not use this CLI for:
- Do not use fleet commands for one client's live data  -  use the per-org commands (e.g. endpoints managed <orgId>, vulnerabilities list <orgId>) which call the API directly.
- Do not use fleet vuln-triage for a single CVE's detail  -  use cve-descriptions <cveId>.
- Do not run fleet commands before a sync  -  they read the local store and return [] until 'sync --full' has populated it.
- Do not use this CLI to remediate or deploy patches interactively in real time  -  launch automations via the automations commands and check results afterwards; there is no live remote-control surface beyond what the Action1 API exposes.

## Unique Capabilities

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

## Command Reference

**audit**  -  Manage audit

- `action1-cli audit events-get`  -  **Requires permission: `view_audit`** 'The Audit Trail contains event records associated with user actions
- `action1-cli audit events-id-get`  -  **Requires permission: `view_audit`** Get a specific audit record by its ID.
- `action1-cli audit export-get`  -  **Requires permission: `view_audit`** Exports audit data. Use parameters to filter out exported data.

**automations**  -  Manage automations

- `action1-cli automations actions-templates-get`  -  Gets a list of existing action templates.
- `action1-cli automations actions-templates-template-id-get`  -  Gets details for an action template specified by its ID.
- `action1-cli automations policies-instances-org-id-get`  -  **Requires permission
- `action1-cli automations policies-instances-org-id-id-get`  -  **Requires permission: `view_automations`** Gets details about a automation instance specified by its ID.
- `action1-cli automations policies-instances-org-id-instance-id-endpoint-results-endpoint-id-details-get`  -  **Requires permission
- `action1-cli automations policies-instances-org-id-instance-id-endpoint-results-get`  -  **Requires permission
- `action1-cli automations policies-instances-org-id-instance-id-stop-post`  -  **Requires permission
- `action1-cli automations policies-instances-org-id-post`  -  **Requires permission
- `action1-cli automations policies-schedules-org-id-get`  -  **Requires permission: `view_automations`** Lists existing scheduled automations.
- `action1-cli automations policies-schedules-org-id-id-actions-action-id-delete`  -  **Requires permission: `manage_automations`** Deletes a specified action from a scheduled automation.
- `action1-cli automations policies-schedules-org-id-id-delete`  -  **Requires permission: `manage_automations`** Deletes a scheduled automation specified by its ID.
- `action1-cli automations policies-schedules-org-id-id-deployment-statuses-get`  -  **Requires permission: `view_automations`** Gets the deployment statuses of a automation specified by its ID.
- `action1-cli automations policies-schedules-org-id-id-get`  -  **Requires permission: `view_automations`** Gets a scheduled automation specified by its ID.
- `action1-cli automations policies-schedules-org-id-id-patch`  -  **Requires permission
- `action1-cli automations policies-schedules-org-id-post`  -  **Requires permission: `manage_automations`** Schedules a new automation.

**cve-descriptions**  -  Manage cve descriptions

- `action1-cli cve-descriptions <cveId>`  -  **Requires permission: `view_vulnerabilities`** Retrieves detailed information about a specific vulnerability in general

**data-sources**  -  A data source is a scripting template that queries certain endpoint data, such as disk volume or local group members, and presents it in a structured way consumed by reports or alerts. Data sources can be built-in or custom (defined by 'builtin' parameter). Built-in data sources are maintained by Action1 and cannot be modified. Custom data sources can be created and maintained by the user.

- `action1-cli data-sources all-post`  -  **Requires permission: `manage_data_sources`** Creates a new custom data source.
- `action1-cli data-sources org-id-get`  -  **Requires one of the following permissions: `manage_data_sources`
- `action1-cli data-sources org-id-id-delete`  -  **Requires permission: `manage_data_sources`** Deletes an existing custom data source.
- `action1-cli data-sources org-id-id-get`  -  **Requires one of the following permissions: `manage_data_sources`
- `action1-cli data-sources org-id-id-patch`  -  **Requires permission: `manage_data_sources`** Updates a custom data source specified by its ID.

**endpoints**  -  The endpoint object represents a server, workstation, or other device managed by Action1 within a specific organization. Endpoints can only belong to one Action1 organization at one time, but can be moved between organizations.

- `action1-cli endpoints agent-installation`  -  **Requires permission: `manage_endpoints`** In order to manage an endpoint
- `action1-cli endpoints connector-installation-org-id-windows-exe-get`  -  **Requires permission: `manage_endpoints`** Obtains an URL to download the Deployer installation file.
- `action1-cli endpoints connectors-org-id-deployer-id-delete`  -  **Requires permission
- `action1-cli endpoints connectors-org-id-deployer-id-get`  -  **Requires permission
- `action1-cli endpoints connectors-org-id-get`  -  **Requires permission: `manage_endpoints`** Lists all Action1 Deployer services in the specified organization.
- `action1-cli endpoints discovery-org-id-get`  -  **Requires permission: `manage_endpoints`** Obtains the current Agent Deployment settings in a specified organization.
- `action1-cli endpoints discovery-org-id-patch`  -  **Requires permission: `manage_endpoints`** Updates the Agent Deployment configuration for a specific organization.
- `action1-cli endpoints groups`  -  **Requires permission: `manage_endpoints`** Creates a new endpoint group within the specified organization.
- `action1-cli endpoints groups-group-id`  -  **Requires permission: `view_endpoints`** Gets a specific endpoint group by its ID.
- `action1-cli endpoints groups-group-id-contents-get`  -  **Requires permission: `view_endpoints`** Lists all endpoints included in the specified group.
- `action1-cli endpoints groups-group-id-contents-post`  -  **Requires permission: `manage_endpoints`** Joins endpoints with specified IDs or names to the endpoint group.
- `action1-cli endpoints groups-group-id-delete`  -  **Requires permission: `manage_endpoints`** Deletes an existing group in the specified organization.
- `action1-cli endpoints groups-group-id-patch`  -  **Requires permission
- `action1-cli endpoints groups-org-id`  -  **Requires permission: `view_endpoints`** Lists existing endpoint groups.
- `action1-cli endpoints managed`  -  **Requires permission: `view_endpoints`** Lists all endpoints within the organization.
- `action1-cli endpoints managed-id`  -  **Requires permission: `view_endpoints`** Obtains current information about the endpoint with the specified ID.
- `action1-cli endpoints managed-id-delete`  -  **Requires permission: `manage_endpoints`** Removes a specified endpoint and attempts to uninstall its agent.
- `action1-cli endpoints managed-id-missing-updates`  -  **Requires permission: `view_endpoints`** Obtains a list of missing software updates for a specific endpoint.
- `action1-cli endpoints managed-id-move`  -  **Requires permission: `manage_endpoints`** Moves the endpoint to another organization.
- `action1-cli endpoints managed-id-patch`  -  **Requires permission: `manage_endpoint_attributes`** Changes the user-defined 'comment'
- `action1-cli endpoints managed-id-remote-sessions-post`  -  **Requires permission: `remote_connect`** Sends a request to the endpoint to start a new remote session.
- `action1-cli endpoints managed-id-remote-sessions-session-id-get`  -  **Requires permission: `remote_connect`** Gets details for an existing remote session specified by ID.
- `action1-cli endpoints managed-id-remote-sessions-session-id-patch`  -  **Requires permission: `remote_connect`** Changes the 'current_monitor' parameter for a specific remote session.
- `action1-cli endpoints status`  -  Retrieves information if any endpoints were added to an organization.

**enterprise**  -  Manage enterprise

- `action1-cli enterprise get`  -  Gets settings for an enterprise. You can query data for your enterprise only.
- `action1-cli enterprise patch`  -  **Requires permission: `manage_enterprise`** Updates settings for a current enterprise.
- `action1-cli enterprise request-closure`  -  **Requires permission: `manage_enterprise`** The request is available only for a free Action1 account.
- `action1-cli enterprise revoke-closure`  -  **Requires permission

**installed-software**  -  Manage installed software


**logs**  -  Manage logs

- `action1-cli logs <orgId>`  -  **Requires permission: `manage_endpoints`** Gets diagnostic logs. Use parameters to pre-filter returned results.

**me**  -  Manage me

- `action1-cli me me`  -  Gets settings for the currently authenticated user.
- `action1-cli me patch`  -  Updates settings for the currently authenticated user.
- `action1-cli me subscriptions-get`  -  Gets a list of report subscriptions.
- `action1-cli me subscriptions-post`  -  **Requires permission: `view_reports`** Creates a new report subscription.
- `action1-cli me subscriptions-subscription-id-delete`  -  Removes the report subscription.
- `action1-cli me subscriptions-subscription-id-patch`  -  **Requires permission: `view_reports`** Updates the report subscription.

**oauth2**  -  Manage oauth2

- `action1-cli oauth2`  -  Generates a token.

**organizations**  -  Manage organizations

- `action1-cli organizations get`  -  Gets a list of organizations within the current Action1 enterprise.
- `action1-cli organizations org-id-delete`  -  **Requires permission: `manage_organizations`** Removes an organization from the enterprise.
- `action1-cli organizations org-id-patch`  -  **Requires permission: `manage_organizations`** Updates settings for an organization specified by its ID.
- `action1-cli organizations post`  -  **Requires permission: `manage_organizations`** Creates a new organization.

**permissions**  -  Manage permissions

- `action1-cli permissions`  -  Gets a list of available permission templates.

**remote_search**  -  Manage remote search

- `action1-cli remote-search <orgId>`  -  **Requires permissions: `view_endpoints`, `view_software_repository`

**reportdata**  -  Manage reportdata


**reports**  -  Manage reports

- `action1-cli reports all-get`  -  Gets a list of existing reports. At this time all reports are enterprise-wide.
- `action1-cli reports org-id-category-id-get`  -  Gets a list of reports and categories. At this time all reports are enterprise-wide.
- `action1-cli reports org-id-custom-id-delete`  -  **Requires permission: `manage_reports`** Deletes a custom report specified by its ID.
- `action1-cli reports org-id-custom-id-patch`  -  **Requires permission: `manage_reports`** Updates a custom report. You cannot changes a custom report's category.
- `action1-cli reports org-id-custom-post`  -  **Requires permission: `manage_reports`** Creates a custom report in the predefined Custom report category.

**roles**  -  Manage roles

- `action1-cli roles get`  -  **Requires permission: `manage_roles`** Gets a list of available roles.
- `action1-cli roles id-delete`  -  **Requires permission: `manage_roles`** Deletes a role specified by its ID.
- `action1-cli roles id-get`  -  **Requires permission: `manage_roles`** Gets details about a role specified by its ID.
- `action1-cli roles id-patch`  -  **Requires permission: `manage_roles`** Updates a role specified by its ID.
- `action1-cli roles post`  -  **Requires permission: `manage_roles`** Creates a new role.

**scripts**  -  Manage scripts

- `action1-cli scripts org-id-get`  -  **Requires permission: `use_scripts`** Gets a list of existing scripts from the Script Library.
- `action1-cli scripts org-id-id-delete`  -  **Requires permission: `manage_scripts`** Deletes an existing custom script specified by its ID.
- `action1-cli scripts org-id-id-get`  -  **Requires permission: `use_scripts`** Gets details for a script specified by its ID.
- `action1-cli scripts org-id-id-patch`  -  **Requires permission: `manage_scripts`** Updates details for an existing custom script specified by its ID.
- `action1-cli scripts org-id-post`  -  **Requires permission: `manage_scripts`** Creates a new custom script and adds it to the Script Library.

**setting-templates**  -  Manage setting templates

- `action1-cli setting-templates org-id-get`  -  Gets a list of existing setting templates. Setting templates are maintained by Action1 and cannot be modified.
- `action1-cli setting-templates org-id-template-id-get`  -  Gets details about a setting template specified by its ID.

**settings**  -  Manage settings

- `action1-cli settings org-id-get`  -  **Requires permission: `manage_advanced_settings`** Lists all existing settings.
- `action1-cli settings org-id-id-delete`  -  **Requires permission: `manage_advanced_settings`** Deletes an existing setting specified by its ID.
- `action1-cli settings org-id-id-get`  -  **Requires permission: `manage_advanced_settings`** Gets details about the setting configuration.
- `action1-cli settings org-id-id-patch`  -  **Requires permission: `manage_advanced_settings`** Updates an existing setting specified by its ID.
- `action1-cli settings org-id-post`  -  **Requires permission: `manage_advanced_settings`** Creates a new setting.

**software-repository**  -  Action1 Software Repository is a continuously updated private application repository that hosts the latest versions of all supported applications to keep your endpoints secure and patched. Action1 maintains built-in software repository packages that are available to all customers as well as enables users to create private software packages to be shared within their organizations only. The package object is a container of software versions and initially, it should contain the first available version of the software (child object called "version"). New versions are added continuously as the respective vendor releases them. Each version object contains the software binary setup files in MSI, EXE or ZIP format (up to 32Gb in size) and deployment settings, such as silent install switches. It may also include the additional actions to be executed before or after a software installation or uninstallation (e.g., reboot or run a script).

- `action1-cli software-repository packages-all-get`  -  **Requires permission
- `action1-cli software-repository packages-all-package-id-delete`  -  **Requires permission: `manage_software_repository`** Deletes a custom Software Repository package specified by its ID.
- `action1-cli software-repository packages-all-package-id-get`  -  **Requires permission: `view_software_repository`** Gets details for a Software Repository package specified by its ID.
- `action1-cli software-repository packages-all-package-id-patch`  -  **Requires permission
- `action1-cli software-repository packages-all-post`  -  **Requires permission

**subscription**  -  View Action licenses, start or extend a free trial, or request a price quote.

- `action1-cli subscription license-enterprise-get`  -  **Requires permission: `manage_enterprise`** Gets details about the enterprise license.
- `action1-cli subscription license-enterprise-quote-post`  -  **Requires permission: `manage_enterprise`** Sends a quote request to the Action1 Sales department.
- `action1-cli subscription license-enterprise-trial-post`  -  **Requires permission
- `action1-cli subscription license-usage-enterprise-get`  -  **Requires permission: `manage_enterprise`** Gets details about license usage for the entire enterprise.
- `action1-cli subscription license-usage-organizations-get`  -  **Requires permission
- `action1-cli subscription license-usage-organizations-org-id-get`  -  **Requires permission

**updates**  -  Manage updates

- `action1-cli updates org-id-get`  -  **Requires one of the following permissions: `approve_updates`, `view_dashboards`
- `action1-cli updates org-id-package-id-get`  -  **Requires one of the following permissions: `approve_updates`, `view_dashboards`

**users**  -  Manage users

- `action1-cli users get`  -  **Requires permission: `view_users`** Gets a list of users within the current Action1 enterprise.
- `action1-cli users id-delete`  -  **Requires permission: `manage_users`** Deletes an existing user.
- `action1-cli users id-get`  -  **Requires permission: `view_users`** Gets an existing user.
- `action1-cli users id-patch`  -  **Requires permission: `manage_users`** Updates an existing user.
- `action1-cli users post`  -  **Requires permission: `manage_users`** Creates a new user.

**vulnerabilities**  -  Manage vulnerabilities

- `action1-cli vulnerabilities org-id-cve-id-get`  -  **Requires permission
- `action1-cli vulnerabilities org-id-get`  -  **Requires permission


**fleet**


- `action1-cli fleet patch-posture`  -  See every endpoint across all your client organizations ranked by how many updates it is missing  -  one fleet-wide view.

- `action1-cli fleet vuln-triage`  -  Rank CVEs across every organization by how many endpoints they hit, weighted by CVSS and the CISA Known-Exploited flag.

- `action1-cli fleet stale`  -  Surface endpoints that have not checked in for N days, or are offline, across all organizations.

- `action1-cli fleet patch-drift`  -  Diff two synced snapshots to show which updates were remediated and which newly appeared since last sync.

- `action1-cli fleet software-rollup`  -  Deduplicate installed software across the whole fleet into app name x version spread x install count.

- `action1-cli fleet health-score`  -  A single composite score per endpoint from missing updates, open vulnerabilities, reboot-required, and staleness  -  fleet-ranked.

- `action1-cli fleet automation-health`  -  Success and failure rates across automation instances and their per-endpoint results, aggregated across organizations.

- `action1-cli fleet org-scorecard`  -  One row per client organization: endpoint count, missing updates, open CVEs, KEV exposure, and stale agents  -  the per-client posture number MSPs report to account managers.

- `action1-cli fleet reboot-pending`  -  Every endpoint fleet-wide where an installed update is waiting on a reboot to finish  -  the action queue that closes out a patch cycle.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
action1-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Worst-patched endpoints, fleet-wide

```bash
action1-cli fleet patch-posture --agent --limit 25
```

Cross-org rollup of endpoints ranked by missing-update count.

### Known-exploited CVE triage

```bash
action1-cli fleet vuln-triage --kev-only --agent --limit 25
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
action1-cli fleet patch-drift --agent --limit 50
```

What got patched and what newly appeared since the previous sync.

### Read or update a shared software installer

Use `action1_search` with `installer`, then package GET with `fields: versions`
to discover `versions[].id`. Pass that ID as `versionId`, not the displayed
`version` number. `orgId: all` requests Enterprise scope and needs repository
permissions there; an organization-scoped read does not prove Enterprise write
access. Manage Software Repository includes View in the same scope. Version
PATCH takes edited properties directly in MCP `params`; approval and EULA
acceptance fields require separate permissions. Built-in versions cannot be
modified. API-key access is separate from browser/Entra SSO. Preserve existing
configuration and read it back after an authorized update. See the
[Software installer MCP recipe](./README.md#software-installer-packages-through-the-mcp-server)
for endpoint IDs, params, and permission requirements.

## Auth Setup

Action1 uses OAuth2 token-mint. Generate a Client ID and Client Secret on the API Credentials page in the Action1 console, then set ACTION1_CLIENT_ID and ACTION1_CLIENT_SECRET. The CLI POSTs them to /oauth2/token (JSON body) to obtain a bearer token automatically. Set ACTION1_ORG_ID to scope the fleet commands to one organization by default. For the EU or Australia data center set ACTION1_BASE_URL to https://app.eu.action1.com/api/3.0 or https://app.au.action1.com/api/3.0; the token is minted from the same host. ACTION1_OAUTH2 takes a pre-minted bearer token instead of the client pair. Per-organization API commands like 'endpoints managed <orgId>' take the organization id as their first argument.

Run `action1-cli doctor` to verify setup.

## Credential safety

- The `Authorization` header, the client credentials and every API request go only to the scheme, host and port of `ACTION1_BASE_URL` (default `https://app.action1.com/api/3.0`). The token is minted from `<base URL>/oauth2/token` on that same host, so an EU or Australia base URL mints from its own region. A redirect to any other host is refused, so a credential never reaches it.
- Any output unit that would repeat a credential this program holds, in any encoding, is replaced whole by a notice such as "[credkit: withheld from stdout: it carried a credential this program holds]": stdout, stderr, files, the local store and every MCP message. Held: `ACTION1_OAUTH2`, `ACTION1_CLIENT_SECRET`, the unique part of `ACTION1_CLIENT_ID` (the `api-key-` prefix and `@action1.com` stay printable, so the API host is never hidden), a minted token, and the token fields Action1 returns. Responses are never cached on disk.
- Only `credentials.toml` (mode 0600) holds a credential. A client ID and secret supplied by the environment are never written to disk, and nor is the token minted for them: each process mints its own and keeps it in memory (one mint per CLI run; the MCP server reuses its token until it expires). A token is cached in `credentials.toml` only beside the client pair stored there (not at all under `ACTION1_NO_CONFIG_WRITE=1`), and a stored token is only ever sent for the client it was minted for. `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing and never mints a token. Over MCP, a call that changes Action1 (`action1_execute`, `import`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

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
  action1-cli enterprise get --agent --select allowed_fast_closure,closure_requested_at,closure_requested_by
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

- Use `--home <dir>` for one invocation, or set `ACTION1_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `ACTION1_CONFIG_DIR`, `ACTION1_DATA_DIR`, `ACTION1_STATE_DIR`, `ACTION1_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `ACTION1_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `action1-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "action1": {
        "command": "action1-mcp",
        "env": {
          "ACTION1_HOME": "/srv/action1"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `ACTION1_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `ACTION1_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `ACTION1_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `ACTION1_DATA_SOURCE` sets the `--data-source` default.

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
action1-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "action1-cli learnings confirm 12"] }
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
       materially more, record the divergence via `action1-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `action1-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `action1-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
action1-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
action1-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
action1-cli teach-playbook \
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
action1-cli playbook amend \
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

`action1-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `ACTION1_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
action1-cli feedback "the --since flag is inclusive but docs say exclusive"
action1-cli feedback --stdin < notes.txt
action1-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `ACTION1_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ACTION1_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
action1-cli profile save briefing --json
action1-cli --profile briefing enterprise get
action1-cli profile list --json
action1-cli profile show briefing
action1-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `action1-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add action1-mcp -- action1-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `action1-mcp --transport http` requires `ACTION1_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `ACTION1_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `ACTION1_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which action1-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   action1-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `action1-cli <command> --help`.
