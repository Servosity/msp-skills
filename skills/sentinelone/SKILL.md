---
name: sentinelone
description: "Every SentinelOne v2.1 management endpoint, plus an offline SQLite store and cross-entity analytics  -  fleet health, threat triage, blast radius, drift  -  that no console view offers. Trigger phrases: `triage sentinelone threats`, `check sentinelone threats`, `sentinelone fleet health`, `which endpoints have active threats`, `what changed in sentinelone overnight`, `sentinelone agent status`, `use sentinelone`, `run sentinelone-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "SentinelOne"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - sentinelone-cli
    install:
      - kind: go
        bins: [sentinelone-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/sentinelone/cmd/sentinelone-cli
---

# SentinelOne  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `sentinelone-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install sentinelone --cli-only
   ```
2. Verify: `sentinelone-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/sentinelone/cmd/sentinelone-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Query and manage your whole SentinelOne fleet from the terminal: agents, threats, activities, sites, groups, exclusions, Ranger, and more. Sync to a local store for offline full-text search, then run analytics the console can't  -  `fleet-health stale` ranks decaying endpoints, `threats blast-radius` traces one hash across the fleet, `whatchanged --since 24h` diffs overnight, and `posture` rolls up a per-tenant scorecard. Ships an MCP server so an AI agent can drive all of it.

## When to Use This CLI

Use this CLI when an agent or analyst needs to query or act on a SentinelOne tenant from the terminal or via MCP: listing and filtering agents/threats/activities, taking agent or threat actions, or  -  its differentiator  -  answering cross-entity and historical questions (fleet health, coverage gaps, blast radius, overnight drift, version rollout, MTTR) that the web console and raw API can't compute. Prefer it over raw API calls whenever the question spans multiple entities, multiple sites, or time.

## Anti-triggers

Do not use this CLI for:
- Data Lake / Purple AI queries (alerts, vulnerabilities, misconfigurations, PowerQuery events)  -  that is a different SentinelOne surface; use the official Sentinel-One/purple-mcp server instead.
- Bulk SIEM-scale telemetry ingestion of Deep Visibility events  -  use the official SIEM integrations (Sumo Logic, Google SecOps); this CLI persists bounded dv pulls, not a streaming pipeline.
- Console-only settings not exposed by the v2.1 management API (SSO config, billing, console UI preferences)  -  use the web console.
- Anything requiring request bodies the public spec omits beyond the documented --stdin JSON passthrough  -  check the tenant's own api-doc for exact field names first.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Time-Travel & Diffing
- **`whatchanged`**  -  One answer to 'what changed across all my tenants since I logged off?'  -  new threats, newly-offline or newly-unhealthy agents, version regressions, and protection-mode flips. Needs at least 2 syncs of local history.

  _Reach for this instead of paging Get_Threats + Get_Agents and diffing by hand  -  it returns the cross-entity delta over a window that no single API call provides._

  ```bash
  sentinelone-cli whatchanged --since 24h --agent
  ```
- **`threats verdicts`**  -  Flags threats whose analyst verdict, confidence level, or incident status changed since the last sync  -  suspicious to malicious, or an auto-mitigated threat re-opened  -  so nothing flips silently. Needs at least 2 syncs of local history.

  _Use to catch silent verdict flips  -  it diffs against stored prior state the API doesn't retain._

  ```bash
  sentinelone-cli threats verdicts --changed --agent
  ```

### Threat Intelligence Joins
- **`threats recurrence`**  -  Surfaces threats whose same hash or name re-appears across endpoints  -  or returns on an endpoint after a prior mitigation  -  the signal of an unkilled root cause. Needs at least 2 syncs of local history.

  _Use when a threat keeps coming back  -  it identifies the recurring hash/endpoint pair a single threat listing can't reveal._

  ```bash
  sentinelone-cli threats recurrence --by-agent --agent
  ```
- **`threats mttr`**  -  Computes mean time from threat detection to mitigation per site, and flags SLA breaches and the longest-unresolved threats. Needs at least 2 syncs of local history.

  _Use to report response performance  -  it derives detection-to-mitigation durations the API only exposes as raw timestamps._

  ```bash
  sentinelone-cli threats mttr --agent
  ```
- **`threats blast-radius`**  -  For one threat, instantly shows every endpoint it touched, which are mitigated vs still active, the affected sites/groups, and the spread timeline.

  _Use during incident response  -  it answers 'where else is this?' by joining one threat across the whole fleet and timeline._

  ```bash
  sentinelone-cli threats blast-radius 3f5a9c2e1b7d8a4f6c0e2d1a9b8c7f6e5d4c3b2a --agent
  ```
- **`threats triage`**  -  One ranked, cross-site worklist of every open threat  -  scored by confidence × severity × age  -  so the morning triage order needs zero console scope flips.

  _Reach for this first each morning  -  it returns the cross-site triage order no single threats query computes._

  ```bash
  sentinelone-cli threats triage --agent
  ```
- **`agents dossier`**  -  Everything about one endpoint on one card: agent state, full threat history, recent activities, and site/group membership  -  the IR handoff view.

  _Use when drilling into one box during incident response  -  it joins the endpoint's whole story in one call._

  ```bash
  sentinelone-cli agents dossier "FINANCE-LT-042" --agent
  ```

### Fleet Health & Coverage
- **`fleet-health stale`**  -  Ranks endpoints by a composite decay score  -  last-seen age, last-scan age, out-of-date agent version, and reduced or disabled protection  -  so the riskiest agents triage first.

  _Reach for this to answer 'which endpoints are rotting?'  -  a ranked health score the console never computes._

  ```bash
  sentinelone-cli fleet-health stale --agent
  ```
- **`coverage gaps`**  -  Lists endpoints in detect-only mode, with self-protection off, or with Ranger/firewall/device-control disabled  -  the 'are we actually protecting everyone?' compliance view per tenant.

  _Use for compliance/QBR prep  -  it finds unprotected endpoints by site that require joining policy state to site membership._

  ```bash
  sentinelone-cli coverage gaps --agent
  ```
- **`versions rollout`**  -  Shows agent-version distribution per site over time and flags sites stuck on EOL/old versions or stalled mid-upgrade-wave. Needs at least 2 syncs of local history.

  _Reach for this during an upgrade campaign  -  it tracks rollout progress per site, which requires snapshot history the API doesn't keep._

  ```bash
  sentinelone-cli versions rollout --agent
  ```
- **`ranger exposure`**  -  Surfaces unmanaged/rogue endpoints on each subnet by cross-referencing Ranger-discovered devices against managed agents, ranked by managed-peer density.

  _Reach for this to find blind spots  -  it joins network discovery to managed agents to surface rogue devices a single listing can't._

  ```bash
  sentinelone-cli ranger exposure --agent
  ```
- **`fleet-health summary`**  -  At-a-glance fleet counts  -  online/offline/decommissioned, infected, out-of-date, under-protected  -  across all sites in one call.

  _Use for the weekly fleet sweep  -  one command replaces the per-client CSV-export-and-pivot ritual._

  ```bash
  sentinelone-cli fleet-health summary --agent
  ```

### Reporting & Rollups
- **`posture`**  -  A one-page per-tenant rollup  -  agent health %, coverage %, open-threat count, oldest unresolved, version compliance  -  for the morning MSSP review or a client QBR.

  _Reach for this for a client-ready summary  -  it composes health, coverage, and threat metrics into one scorecard the API never returns._

  ```bash
  sentinelone-cli posture --agent
  ```
- **`exclusions audit`**  -  Flags risky exclusions  -  never matched by any threat, wildcard paths, and entries older than a threshold  -  the 'are we hiding real threats?' review.

  _Use for periodic security hygiene  -  it cross-references exclusions to threat history the API never correlates._

  ```bash
  sentinelone-cli exclusions audit --agent
  ```
- **`sites risk`**  -  Ranks sites/clients against each other by composite risk  -  open-threat density, coverage gaps, stale agents, MTTR  -  so you know which tenant to call first.

  _Reach for this for the portfolio view  -  it ranks every client by risk in one command._

  ```bash
  sentinelone-cli sites risk --agent
  ```

## Command Reference

**accounts**  -  accounts operations

- `sentinelone-cli accounts create`  -  Create a new Account. This command requires Global permissions and an MSSP deployment.
- `sentinelone-cli accounts get`  -  Get the Accounts, and their data, that match the filter.
- `sentinelone-cli accounts get-by-id`  -  Get Account data from a given Account ID. To get an Account ID, run 'accounts'.
- `sentinelone-cli accounts update`  -  Change the data of an Account. This command requires a Global user or an Account user and Admin role.

**activities**  -  activities operations

- `sentinelone-cli activities get`  -  Get the activities, and their data, that match the filters. We recommend that you set some values for the filters.
- `sentinelone-cli activities get-activity-types`  -  Get a list of activity types. This is useful to see valid values to filter activities in other commands.

**agents**  -  agents operations

- `sentinelone-cli agents abort-scan`  -  Immediately stop a Full Disk Scan on all Agents that match the filter.
- `sentinelone-cli agents approve-uninstall`  -  If a user tries to uninstall the SentinelOne Agent from an endpoint, an uninstall request is sent to the Management.
- `sentinelone-cli agents broadcast-message`  -  You can send a message through the Agents that users can see. <BR>This is useful for endpoints that have human users.
- `sentinelone-cli agents can-run-remote-shell`  -  Who can run Remote Shell? Remote Shell is a powerful way to respond remotely to events on endpoints.
- `sentinelone-cli agents clear-remote-shell`  -  Remote Shell is a powerful way to respond remotely to events on endpoints.
- `sentinelone-cli agents connect-to-network`  -  After you run 'disconnect from network' on endpoints, analyze the issue, and mitigate threats.
- `sentinelone-cli agents count`  -  Get the count of Agents that match a filter. This command is useful to run before you run other commands.
- `sentinelone-cli agents decommission`  -  If a user is scheduled for time off, or a device is scheduled for maintenance, you can decommission the Agent.
- `sentinelone-cli agents disable`  -  Use this command to disable Agents that match the filter.
- `sentinelone-cli agents disable-ranger`  -  Disable Ranger from the Agents that match the filter.
- `sentinelone-cli agents disconnect-from-network`  -  Use this command to isolate (quarantine) endpoints from the network, if the endpoints match the filter.
- `sentinelone-cli agents enable`  -  Use this command to enable disabled Agents that match the filter.
- `sentinelone-cli agents enable-ranger`  -  SentinelOne Ranger gives full visibility of all devices connected to your network.
- `sentinelone-cli agents fetch-firewall-logs`  -  Get Firewall Control events in the local log file, written in clear text
- `sentinelone-cli agents fetch-firewall-rules`  -  Firewall Control is disabled at the Global level.
- `sentinelone-cli agents fetch-logs`  -  Get the Agent and Endpoint logs from Agents that match the filter.
- `sentinelone-cli agents get`  -  Get the Agents, and their data, that match the filter.
- `sentinelone-cli agents get-application`  -  Get the installed applications for a specific Agent. <BR>To get the Agent ID, run 'agents'.
- `sentinelone-cli agents get-installed-apps-for`  -  Application Risk Management is an EA feature.
- `sentinelone-cli agents get-passphrase`  -  Show the passphrase for the Agents that match the filter. This is an important command.
- `sentinelone-cli agents initiate-scan`  -  Use this command to run a Full Disk Scan on Agents that match the filter.
- `sentinelone-cli agents mark-as-uptodate`  -  The value of the Agent version as 'up-to-date' is a useful filter for many actions.
- `sentinelone-cli agents move-between-sites`  -  This command requires Account or Global level access.
- `sentinelone-cli agents move-to-console`  -  You can move Agents between Management Consoles.
- `sentinelone-cli agents processes`  -  [OBSOLETE] Returns empty array. To get processes of an Agent, see Applications.
- `sentinelone-cli agents randomize-uuid`  -  IMPORTANT: This action will assign a new UUID to Agents that match the filter.
- `sentinelone-cli agents reject-uninstall`  -  Reject uninstall requests for all Agents that match the filter.
- `sentinelone-cli agents reset-local-config`  -  SentinelCtl is the CLI for Agents. It runs commands directly on one Agent at a time.
- `sentinelone-cli agents restart`  -  Use this command to restart endpoints that have an Agent installed and that fit the filter.
- `sentinelone-cli agents set-external-id`  -  You can add a Customer Identifier (a string) to identify each endpoint or to tag sets of endpoints.
- `sentinelone-cli agents set-persistent-configuration-overrides`  -  This command requires Global permissions or Support.
- `sentinelone-cli agents shutdown`  -  You can shut down endpoints remotely for performance, maintenance, or security.
- `sentinelone-cli agents start-remote-profiling`  -  Use this command to start remote profiling on Agents that match the filter.
- `sentinelone-cli agents start-remote-shell`  -  Remote shell is an opened websocket between the browser and the Agent
- `sentinelone-cli agents stop-remote-profiling`  -  Use this command to stop remote profiling on Agents that match the filter.
- `sentinelone-cli agents terminate-remote-shell`  -  Remote Shell is a powerful, full shell for Windows, macOS, and Linux.
- `sentinelone-cli agents uninstall`  -  Use this command to uninstall Agents that match the filter.
- `sentinelone-cli agents update-software`  -  Use this command to update the Agent version on endpoints that have the Agent installed and that match the filter.

**application-inventory**  -  application-inventory operations

- `sentinelone-cli application-inventory`  -  [DEPRECATED] Retrieve application inventory grouped by Name, Publisher.

**application-inventory-counts**  -  application-inventory-counts operations

- `sentinelone-cli application-inventory-counts`  -  [DEPRECATED] Application inventory counters.

**applications**  -  applications operations


**cloud-detection**  -  cloud-detection operations

- `sentinelone-cli cloud-detection activate-rules`  -  Activate Custom Detection Rules based on a filter.
- `sentinelone-cli cloud-detection create-rule`  -  Create a Custom Detection Rule for a scope specified by ID.
- `sentinelone-cli cloud-detection delete-rules`  -  Deletes Custom Detection Rules that match a filter.
- `sentinelone-cli cloud-detection disable-rules`  -  Disable Custom Detection Rules based on a filter.
- `sentinelone-cli cloud-detection get-alerts`  -  Get a list of alerts for a given scope
- `sentinelone-cli cloud-detection get-rules`  -  Get a list of Custom Detection Rules for a given scope.
- `sentinelone-cli cloud-detection update-alert-analyst-verdict`  -  Change the verdict of an alert
- `sentinelone-cli cloud-detection update-rule`  -  Change a Custom Detection rule. This command requires the rule ID. (See Get Rules).
- `sentinelone-cli cloud-detection updated-threat-incident`  -  Update the incident details of an alert.

**config-override**  -  config-override operations

- `sentinelone-cli config-override create`  -  Override the configuration of Agents that match the filter.
- `sentinelone-cli config-override delete`  -  Delete overrides value. To get the required IDs, run 'config-override'.
- `sentinelone-cli config-override delete-configoverride`  -  Delete an override value. To get the required ID, run 'config-override'.
- `sentinelone-cli config-override get`  -  There are different ways to override the configuration of an Agent
- `sentinelone-cli config-override update`  -  Use this command to change the value of one configuration value. To get the required ID, run 'config-override'.

**device-control**  -  device-control operations

- `sentinelone-cli device-control copy-rules`  -  You can copy a set of Device Control rules to use in other Accounts, Sites, or Groups.
- `sentinelone-cli device-control create-rule`  -  Use this command to create a new Device Control rule. These rules allow or block devices, based on device identifiers.
- `sentinelone-cli device-control delete-rules`  -  Delete Device Control rules that match the filter.
- `sentinelone-cli device-control enable-disable-rules`  -  It is best practice to disable a rule rather than delete it.
- `sentinelone-cli device-control export-rules`  -  Export Device Control rules to a CSV file.
- `sentinelone-cli device-control get-configuration`  -  Get Device Control configuration for a given scope. You can enter a Group ID, Site ID, Account ID, or 'tenant = true'.
- `sentinelone-cli device-control get-device-rules`  -  Get the Device Control rules of a specified Account, Site, Group or Global (tenant) that match the filter.
- `sentinelone-cli device-control get-events`  -  Get the data of Device Control events on Windows and macOS endpoints with Device Control-enabled Agents that match the
- `sentinelone-cli device-control import-rules`  -  Import Device Control rules from a CSV file.
- `sentinelone-cli device-control move-rules`  -  You can move a set of Device Control rules to other Accounts, Sites, or Groups.
- `sentinelone-cli device-control reorder-rules`  -  When an external device connects to an endpoint
- `sentinelone-cli device-control update-configuration`  -  Use this command to change the Device Control configuration. Enter a Group ID, Site ID, Account ID, or 'tenant = true'.
- `sentinelone-cli device-control update-device-rule`  -  Change the Device Control rule that matches the filter. To learn more about the fields, see https://support.sentinelone.

**dv**  -  dv operations

- `sentinelone-cli dv cancel-running-query`  -  Stop a Deep Visibility Query by queryId. The body is {'queryID':'string_ID'}. Get the ID of the query from 'init-query'.
- `sentinelone-cli dv create-query-and-get-query-id`  -  Start a Deep Visibility Query and get the queryId.
- `sentinelone-cli dv download-source-process-file`  -  Download the source process file associated with a Deep Visibility event.
- `sentinelone-cli dv get-events`  -  Get all Deep Visibility events from a queryId.
- `sentinelone-cli dv get-events-by-type`  -  Get Deep Visibility results from the query that matches the given event type.
- `sentinelone-cli dv get-process-state`  -  Get details of all Deep Visibility processes from a queryId.To get the ID from 'init-query'.
- `sentinelone-cli dv get-query-status`  -  Get that status of a Deep Visibility Query.

**exclusions**  -  exclusions operations

- `sentinelone-cli exclusions create`  -  Create Exclusions to make your Agents suppress alerts and mitigation for items that you consider to be benign or which
- `sentinelone-cli exclusions delete`  -  Every Exclusion opens a possible security hole.
- `sentinelone-cli exclusions get`  -  Get a list of all the Exclusions that match the filter.
- `sentinelone-cli exclusions update`  -  Change the properties of an Exclusion through the data fields.
- `sentinelone-cli exclusions validate-item`  -  Check if an exclusion is on the list of SentinelOne items that are 'Not Allowed' or 'Not Recommended'.

**filters**  -  filters operations

- `sentinelone-cli filters delete`  -  Delete a saved filter.
- `sentinelone-cli filters delete-deep-visibility`  -  Delete a saved Deep Visibility query.
- `sentinelone-cli filters get`  -  Get the list of saved filters. See Save Filter.
- `sentinelone-cli filters get-deep-visibility`  -  Get saved Deep Visibility queries with full data. See Save Deep Visibility Filters.
- `sentinelone-cli filters save`  -  Save a new filter to get a list of matching endpoints.
- `sentinelone-cli filters save-deep-visibility`  -  Save a Deep Visibility query with data as a filter
- `sentinelone-cli filters update`  -  Update an existing filter
- `sentinelone-cli filters update-deep-visibility`  -  Change a saved Deep Visibility filter. To get the ID and fields to change, run Get Deep Visibility Filters.

**firewall-control**  -  firewall-control operations

- `sentinelone-cli firewall-control add-rule-tags`  -  Create a Firewall Rule tag. Create tags to represent Firewall policies - a set of rules in a specific order.
- `sentinelone-cli firewall-control copy-rules`  -  Copy a set of rules to other scopes. In the filter of the body, enter the properties to define the source.
- `sentinelone-cli firewall-control create-firewall-rule`  -  Create a Firewall Control rule for a scope specified by ID (run 'accounts', 'sites', 'groups'
- `sentinelone-cli firewall-control create-firewall-rule-by-category`  -  Create a Firewall Control rule for a scope specified by ID (run 'accounts', 'sites', 'groups'
- `sentinelone-cli firewall-control delete-rules`  -  Delete Firewall Control rules that match the filter.
- `sentinelone-cli firewall-control delete-rules-by-category`  -  Delete Firewall Control rules that match the filter.
- `sentinelone-cli firewall-control enable-disable-rules`  -  Change the status of a set of Firewall Control rules that match the filter to 'Enabled' or 'Disabled'.
- `sentinelone-cli firewall-control export-rules`  -  Export Firewall Control rules that match the filter to a JSON file from a scope specified by ID (run 'accounts', 'sites'
- `sentinelone-cli firewall-control get-configuration`  -  Get the Firewall Control configuration for a given scope.
- `sentinelone-cli firewall-control get-firewall-rules`  -  Get the Firewall Control rules for a scope specified by ID (run 'accounts', 'sites, 'groups', or set 'tenant' to 'true')
- `sentinelone-cli firewall-control get-firewall-rules-by-category`  -  Get the Firewall Control rules for a scope specified by ID (run 'accounts', 'sites, 'groups', or set 'tenant' to 'true')
- `sentinelone-cli firewall-control get-protocols`  -  Get a list of protocols that can be used in Firewall Control rules.
- `sentinelone-cli firewall-control get-tag-firewall-rules`  -  Get all Firewall rules linked to tag, regardless of inheritance mode.
- `sentinelone-cli firewall-control import-rules`  -  Import Firewall Control rules from an exported JSON file to scopes specified by ID (run 'accounts', 'sites', 'groups'
- `sentinelone-cli firewall-control move-rules`  -  Remove Firewall Rules, defined with the ID of the rules (run 'firewall-control')
- `sentinelone-cli firewall-control remove-rule-tags`  -  Remove firewall tags from rules matching the filter.
- `sentinelone-cli firewall-control reorder-rules`  -  Change the order of rules for a scope specified by ID (run 'accounts', 'sites', or 'groups').
- `sentinelone-cli firewall-control set-location`  -  Set location attributes for a Location Aware Firewall Control rule.
- `sentinelone-cli firewall-control update-configuration`  -  Change the Firewall Control configuration for a given scope.
- `sentinelone-cli firewall-control update-firewall-rule-by-category`  -  Change a Firewall Control rule.

**groups**  -  groups operations

- `sentinelone-cli groups create`  -  Create a new group. You must create the Group in a Site (run 'sites' to get the Site ID) for which you have permissions.
- `sentinelone-cli groups delete`  -  Delete a Group given by the required Group ID (run 'groups').
- `sentinelone-cli groups get`  -  Get data of groups that match the filter. Best practice: use as narrow a filter as you can.
- `sentinelone-cli groups get-by-id`  -  Get data of a given Group. To get a Group ID, run 'groups'.
- `sentinelone-cli groups update`  -  Change properties of a Group specified by its ID (run 'groups').
- `sentinelone-cli groups update-ranks`  -  An Agent can belong to only one Group.

**hashes**  -  hashes operations


**installed-applications**  -  installed-applications operations

- `sentinelone-cli installed-applications get`  -  Get the applications, and their data (such as risk level)
- `sentinelone-cli installed-applications get-cves`  -  Get known CVEs for applications that are installed on endpoints with Application Risk-enabled Agents.

**last-activity-as-syslog**  -  last-activity-as-syslog operations

- `sentinelone-cli last-activity-as-syslog`  -  Get the Syslog message that corresponds to the last activity that matches the filter.

**locations**  -  locations operations

- `sentinelone-cli locations create`  -  Create a location that defines parameters of Agents in a scope filter.
- `sentinelone-cli locations delete`  -  Delete location definitions of a given location. To get location IDs, run 'locations'.
- `sentinelone-cli locations get`  -  Get the locations of Agents in a given scope that match the filter.
- `sentinelone-cli locations update`  -  Change the parameter values of a location definition. See Create Location.

**ranger**  -  ranger operations

- `sentinelone-cli ranger add-cred-details`  -  Add cred details to a cred group.
- `sentinelone-cli ranger add-new-deploy-command-for-device-from-agent-from-task-infra`  -  Creates a new agent deploy command for devices. Used for communication between API service and Task Infra service
- `sentinelone-cli ranger change-device-review`  -  Change the review state of one device.
- `sentinelone-cli ranger change-device-review-in-bulk`  -  Change the review state of more than one device.
- `sentinelone-cli ranger change-device-tags`  -  Change the device tags.
- `sentinelone-cli ranger create-cred-group`  -  Create a new Cred Group.
- `sentinelone-cli ranger delete-cred-group`  -  Delete cred group value.
- `sentinelone-cli ranger delete-cred-group-detail`  -  Delete cred group detail value.
- `sentinelone-cli ranger export-data`  -  Export Ranger data to csv. You can set filters to get only relevant data. The response sends the csv data as text.
- `sentinelone-cli ranger get-cred-group-details`  -  Get the data for each row in the Cred Groups details table.
- `sentinelone-cli ranger get-cred-groups`  -  Get the data for each row in the Cred Groups table.
- `sentinelone-cli ranger get-gateways`  -  Get the gateways in your deployment that match the filter from a Ranger scan. Ranger requires a Ranger license.
- `sentinelone-cli ranger get-settings`  -  Ranger gives full visibility of all devices connected to your network.
- `sentinelone-cli ranger get-table`  -  Get the data for each row in the Ranger Device Inventory Table. Best practice: Set filters.
- `sentinelone-cli ranger update-cred-group`  -  Update cred group values.
- `sentinelone-cli ranger update-cred-group-details`  -  Update cred group values.
- `sentinelone-cli ranger update-gateway`  -  Change the Ranger scan configuration for a gateway that Ranger discovered
- `sentinelone-cli ranger update-gateways`  -  Change the status of filtered gateways discovered by Ranger.
- `sentinelone-cli ranger update-settings`  -  Change the Ranger Settings. Best Practice: Get the current settings before you change them. See: Get Ranger Settings.

**rbac**  -  rbac operations

- `sentinelone-cli rbac create-new-role`  -  Create a new role for Role-Based Access Control (RBAC).
- `sentinelone-cli rbac delete-role`  -  With the ID of a role (see Get All Roles), you can delete a role.
- `sentinelone-cli rbac get-all-roles`  -  See roles assigned to users that match the filter, a basic description of the roles
- `sentinelone-cli rbac get-specific-role-definition`  -  With the ID of a role (see Get All Roles) you can see the permissions of that role.
- `sentinelone-cli rbac get-template-for-new-role`  -  Get the template for a new role.
- `sentinelone-cli rbac update-role`  -  With the ID of a role (see Get All Roles), you can update the permissions of users with this role.

**remote-scripts**  -  remote-scripts operations

- `sentinelone-cli remote-scripts get-scripts`  -  Get the SentinelOne scripts from the Script Library.
- `sentinelone-cli remote-scripts run`  -  Run remote script
- `sentinelone-cli remote-scripts upload-a-new-script`  -  Upload a new script

**report-tasks**  -  report-tasks operations

- `sentinelone-cli report-tasks create`  -  Create a task to generate a report immediately, one time in the future, or on a schedule.
- `sentinelone-cli report-tasks get`  -  Get the tasks that were done to generate reports and to schedule future reports. Best Practice: Use a filter.
- `sentinelone-cli report-tasks update`  -  Update the report task of the given ID. To get the task ID, and the data to change, run Get Report Tasks.

**reports**  -  reports operations

- `sentinelone-cli reports delete`  -  Delete the reports that match the filter. To delete a specific report, use its ID (see Get Reports).
- `sentinelone-cli reports delete-tasks`  -  You can schedule a report to be generated on a routine.
- `sentinelone-cli reports download`  -  When the Management generates a report, it is uploaded to the Management Console.
- `sentinelone-cli reports get`  -  Get the reports that match the filter and the data of the reports.
- `sentinelone-cli reports get-insight`  -  Get the Insight Report types.

**restrictions**  -  restrictions operations

- `sentinelone-cli restrictions create-blacklist-item`  -  Create a blacklist item for a SHA1 hash, for the scopes you enter in the filter fields.
- `sentinelone-cli restrictions delete-blacklist-item`  -  Agents immediately identify files on the blacklist and block them from executing.
- `sentinelone-cli restrictions get-blacklist`  -  Get a list of all the items in the Blacklist that match the filter.
- `sentinelone-cli restrictions update-blacklist-item`  -  Change the properties of a Blacklist item through the data fields.
- `sentinelone-cli restrictions validate-blacklist-item`  -  Check if a hash is on the list of SentinelOne items that are 'Not Allowed' or 'Not Recommended'.

**rogues**  -  rogues operations

- `sentinelone-cli rogues export-data`  -  Export Rogues data to CSV. You can set filters to get only relevant data. The response sends the CSV data as text.
- `sentinelone-cli rogues get-settings`  -  Rogues gives full visibility of all unsecured devices connected to your network.
- `sentinelone-cli rogues get-table`  -  Get the data for each row in the Rogues Device Inventory Table. <BR>Best practice: Set filters.
- `sentinelone-cli rogues update-settings`  -  Change the Rogues Settings. Best Practice: Get the current settings before you change them. See: Get Rogues Settings.

**sentinelone-export**  -  Manage sentinelone export

- `sentinelone-cli sentinelone-export activities`  -  Export the list of activities.
- `sentinelone-cli sentinelone-export agents`  -  Export Agent data to a CSV, for Agents that match the filter.
- `sentinelone-cli sentinelone-export events`  -  Export threat events in CSV or JSON format.
- `sentinelone-cli sentinelone-export list-installed-applications`  -  Export the list of applications installed on endpoints with Application Risk-enabled Agents and their properties
- `sentinelone-cli sentinelone-export threat-timeline`  -  Export a threat's timeline.

**sentinelonerss**  -  sentinelonerss operations

- `sentinelone-cli sentinelonerss`  -  Get the SentinelOne RSS feed. In the SentinelOne Management Console, we show the feed contents in the Dashboard.

**settings**  -  settings operations

- `sentinelone-cli settings clear-pending-emails`  -  Clear (discard without sending) pending email notifications for the given Sites (to get the IDs, run 'sites')
- `sentinelone-cli settings delete-notification-recipient`  -  Delete a notification recipient by ID. To get the IDs of recipients, run 'recipients' (see Get Notification Recipients).
- `sentinelone-cli settings get-ad`  -  Get the Global Active Directory settings.
- `sentinelone-cli settings get-ad-fqdns`  -  Get the map of Active Directory FQDNs to user roles of the given Sites (use 'sites' to get IDs) or Accounts ('accounts')
- `sentinelone-cli settings get-microsoft`  -  [DEPRECATED] Gets the Microsoft settings of the Sites or Accounts.
- `sentinelone-cli settings get-notification`  -  Get the notification settings for the given Sites (to get the IDs, run 'settings') or Accounts ('accounts').
- `sentinelone-cli settings get-notification-recipients`  -  Get the emails that are configured to receive notifications.
- `sentinelone-cli settings get-sms`  -  [DEPRECATED] Gets the site's SMS settings.
- `sentinelone-cli settings get-smtp`  -  Get the SMTP server configuration of the given Sites (to get the IDs, run 'sites') or Accounts ('accounts').
- `sentinelone-cli settings get-sso`  -  Get the Single Sign-On configuration for the given Sites (to get the IDs, run 'sites') or Accounts ('accounts').
- `sentinelone-cli settings get-syslog`  -  Get the configuration of the syslog server integrated with the given Sites (to get the IDs, run 'sites')
- `sentinelone-cli settings set-ad`  -  Update the Global Active Directory settings.
- `sentinelone-cli settings set-ad-fqdns`  -  Update the Active Directory FQDNs of a Site or Account.
- `sentinelone-cli settings set-microsoft`  -  [DEPRECATED] Update Microsoft settings for the given Sites or Accounts.
- `sentinelone-cli settings set-notification`  -  Change the notifications for the given Sites (to get the IDs, run 'settings') or Accounts ('accounts').
- `sentinelone-cli settings set-notification-recipients`  -  Set the emails of recipients to get notifications.
- `sentinelone-cli settings set-sms`  -  [DEPRECATED] Set SMS settings.
- `sentinelone-cli settings set-smtp`  -  Change the SMTP server configuration for the given Sites or Accounts.
- `sentinelone-cli settings set-sso`  -  Change the Single Sign-On configuration for the given Sites (to get the IDs, run 'sites') or Accounts ('accounts').
- `sentinelone-cli settings set-syslog`  -  Change the configuration of the syslog server of the given Sites (to get the IDs, run 'sites') or Accounts ('accounts').
- `sentinelone-cli settings test-ad`  -  Test Active Directory settings.
- `sentinelone-cli settings test-microsoft`  -  [DEPRECATED] Test Microsoft settings.
- `sentinelone-cli settings test-smtp`  -  Test SMTP settings between the Management and the SMTP server.
- `sentinelone-cli settings test-sso`  -  Test Single Sign-On settings.
- `sentinelone-cli settings test-syslog`  -  Test Syslog settings. The Management tests the connection to the Syslog server.

**singularity-marketplace**  -  singularity-marketplace operations

- `sentinelone-cli singularity-marketplace delete-marketplace-application`  -  Delete application integration from your Marketplace.
- `sentinelone-cli singularity-marketplace enable-or-disable-application`  -  Use this command to enable or disable application integrations that match the filter.
- `sentinelone-cli singularity-marketplace get-applications-catalog`  -  Get the Marketplace Application Catalog.
- `sentinelone-cli singularity-marketplace get-configuration-fields`  -  Get the Catalog Application Configuration Fields.
- `sentinelone-cli singularity-marketplace get-configuration-fields-for-catalog-application`  -  Returns The configuration schema for a requested Application Catalog.
- `sentinelone-cli singularity-marketplace get-marketplace-applications`  -  Get the installed Marketplace applications for a scope specified.
- `sentinelone-cli singularity-marketplace install-applications`  -  Install application from the Application Catalog.
- `sentinelone-cli singularity-marketplace update-application-configuration`  -  Update installed application configuration.

**site-with-admin**  -  site-with-admin operations

- `sentinelone-cli site-with-admin`  -  Create a Site and an Admin role user.

**sites**  -  sites operations

- `sentinelone-cli sites create`  -  Create a Site.
- `sentinelone-cli sites create-duplicate`  -  [DEPRECATED] Create duplicate site.
- `sentinelone-cli sites delete`  -  Delete the Site of the given ID. To get the ID, run 'sites'.
- `sentinelone-cli sites get`  -  Get the Sites that match the filters. The response includes the IDs of Sites, which you can use in other commands.
- `sentinelone-cli sites get-by-id`  -  Get the data of the Site of the ID. To get the ID, run 'sites'.
- `sentinelone-cli sites update`  -  Change the policy and properties of the Site given by ID. To get the ID, run 'sites'.

**system**  -  system operations

- `sentinelone-cli system cache-status`  -  Get an indication of the system's cache health status.
- `sentinelone-cli system database-status`  -  Get an indication of the system's database health status.
- `sentinelone-cli system get-config`  -  Get the configuration of your SentinelOne system.
- `sentinelone-cli system info`  -  Get the Console build, version, patch, and release information.
- `sentinelone-cli system set-config`  -  Change the system configuration. Before you run this, see Get System Config.
- `sentinelone-cli system status`  -  Get an indication of the system's health status.

**tags**  -  tags operations

- `sentinelone-cli tags create`  -  Add tags to create user-defined logical groups.
- `sentinelone-cli tags delete`  -  Delete tags by given filter.
- `sentinelone-cli tags delete-by-id`  -  Delete tag by ID.
- `sentinelone-cli tags edit`  -  Edit tag
- `sentinelone-cli tags get`  -  Get tags.

**tasks-configuration**  -  tasks-configuration operations

- `sentinelone-cli tasks-configuration create-task`  -  Create a task configuration.
- `sentinelone-cli tasks-configuration get-child-scope-task-configuration`  -  Get the task configuration of child scopes of the given scope, if the tasks are not inherited.
- `sentinelone-cli tasks-configuration get-task-configuration`  -  Get the task configuration of a scope.
- `sentinelone-cli tasks-configuration has-child-scopes`  -  From a given scope, see if there are scopes under it that have local, explicit tasks.

**tenant**  -  tenant operations

- `sentinelone-cli tenant global-policy`  -  Get the Global policy. This is the default policy for your deployment. See also: Get Policy.
- `sentinelone-cli tenant update-global-policy`  -  Change the policy of your deployment. Best practice: Get the Global policy before you attempt to change it.

**tests**  -  tests operations

- `sentinelone-cli tests`  -  Returns a metadata list of the available free-text filters

**threat-intelligence**  -  threat-intelligence operations

- `sentinelone-cli threat-intelligence create-io-cs`  -  Add an IoC to the Threat Intelligence database.
- `sentinelone-cli threat-intelligence delete-io-cs`  -  Delete an IoC from the Threat Intelligence database that matches a filter using the accountID and one other field.

**threats**  -  threats operations

- `sentinelone-cli threats add-note-to-multiple`  -  Add a threat note to multiple threats.
- `sentinelone-cli threats add-to-blacklist`  -  Add threats that have a SHA1 hash and that match the filter to the Blacklist of the target scope: Global, Account, Site
- `sentinelone-cli threats add-to-exclusions`  -  Add a threat to exclusions. The 'whitening option' is required.
- `sentinelone-cli threats disable-engines`  -  If your list of threats shows too many False Positives
- `sentinelone-cli threats export`  -  Export data of threats (as seen in the Console > Incidents) that match the filter. Note: Use the filter.
- `sentinelone-cli threats export-mitigation-report`  -  Export the mitigation report as a CSV file.
- `sentinelone-cli threats fetch-file`  -  Fetch a file associated with the threat that matches the filter.
- `sentinelone-cli threats get`  -  Get data of threats that match the filter. <BR>Best Practice: Use the filters.
- `sentinelone-cli threats mitigate`  -  Apply a mitigation action to a group of threats that match the filter.
- `sentinelone-cli threats update-analyst-verdict`  -  Change the verdict of a threat, as determined by a Console user.
- `sentinelone-cli threats update-external-ticket-id`  -  Change the external ticket ID of a threat.
- `sentinelone-cli threats updated-incident`  -  Update the incident details of a threat.

**update**  -  update operations

- `sentinelone-cli update delete-packages`  -  Delete Agent packages from your Management. Use the IDs from Get Latest Packages.
- `sentinelone-cli update download-agent-package`  -  [DEPRECATED] Download an agent package by package ID.Rate limit: 2 call per minute for each different user token
- `sentinelone-cli update download-package`  -  Download a package by site_id ('sites') and filename. Rate limit: 2 call per minute for each user token.
- `sentinelone-cli update get-latest-packages`  -  Get the Agent packages that are uploaded to your Management.
- `sentinelone-cli update latest-packages-by-os`  -  [DEPRECATED] Use 'Latest packages' API call instead ('GET /web/api/v2.1/update/agent/packages').
- `sentinelone-cli update package`  -  Update the metadata for an existing package.

**upload**  -  upload operations

- `sentinelone-cli upload agent-package`  -  If you have an On-Prem Management or you are a participant in the Beta program
- `sentinelone-cli upload deploy-system-package`  -  If you have an On-Prem Management or you are a participant in the Beta program
- `sentinelone-cli upload system-package`  -  If you have an On-Prem Management or otherwise require a manual package upload

**user**  -  users operations

- `sentinelone-cli user`  -  Get a user by token.

**users**  -  users operations

- `sentinelone-cli users auth-app`  -  Authenticate a user with a third-party app, such as DUO or Google Authenticator
- `sentinelone-cli users auth-by-sso`  -  Authenticate a Single Sign-On response over SAML v2 protocol.
- `sentinelone-cli users auth-recovery-code`  -  Authenticate a user with a recovery code.
- `sentinelone-cli users bulk-delete`  -  Delete all users that match the filter.
- `sentinelone-cli users change-password`  -  Change the user password.
- `sentinelone-cli users check-global`  -  See if logged in user is a user with the Global scope of access.
- `sentinelone-cli users check-remote-shell-permissions`  -  See if the logged in user is allowed to use Remote Shell.
- `sentinelone-cli users check-viewer`  -  See if the logged in user has only viewer permissions.
- `sentinelone-cli users create`  -  Create a new user.
- `sentinelone-cli users delete`  -  Delete a user by ID.
- `sentinelone-cli users disable-2-fa`  -  Disable Two-Factor Authentication for one user. This requires the ID of the user (run 'users').
- `sentinelone-cli users email-verification`  -  When a new user verifies their email, the Management gets a token.
- `sentinelone-cli users enable-2-fa`  -  Enable two-factor authentication for a given user.
- `sentinelone-cli users enable-2-fa-app`  -  Enable support for the 2FA app (such as Duo or Google Authenticator) that your Console users will use to log in.
- `sentinelone-cli users generate-api-token`  -  Get the API token for the authenticated user.
- `sentinelone-cli users generate-i-frame-token`  -  Get a new iFrame token with the provided limitations.
- `sentinelone-cli users generate-recovery-code`  -  Get recovery codes for user authentication.
- `sentinelone-cli users get`  -  Get a user by ID.
- `sentinelone-cli users list`  -  Get a list of users.
- `sentinelone-cli users login`  -  Authenticate a user by username and password and return an authentication token.
- `sentinelone-cli users login-by-api-token`  -  Log in to the API with a token.
- `sentinelone-cli users login-by-token`  -  Log in with user token.
- `sentinelone-cli users logout`  -  Log out the authenticated user.
- `sentinelone-cli users redirect-to-sso`  -  If SSO is enabled for a deployment or scope, and a user attempts to log in with name and password
- `sentinelone-cli users request-2-fa-app`  -  Request 2FA App response.
- `sentinelone-cli users revoke-api-token`  -  Revoke an API token.
- `sentinelone-cli users send-verification-email`  -  Send verification email to users that match the filter.
- `sentinelone-cli users sign-eula`  -  Mark the End User License Agreement (EULA) as signed for user scopes.
- `sentinelone-cli users token-details`  -  Get details of the API token that matches the filter.
- `sentinelone-cli users update`  -  Change properties of the user of the given ID.
- `sentinelone-cli users validate-verification-token`  -  When a new user verifies their email, the Management gets a token. Use this command to validate the token.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
sentinelone-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Triage overnight

```bash
sentinelone-cli whatchanged --since 24h --agent
```

Cross-entity delta of new threats, lost/unhealthy agents, and mode flips since yesterday.

### Morning triage worklist

```bash
sentinelone-cli threats triage --agent
```

Every open threat across all sites, ranked by confidence × severity × age  -  the cross-site triage order the console can't show.

### Narrow a noisy threat feed

```bash
sentinelone-cli threats get --agent --select data.threatInfo.threatName,data.threatInfo.sha1,data.agentRealtimeInfo.agentComputerName
```

Pair --agent with --select dotted paths to pull only the high-gravity fields from deeply-nested threat objects.

### Find decaying endpoints

```bash
sentinelone-cli fleet-health stale --agent
```

Composite decay rank across last-seen, last-scan, version, and protection state.

### Trace a threat across the fleet

```bash
sentinelone-cli threats blast-radius 3f5a9c2e1b7d8a4f6c0e2d1a9b8c7f6e5d4c3b2a --agent
```

Every endpoint a hash touched, mitigated vs active, with a spread timeline.

### Client QBR scorecard

```bash
sentinelone-cli posture --agent
```

Per-site health %, coverage %, open threats, and version compliance in one rollup.

## Auth Setup

Authenticate with a SentinelOne API token: create a Service User (Settings > Users > Service Users), generate its API token, then set `SENTINELONE_API_TOKEN`. Tokens are sent as `Authorization: ApiToken <token>` and inherit the generating user's role and scope. Point the CLI at your console with `SENTINELONE_BASE_URL=https://<your-console>.sentinelone.net/web/api/v2.1`. Run `sentinelone-cli doctor` to confirm reachability and auth.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  sentinelone-cli accounts get --agent --select id,name,status
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
sentinelone-cli feedback "the --since flag is inclusive but docs say exclusive"
sentinelone-cli feedback --stdin < notes.txt
sentinelone-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/sentinelone-cli/feedback.jsonl`. They are never POSTed unless `SENTINELONE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SENTINELONE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
sentinelone-cli profile save briefing --json
sentinelone-cli --profile briefing accounts get
sentinelone-cli profile list --json
sentinelone-cli profile show briefing
sentinelone-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `sentinelone-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/sentinelone/cmd/sentinelone-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add sentinelone-mcp -- sentinelone-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which sentinelone-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   sentinelone-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `sentinelone-cli <command> --help`.
