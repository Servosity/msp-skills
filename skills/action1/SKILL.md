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
      - kind: go
        bins: [action1-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/action1/cmd/action1-cli
---

# Action1  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `action1-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install action1 --cli-only
   ```
2. Verify: `action1-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/action1/cmd/action1-cli@latest
```

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


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
action1-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Action1 uses OAuth2 token-mint. Generate a Client ID and Client Secret on the API Credentials page in the Action1 console, then set ACTION1_CLIENT_ID and ACTION1_CLIENT_SECRET. The CLI POSTs them to /oauth2/token (JSON body) to obtain a bearer token automatically. Set ACTION1_ORG_ID to scope the fleet commands to one organization by default, and ACTION1_REGION (us/eu/au) to pick your data center. Per-organization API commands like 'endpoints managed <orgId>' take the organization id as their first argument.

Run `action1-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  action1-cli enterprise get --agent --select id,name,status
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
action1-cli feedback "the --since flag is inclusive but docs say exclusive"
action1-cli feedback --stdin < notes.txt
action1-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/action1-cli/feedback.jsonl`. They are never POSTed unless `ACTION1_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ACTION1_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `action1-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/action1/cmd/action1-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add action1-mcp -- action1-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which action1-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   action1-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `action1-cli <command> --help`.
