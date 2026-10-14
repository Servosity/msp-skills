---
name: threatdown
description: "Use when the user asks to find ThreatDown (Malwarebytes) OneView endpoints that stopped checking in across customer sites, rank sites by what needs attention, reconcile seats before billing, triage detections and threats spreading across customers, profile one customer site, isolate, scan, reboot, update or release endpoints with a preview first, or find response jobs that never ran. Trigger phrases: `which endpoints haven't checked in`, `isolate this machine in threatdown`, `threatdown fleet health`, `malwarebytes oneview detections`, `use threatdown`, `run threatdown`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ThreatDown"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - threatdown-cli
    install:
      - kind: script
        bins: [threatdown-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.ps1
---

# ThreatDown OneView Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `threatdown-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.ps1 | iex
   ```
3. Verify: `threatdown-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `threatdown-cli` and `threatdown-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The fleet, threats, respond, seats and customer commands resolve every site's account id for you, so cross-site questions are one command; the generated endpoint, detection and job searches take --account-ids, which 'sites list' shows. fleet stale turns the vendor's endpoints-not-seen-in-24-hours script into one command, fleet health ranks sites by what needs attention, and respond issues scan, isolate, restore, reboot and update jobs with a target preview and a blast-radius guard.

## When to Use This CLI

Use this CLI when an agent needs to read or act on ThreatDown endpoint security across many MSP customer sites: finding offline or infected endpoints, triaging detections, issuing scan or isolation jobs, or reconciling seats. The fleet, threats, respond, seats and customer commands work across every site without account ids; the generated per-resource commands need --account-ids from 'sites list'. threats spread, threats queue and customer report read a local copy that 'fleet sync' fills, so run it first.

## Anti-triggers

Do not use this CLI for:
- Managing a single ThreatDown Nebula (non-OneView) account; this CLI targets the OneView multi-site API
- Consumer Malwarebytes products or the Malwarebytes scam-check service
- Real-time alerting; subscribe to OneView webhooks instead of polling

## Unique Capabilities

### Fleet health across every site
- **`fleet stale`**  -  Every endpoint not seen in N hours across all your sites, with site name and exact hours since last seen.

  _Reach for this for the Monday dark-machine sweep instead of paging endpoints site by site._

  ```bash
  threatdown-cli fleet stale --hours 24 --agent
  ```
- **`fleet health`**  -  One ranked row per site with total, online, stale, infected, suspicious, isolated, reboot-required, unprotected, scan-overdue and update-available counts plus an attention score.

  _Use it to decide which customer needs attention first._

  ```bash
  threatdown-cli fleet health --agent
  ```
- **`fleet sync`**  -  Pull sites, endpoints, detections and jobs from every managed site into one local database.

  _Required before threats spread, threats queue and customer report, which read only this local copy (the generic sync does not fill it); their --since window cannot reach further back than this sync's --since._

  ```bash
  threatdown-cli fleet sync --since 7d
  ```
- **`seats reconcile`**  -  Per-site seat allocations compared with deployed and recently seen endpoints, with expiry flags.

  _Use it before PSA billing sync to catch over- or under-allocated sites._

  ```bash
  threatdown-cli seats reconcile --agent
  ```

### Safe response actions
- **`respond isolate`**  -  Isolate endpoints by host name with a target preview and a 25-endpoint blast-radius guard; sibling commands respond restore, respond scan, respond reboot and respond update work the same way.

  _Use it when an agent must contain an endpoint safely without hand-building a jobs body. It only previews until you add --confirm (or the global --yes), and it needs the execute OAuth scope._

  ```bash
  threatdown-cli respond isolate --host LAPTOP-042
  ```
- **`respond pending`**  -  Jobs that timed out, expired or failed, plus jobs still waiting after --min-age (default 1h), grouped by site and command with a re-issue hint.

  _Use it to confirm that scans, isolations or RMM-issued jobs actually ran._

  ```bash
  threatdown-cli respond pending --since 7d --agent
  ```

### Detection triage from the local store
- **`threats spread`**  -  Every threat in a window ranked by reach (distinct sites and endpoints), by hash or threat name; add --min-sites 2 for cross-site threats only.

  _Use it to judge blast radius during detection triage. Reads the local copy, so run fleet sync first._

  ```bash
  threatdown-cli threats spread --since 7d --agent
  ```
- **`threats queue`**  -  Prioritized list of infected or suspicious endpoints with online and isolated state and a suggested response.

  _Use it to pick which endpoint to contain next. Reads the local copy, so run fleet sync first._

  ```bash
  threatdown-cli threats queue --since 7d --agent
  ```
- **`customer report`**  -  Everything about one customer site in one output: health, stale machines, top threats, isolated endpoints and pending jobs.

  _Use it when a customer calls or a ticket names a site. Reads the local copy, so run fleet sync first._

  ```bash
  threatdown-cli customer report "Acme Corp" --agent
  ```

## Command Reference

**account-info**  -  Manage account info

- `threatdown-cli account-info`  -  Get account id associated to the requesting client

**aidr**  -  Manage aidr

- `threatdown-cli aidr metadata`  -  AI Detection & Response operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/aidr/metadata
- `threatdown-cli aidr rules-create`  -  AI Detection & Response operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/aidr/rules
- `threatdown-cli aidr rules-delete`  -  AI Detection & Response operation the vendor has not documented
- `threatdown-cli aidr rules-get`  -  AI Detection & Response operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/aidr/rules/{id}
- `threatdown-cli aidr rules-list`  -  AI Detection & Response operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/aidr/rules
- `threatdown-cli aidr rules-update`  -  AI Detection & Response operation the vendor has not documented: PUT /oneview/v1/accounts/{account_id}/aidr/rules/{id}
- `threatdown-cli aidr tools-search`  -  AI Detection & Response operation the vendor has not documented: POST /oneview/v1/aidr/tools/search

**app-block**  -  App Block

- `threatdown-cli app-block activity-export`  -  Export app block activity
- `threatdown-cli app-block activity-export-async`  -  Export app block activity asynchronously
- `threatdown-cli app-block activity-search`  -  Search app block activity log
- `threatdown-cli app-block activity-search-groupby`  -  Search app block activity log groupby
- `threatdown-cli app-block rules-create`  -  Create a new App Block Rule
- `threatdown-cli app-block rules-delete`  -  Delete an existing App Block Rule
- `threatdown-cli app-block rules-get`  -  Get an App Block Rule by ID
- `threatdown-cli app-block rules-search`  -  Search App Block Rules
- `threatdown-cli app-block rules-update`  -  Update an existing App Block Rule
- `threatdown-cli app-block rulesets-get`  -  App Block operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/app-block/rulesets/{id}
- `threatdown-cli app-block rulesets-list`  -  App Block operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/app-block/rulesets
- `threatdown-cli app-block settings-get`  -  Get App Block settings for account
- `threatdown-cli app-block settings-update`  -  Create or update App Block settings for account

**asset-ignore-rules**  -  Manage asset ignore rules

- `threatdown-cli asset-ignore-rules create`  -  Ignore Rules operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/assets/ignore-rules
- `threatdown-cli asset-ignore-rules delete`  -  Ignore Rules operation the vendor has not documented: DELETE /oneview/v1/accounts/{account_id}/assets/ignore-rules/{id}
- `threatdown-cli asset-ignore-rules delete-bulk`  -  Ignore Rules operation the vendor has not documented: DELETE /oneview/v1/accounts/{account_id}/assets/ignore-rules
- `threatdown-cli asset-ignore-rules get`  -  Ignore Rules operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/assets/ignore-rules/{id}
- `threatdown-cli asset-ignore-rules list`  -  Ignore Rules operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/assets/ignore-rules
- `threatdown-cli asset-ignore-rules update`  -  Ignore Rules operation the vendor has not documented: PUT /oneview/v1/accounts/{account_id}/assets/ignore-rules/{id}

**cases**  -  Manage cases

- `threatdown-cli cases action`  -  Case Management operation the vendor has not documented: POST /oneview/v1/casemgmt/accounts/{account_id}/cases/action
- `threatdown-cli cases comments-create`  -  Post cases comments
- `threatdown-cli cases comments-list`  -  Get cases comments
- `threatdown-cli cases favorite`  -  Put Cases favorite
- `threatdown-cli cases file`  -  Get Cases file
- `threatdown-cli cases multiplexer-bulk`  -  Post multiplexer cases bulk
- `threatdown-cli cases multiplexer-events`  -  Get cases events
- `threatdown-cli cases multiplexer-metrics-bulk`  -  Post multiplexer metrics bulk
- `threatdown-cli cases overview`  -  Get cases overview
- `threatdown-cli cases requests`  -  Post Cases Requests
- `threatdown-cli cases search`  -  Search Cases across accounts
- `threatdown-cli cases wall`  -  Get cases wall

**content-filtering**  -  Content Filtering

- `threatdown-cli content-filtering blockpage-get`  -  Get DNS Blockpage settings for account
- `threatdown-cli content-filtering blockpage-search`  -  Search DNS blockPages for a given set of accounts
- `threatdown-cli content-filtering blockpage-update`  -  Save DNS Blockpage settings for account
- `threatdown-cli content-filtering categories`  -  Get a collection of DNS content filtering categories with detailed information, including id, name
- `threatdown-cli content-filtering create`  -  Create a new content filtering rule
- `threatdown-cli content-filtering delete`  -  Delete a content filtering rule
- `threatdown-cli content-filtering domain`  -  Get domain details
- `threatdown-cli content-filtering enable`  -  Enable/Disable a content filtering rule
- `threatdown-cli content-filtering enable-for-policy`  -  Enable/Disable a content filtering rule for a Nebula policy
- `threatdown-cli content-filtering exclusions-get`  -  Get DNS Global Exclusions for account
- `threatdown-cli content-filtering exclusions-search`  -  Search DNS global exclusions for a given set of accounts
- `threatdown-cli content-filtering exclusions-update`  -  Save DNS Global Exclusions for account
- `threatdown-cli content-filtering frequent-domains`  -  Get frequently used domain names
- `threatdown-cli content-filtering get`  -  Get a content filtering rule details by id
- `threatdown-cli content-filtering policies`  -  Get list of policies
- `threatdown-cli content-filtering search`  -  Get a list of content filtering rules for current account
- `threatdown-cli content-filtering tlds`  -  Get all known top level domains (TLDs)
- `threatdown-cli content-filtering update`  -  Update a content filtering rule

**cve**  -  Manage cve

- `threatdown-cli cve bulk`  -  Get CVE details in bulk.
- `threatdown-cli cve export`  -  Export CVE data. You can export across multiple accounts. Specify your interested fields and the output format.
- `threatdown-cli cve export-async`  -  Export CVE asynchronously
- `threatdown-cli cve get`  -  Get CVE details by id
- `threatdown-cli cve ignore`  -  Vulnerability Management operation the vendor has not documented: POST /oneview/v1/cve/ignore
- `threatdown-cli cve search`  -  Search CVE across all your managed accounts.
- `threatdown-cli cve search-groupby`  -  Search CVE grouped by a specific field across all your managed account.

**detections**  -  Detections contain information on threats such as malware, ransomware, and malicious URLs found across your sites.

- `threatdown-cli detections export`  -  Export detections data. You can export across multiple accounts. Specify your interested fields and the output format.
- `threatdown-cli detections export-async`  -  Export detections asynchronously
- `threatdown-cli detections get`  -  Get detection and its full information by its detection ID.
- `threatdown-cli detections search`  -  Search detections across all your managed accounts.
- `threatdown-cli detections submit-fp`  -  Submit false positive for a detection

**device-control**  -  The Device Control feature manages access to USB storage drives.

- `threatdown-cli device-control exclusions-bulk`  -  Create device exclusions in bulk.
- `threatdown-cli device-control exclusions-create`  -  Create a device exclusion.
- `threatdown-cli device-control exclusions-delete`  -  Delete a device exclusion.
- `threatdown-cli device-control exclusions-get`  -  Get a device exclusion by its ID.
- `threatdown-cli device-control exclusions-list`  -  Get all device exclusions.
- `threatdown-cli device-control exclusions-search`  -  Search device control exclusions.
- `threatdown-cli device-control exclusions-suggestions`  -  Get excludable devices.
- `threatdown-cli device-control exclusions-update`  -  Update a device exclusion.
- `threatdown-cli device-control export`  -  Export Device Control events. You can export across multiple accounts.
- `threatdown-cli device-control export-async`  -  Export device control events asynchronously
- `threatdown-cli device-control get`  -  Get device control event and its full information by the device control event ID.
- `threatdown-cli device-control search`  -  Search device control events across all your managed accounts.
- `threatdown-cli device-control search-groupby`  -  Group device control events across all your managed accounts.

**dns**  -  DNS

- `threatdown-cli dns export`  -  Export DNS data. You can export across multiple accounts. Specify your interested fields and the output format.
- `threatdown-cli dns export-async`  -  Export DNS Logs asynchronously
- `threatdown-cli dns search`  -  Search DNS Logs across all your managed accounts.
- `threatdown-cli dns search-groupby`  -  Search DNS Logs grouped by a specific field across all your managed accounts.

**email-protection**  -  Email Protection

- `threatdown-cli email-protection 911-mailbox-delete`  -  Delete email protection 911 mailbox
- `threatdown-cli email-protection 911-mailbox-get`  -  Get email protection 911 mailbox
- `threatdown-cli email-protection 911-mailbox-update`  -  Edit email protection 911 mailbox
- `threatdown-cli email-protection allow-list-create`  -  Create a new email protection configurations allow list entry
- `threatdown-cli email-protection allow-list-delete`  -  Delete an email protection configurations allow list entry
- `threatdown-cli email-protection allow-list-search`  -  Get email protection configurations allow list
- `threatdown-cli email-protection allow-list-update`  -  Update email protection configurations allow list
- `threatdown-cli email-protection auto-sync-disable`  -  Disable email protection account auto-sync
- `threatdown-cli email-protection auto-sync-enable`  -  Enable email protection account auto-sync
- `threatdown-cli email-protection auto-sync-groups`  -  Search email protection account auto-sync groups
- `threatdown-cli email-protection auto-sync-status`  -  Get email protection account auto-sync status
- `threatdown-cli email-protection daily-usage`  -  Get email protection account daily usage
- `threatdown-cli email-protection details`  -  Get email protection account details
- `threatdown-cli email-protection disable`  -  Disable an email protection account
- `threatdown-cli email-protection emails`  -  Search email protection emails
- `threatdown-cli email-protection incidents-classify`  -  Classify a specific incident for an account
- `threatdown-cli email-protection incidents-export`  -  Export email protection incidents for an account
- `threatdown-cli email-protection incidents-get`  -  Get the details of a email protection incident for an account
- `threatdown-cli email-protection incidents-scanback`  -  Search Scanback email protection incidents for an account
- `threatdown-cli email-protection incidents-scanback-export`  -  Export Scanback email protection incidents for an account
- `threatdown-cli email-protection incidents-search`  -  Search email protection incidents for an account
- `threatdown-cli email-protection incidents-uncluster`  -  Reclassify incident emails by moving emails from one incident to a new incident with a different classification
- `threatdown-cli email-protection integrations-disable`  -  Disable the integration with Google Workspace or O365
- `threatdown-cli email-protection integrations-domains-add`  -  Add email protection domains
- `threatdown-cli email-protection integrations-domains-check`  -  Check one or more domains for dkim, spf and dmarc records
- `threatdown-cli email-protection integrations-domains-delete`  -  Delete email protection domais
- `threatdown-cli email-protection integrations-domains-list`  -  Get email protection domains
- `threatdown-cli email-protection integrations-gw-admin-consent-url`  -  Get the admin consent URL for the Google Workspace integration
- `threatdown-cli email-protection integrations-gw-authorize`  -  Authorize Google Workspace integration
- `threatdown-cli email-protection integrations-o365-admin-consent-url`  -  Get the admin consent URL for the email protection integration
- `threatdown-cli email-protection integrations-o365-authorize`  -  Authorize access to the O365 tenant
- `threatdown-cli email-protection integrations-o365-phishing-button-manifest`  -  Get the manifest for the Phishing Button OWA XML Manifest
- `threatdown-cli email-protection integrations-status`  -  Get the status of the integration
- `threatdown-cli email-protection mailboxes-export`  -  Export email protection mailboxes for an account
- `threatdown-cli email-protection mailboxes-search`  -  Search email protection mailboxes for an account
- `threatdown-cli email-protection mailboxes-update`  -  Edit email protection mailboxes
- `threatdown-cli email-protection migrate`  -  Migrate an email protection direct customer account to a new MSP parent account
- `threatdown-cli email-protection migration-eligibility`  -  Check if an account is eligible for migration to a new parent
- `threatdown-cli email-protection monthly-usage`  -  Get email protection account monthly usage
- `threatdown-cli email-protection notification-recipients-create`  -  Create email protection notification recipients
- `threatdown-cli email-protection notification-recipients-list`  -  Get email protection notification recipients
- `threatdown-cli email-protection register`  -  Register an email protection account
- `threatdown-cli email-protection register-msp`  -  Register an email protection MSP account
- `threatdown-cli email-protection register-msp-site`  -  Register an email protection MSP site account
- `threatdown-cli email-protection settings-get`  -  Get email protection account settings
- `threatdown-cli email-protection settings-update`  -  Edit email protection account settings
- `threatdown-cli email-protection site-integration-status`  -  Get email protection site integration status
- `threatdown-cli email-protection stats`  -  Get email protection statistics for an account
- `threatdown-cli email-protection stats-domains`  -  Get email protection domains statistics
- `threatdown-cli email-protection stats-emails`  -  Get email protection statistics
- `threatdown-cli email-protection stats-incidents`  -  Get email protection incident statistics
- `threatdown-cli email-protection stats-most-targeted-departments`  -  Get email protection most targeted departments
- `threatdown-cli email-protection stats-most-targeted-users`  -  Get email protection most targeted users

**endpoint-jobs**  -  Manage endpoint jobs

- `threatdown-cli endpoint-jobs bulk-cancel`  -  Cancel pending jobs across accounts
- `threatdown-cli endpoint-jobs bulk-issue`  -  Use this endpoint to issue a job across multiple accounts you manage.
- `threatdown-cli endpoint-jobs export`  -  Export jobs data. You can export across multiple accounts. Specify your interested fields and the output format.
- `threatdown-cli endpoint-jobs get`  -  Get a job and its full information by its job ID.
- `threatdown-cli endpoint-jobs issue`  -  Use this endpoint to issue a job on one or more of your endpoints within an account Possible `command`s are
- `threatdown-cli endpoint-jobs parent-cancel`  -  Cancel parent jobs.
- `threatdown-cli endpoint-jobs parent-children-cancel`  -  Cancel children jobs by correlation ID across multiple accounts.
- `threatdown-cli endpoint-jobs parent-children-reissue`  -  Reissue children jobs.
- `threatdown-cli endpoint-jobs parent-children-search`  -  Search children jobs by correlation ID across multiple accounts.
- `threatdown-cli endpoint-jobs parent-children-summary`  -  Get children jobs summary by correlation ID across multiple accounts.
- `threatdown-cli endpoint-jobs parent-export`  -  Export parent jobs.
- `threatdown-cli endpoint-jobs parent-get`  -  Get parent job by ID across multiple accounts.
- `threatdown-cli endpoint-jobs parent-reissue`  -  Reissue jobs by correlation IDs.
- `threatdown-cli endpoint-jobs parent-search`  -  Search parent jobs across multiple accounts.
- `threatdown-cli endpoint-jobs parent-summary`  -  Get parent jobs summary across multiple accounts.
- `threatdown-cli endpoint-jobs quarantine-bulk`  -  Jobs operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/jobs/quarantine/bulk
- `threatdown-cli endpoint-jobs search`  -  Search jobs across all your managed accounts.
- `threatdown-cli endpoint-jobs search-groupby`  -  Search jobs grouped by a specific field across all your managed account.

**endpoints**  -  An Endpoint is a device which has the ThreatDown Endpoint Agent installed.

- `threatdown-cli endpoints agent-info`  -  Retrieve agent information of a given endpoint using its endpoint ID.
- `threatdown-cli endpoints assets`  -  Retrieve asset information of a given endpoint using its endpoint ID.
- `threatdown-cli endpoints delete`  -  Delete an endpoint from the console using the endpoint ID.
- `threatdown-cli endpoints delete-bulk`  -  Use this endpoint to mark machines as deleted.
- `threatdown-cli endpoints diag-log`  -  Generate a diagnostic log for an endpoint using its ID.
- `threatdown-cli endpoints export`  -  Export endpoint data across multiple accounts. Specify your interested fields and the output format.
- `threatdown-cli endpoints export-async`  -  Export endpoints asynchronously
- `threatdown-cli endpoints get`  -  Retrieve information of a single endpoint by its universally unique identifier.
- `threatdown-cli endpoints network`  -  Get network information about an endpoint using its endpoint ID.
- `threatdown-cli endpoints quarantine`  -  Get quarantined items by the endpoint's ID
- `threatdown-cli endpoints sa-close`  -  Close suspicious activity of an endpoint by its endpoint ID and suspicious activity ID.
- `threatdown-cli endpoints sa-exclusions`  -  Fetch suspicious activity of an endpoint using the endpoint ID and suspicious activity ID.
- `threatdown-cli endpoints sa-open`  -  Open suspicious activity of an endpoint by its endpoint ID and suspicious activity ID.
- `threatdown-cli endpoints sa-remediate`  -  Remediate suspicious activity of an endpoint using the endpoint ID and suspicious activity ID.
- `threatdown-cli endpoints scan`  -  Get a scan of an endpoint by ID.
- `threatdown-cli endpoints scan-detections`  -  Get a list of detections of a scan of an endpoint.
- `threatdown-cli endpoints scans`  -  Get a list of scans of an endpoint by its endpoint ID.
- `threatdown-cli endpoints search`  -  Search endpoints across all your managed accounts.
- `threatdown-cli endpoints search-groupby`  -  Search grouped endpoints
- `threatdown-cli endpoints set-tags`  -  Add an alias to an endpoint by its id. To remove the alias, send an empty string
- `threatdown-cli endpoints site-search`  -  Search endpoints within an account. The search scopes to the account_id you specify in the URL.
- `threatdown-cli endpoints status`  -  Get endpoint status by its ID
- `threatdown-cli endpoints suspicious-activity`  -  Fetch suspicious activity of an endpoint using its endpoint ID.

**events**  -  Events

- `threatdown-cli events <account_id>`  -  An event is a record of activity on your account, such as found a threat, completed a scan, or removed endpoint agent.

**exclusions**  -  Exclusions allow you to prevent trusted applications, websites, and services from being detected by our security engine.

- `threatdown-cli exclusions create`  -  Use this API endpoint to add exclusions.
- `threatdown-cli exclusions delete`  -  Delete exclusion by the exclusion ID.
- `threatdown-cli exclusions get`  -  Get details of an exclusion by the exclusion ID
- `threatdown-cli exclusions list`  -  Get all exclusions
- `threatdown-cli exclusions update`  -  Update an exclusion by the exclusion ID

**firewall**  -  Manage firewall

- `threatdown-cli firewall policies-assign-policies`  -  Set policies for firewall policy
- `threatdown-cli firewall policies-create`  -  Create a new firewall policy
- `threatdown-cli firewall policies-delete`  -  Delete a firewall policy
- `threatdown-cli firewall policies-get`  -  Get a firewall policy
- `threatdown-cli firewall policies-history`  -  Get firewall policy history
- `threatdown-cli firewall policies-list`  -  Get firewall policies
- `threatdown-cli firewall policies-remove-rulesets`  -  Delete rulesets from firewall policy
- `threatdown-cli firewall policies-set-rulesets`  -  Set rulesets for firewall policy
- `threatdown-cli firewall policies-unassign-policies`  -  Delete policies from firewall policy
- `threatdown-cli firewall policies-update`  -  Update a firewall policy
- `threatdown-cli firewall rules-create`  -  Create a new firewall rule
- `threatdown-cli firewall rules-delete`  -  Delete a firewall rule
- `threatdown-cli firewall rules-get`  -  Get a firewall rule
- `threatdown-cli firewall rules-list`  -  Get firewall rules
- `threatdown-cli firewall rules-update`  -  Update a firewall rule
- `threatdown-cli firewall rulesets-create`  -  Create a new firewall ruleset
- `threatdown-cli firewall rulesets-delete`  -  Delete a firewall ruleset
- `threatdown-cli firewall rulesets-get`  -  Get a firewall ruleset
- `threatdown-cli firewall rulesets-list`  -  Get firewall rulesets
- `threatdown-cli firewall rulesets-remove-rules`  -  Delete rules from firewall ruleset
- `threatdown-cli firewall rulesets-set-rules`  -  Set rules for firewall ruleset
- `threatdown-cli firewall rulesets-update`  -  Update a firewall ruleset

**flight-recorder**  -  EDR customers can use Flight Recorder to search event data captured on endpoints that have suspicious activity monitoring enabled.

- `threatdown-cli flight-recorder chart`  -  Search occurrence of suspicious activity. Wildcards are not supported.
- `threatdown-cli flight-recorder endpoints`  -  Search flight recorder for endpoints that had processes with the selected parameters.
- `threatdown-cli flight-recorder process-graph`  -  Search the process graph.
- `threatdown-cli flight-recorder processes`  -  Retrieve a list of processes matching the search input for the specified endpoint.
- `threatdown-cli flight-recorder raw-events`  -  Retrieve a list of raw events matching the given filters for the specified endpoint.
- `threatdown-cli flight-recorder recap`  -  Search suspicious activity. Wildcards are not supported.
- `threatdown-cli flight-recorder search`  -  Search the flight recorder with advanced queries.

**grid**  -  Using the following API, you can search endpoints, detections, software inventory, vulnerabilities, rid rules, os-patches, device control events and dns logs.

- `threatdown-cli grid action-logs-export`  -  Export action logs
- `threatdown-cli grid action-logs-export-async`  -  Export action logs asynchronously
- `threatdown-cli grid action-logs-search`  -  Search action logs
- `threatdown-cli grid action-logs-search-groupby`  -  Search grouped action logs
- `threatdown-cli grid ai-domains-export`  -  Grid operation the vendor has not documented: POST /oneview/v1/grid/ai-domains/export
- `threatdown-cli grid ai-domains-export-async`  -  Grid operation the vendor has not documented: POST /oneview/v1/grid/ai-domains/export/async
- `threatdown-cli grid ai-domains-search`  -  Grid operation the vendor has not documented: POST /oneview/v1/grid/ai-domains/search
- `threatdown-cli grid ai-domains-search-groupby`  -  Grid operation the vendor has not documented: POST /oneview/v1/grid/ai-domains/search-groupby
- `threatdown-cli grid assets-software-export`  -  Export software assets across accounts
- `threatdown-cli grid assets-software-export-async`  -  Export software assets across accounts
- `threatdown-cli grid assets-software-search`  -  Search software assets across accounts
- `threatdown-cli grid assets-software-search-groupby`  -  Search grouped software assets across accounts
- `threatdown-cli grid cve-events-export`  -  Grid operation the vendor has not documented: POST /oneview/v1/grid/cve/events/export
- `threatdown-cli grid cve-events-search`  -  Grid operation the vendor has not documented: POST /oneview/v1/grid/cve/events/search
- `threatdown-cli grid cve-events-search-groupby`  -  Grid operation the vendor has not documented: POST /oneview/v1/grid/cve/events/search-groupby
- `threatdown-cli grid cve-export`  -  Export CVEs across accounts
- `threatdown-cli grid cve-export-async`  -  Export CVEs across accounts
- `threatdown-cli grid cve-search`  -  Search CVEs across accounts
- `threatdown-cli grid cve-search-groupby`  -  Search grouped CVEs across accounts
- `threatdown-cli grid detections-export`  -  Export detections across accounts
- `threatdown-cli grid detections-export-async`  -  Export detections across accounts
- `threatdown-cli grid detections-search`  -  Search detections across accounts
- `threatdown-cli grid detections-search-groupby`  -  Search grouped detections across accounts
- `threatdown-cli grid device-control-export`  -  Search DNS activity across accounts
- `threatdown-cli grid device-control-export-async`  -  Export device control events asynchronously
- `threatdown-cli grid device-control-search`  -  Search device control events across accounts
- `threatdown-cli grid device-control-search-groupby`  -  Search grouped device control events across accounts
- `threatdown-cli grid dns-export`  -  Export DNS activity across accounts
- `threatdown-cli grid dns-export-async`  -  Export DNS activity across accounts
- `threatdown-cli grid dns-search`  -  Search DNS activity across accounts
- `threatdown-cli grid dns-search-groupby`  -  Search grouped DNS activity across accounts
- `threatdown-cli grid endpoints-delete`  -  Delete endpoints across accounts
- `threatdown-cli grid endpoints-export`  -  Export endpoints across accounts
- `threatdown-cli grid endpoints-export-async`  -  Export endpoints across accounts
- `threatdown-cli grid endpoints-search`  -  Search endpoints across accounts
- `threatdown-cli grid endpoints-search-groupby`  -  Search grouped endpoints across accounts
- `threatdown-cli grid events-export`  -  Export event data
- `threatdown-cli grid events-export-async`  -  Export events asynchronously
- `threatdown-cli grid events-search`  -  Search events
- `threatdown-cli grid events-search-groupby`  -  Search and group events
- `threatdown-cli grid firewall-export`  -  Export firewall activity
- `threatdown-cli grid firewall-search`  -  Search firewall activity
- `threatdown-cli grid firewall-search-groupby`  -  Search grouped firewall activity
- `threatdown-cli grid groups-assign-endpoints`  -  Move endpoints between groups across accounts
- `threatdown-cli grid jobs-bulk`  -  Issue jobs to endpoints across accounts
- `threatdown-cli grid os-patches-export`  -  Export OS Patches across accounts
- `threatdown-cli grid os-patches-export-async`  -  Export OS Patches across accounts
- `threatdown-cli grid os-patches-search`  -  Search OS Patches across accounts
- `threatdown-cli grid os-patches-search-groupby`  -  Search grouped OS Patches across accounts
- `threatdown-cli grid rid-export`  -  Export RID rules across accounts
- `threatdown-cli grid rid-export-async`  -  Export RID rules across accounts
- `threatdown-cli grid rid-search`  -  Search RID rules across accounts
- `threatdown-cli grid rid-search-groupby`  -  Search grouped RID rules across accounts
- `threatdown-cli grid xdr-alerts-export`  -  Export XDR Alerts across accounts
- `threatdown-cli grid xdr-alerts-export-async`  -  Export XDR Alerts across accounts
- `threatdown-cli grid xdr-alerts-search`  -  Search XDR Alerts across accounts
- `threatdown-cli grid xdr-alerts-search-groupby`  -  Search grouped XDR Alerts across accounts

**groups**  -  Groups in OneView are used to contain and organize endpoints.

- `threatdown-cli groups assign-endpoints`  -  Use this endpoint to move machines in a group.
- `threatdown-cli groups create`  -  Create group.
- `threatdown-cli groups delete`  -  Delete group by the Group id.
- `threatdown-cli groups get`  -  Get group by the Group id.
- `threatdown-cli groups list`  -  Get all groups in an account.
- `threatdown-cli groups search`  -  Filter groups across accounts
- `threatdown-cli groups update`  -  Update group.

**installation-tokens**  -  Use these APIs to generate, send, and revoke installation tokens used to activate Mobile Security for Business.

- `threatdown-cli installation-tokens create`  -  Generate an installation token to be used. The token will be only visible during this operation.
- `threatdown-cli installation-tokens revoke`  -  Revoke an installation token that has not been used yet.
- `threatdown-cli installation-tokens search`  -  List the installation tokens generated or used across multiple accounts
- `threatdown-cli installation-tokens send`  -  Send installation tokens to batches of emails

**installers**  -  The Installers APIs allow you to deploy the endpoint agent to Windows and macOS devices.

- `threatdown-cli installers <account_id>`  -  Endpoint software can be installed in the following ways: - Active Directory (AD)

**notifications**  -  This API lets you create notification subscriptions.

- `threatdown-cli notifications bulk-delete`  -  Delete multiple notification subscriptions in bulk.
- `threatdown-cli notifications bulk-update`  -  Update multiple notification subscriptions in bulk.
- `threatdown-cli notifications change-owner`  -  Change notification owner.
- `threatdown-cli notifications create`  -  Create notification subscription.
- `threatdown-cli notifications delete`  -  Delete notification by subscription id.
- `threatdown-cli notifications delete-by-owner`  -  Delete all notifications owned by a given owner.
- `threatdown-cli notifications get`  -  Get notification by subscription id.
- `threatdown-cli notifications list`  -  Get all notification subscriptions.
- `threatdown-cli notifications remove-email`  -  Remove an email address from all the recipients of the notification subscriptions of the account.
- `threatdown-cli notifications update`  -  Update notification subscription.

**os-patches**  -  OS Patches

- `threatdown-cli os-patches export`  -  Export OS Patch data. You can export across multiple accounts. Specify your interested fields and the output format.
- `threatdown-cli os-patches export-async`  -  Export OS Patches asynchronously
- `threatdown-cli os-patches search`  -  Search OS Patches.
- `threatdown-cli os-patches search-groupby`  -  Search OS Patches grouped by a specific field.

**policies**  -  A policy is a set of configurations that determine how the endpoint agent monitors your endpoints, such as protection and scan settings.

- `threatdown-cli policies auto-isolation-get`  -  Get policy auto-isolation options by policy ID
- `threatdown-cli policies auto-isolation-set`  -  Set policy auto-isolation options by policy ID
- `threatdown-cli policies clone`  -  Clone policy by id of an existing policy.
- `threatdown-cli policies create`  -  Create new Policy
- `threatdown-cli policies create-v2`  -  Create policy.
- `threatdown-cli policies delete`  -  Delete policy. by its id
- `threatdown-cli policies delete-v2`  -  Delete policy.
- `threatdown-cli policies expire-endpoints-disable`  -  Configure policy not to expire when inactive
- `threatdown-cli policies expire-endpoints-set`  -  Set property to expire endpoints on a policy
- `threatdown-cli policies get`  -  Get one policy by its id
- `threatdown-cli policies get-v2`  -  Get policy by id.
- `threatdown-cli policies isolation-image-add`  -  This route allows you to add an icon to display on the custom message screen.
- `threatdown-cli policies isolation-image-delete`  -  Delete policy isolation image.
- `threatdown-cli policies isolation-image-get`  -  Get policy isolation image by id.
- `threatdown-cli policies list`  -  Get all policies
- `threatdown-cli policies list-v2`  -  Get all policies.
- `threatdown-cli policies promote`  -  Assign policies to site accounts
- `threatdown-cli policies promote-v2`  -  Promote policy.
- `threatdown-cli policies search`  -  Get policies of multiple accounts
- `threatdown-cli policies secret-get`  -  Get policy tamper protection password
- `threatdown-cli policies secret-get-v2`  -  Get policy secret by id.
- `threatdown-cli policies secret-set`  -  Set policy tamper protection password
- `threatdown-cli policies secret-set-v2`  -  Change policy secret by id.
- `threatdown-cli policies set-default`  -  Mark policy as default
- `threatdown-cli policies system-default`  -  Get system default policy.
- `threatdown-cli policies tamper-protection`  -  Returns the effective tamper protection level (off, policy or site)
- `threatdown-cli policies update`  -  Update policy. by its id
- `threatdown-cli policies update-v2`  -  Update policy.

**preferences**  -  The Preferences APIs allow you to enable or disable all notifications of a specific type (email, webhook, slack, teams), without needing to modify or delete multiple notifications.

- `threatdown-cli preferences account-logo-get`  -  Get account logo preferences
- `threatdown-cli preferences account-logo-update`  -  Put account logo preferences
- `threatdown-cli preferences notifications-get`  -  Get notification delivery preferences.
- `threatdown-cli preferences notifications-update`  -  Modify notification delivery preferences.

**quarantine**  -  When a harmful file is found on a device, it can be neutralized and placed in quarantine, preventing it from posing a threat.

- `threatdown-cli quarantine export`  -  Export quarantined items data. You can export only per one account at a time.
- `threatdown-cli quarantine list`  -  Get all quarantined items scoped by account. Refer to the Jobs apis to restore and delete items from quarantine
- `threatdown-cli quarantine search`  -  Search all quarantined items scoped by (parent) account. It's possible to add some constraints to filter the result

**reports**  -  Reports

- `threatdown-cli reports change-owner`  -  Change reports owner.
- `threatdown-cli reports create`  -  Create report.
- `threatdown-cli reports delete`  -  Delete report by id.
- `threatdown-cli reports delete-bulk`  -  Delete reports bulk.
- `threatdown-cli reports generate`  -  Generate report.
- `threatdown-cli reports get`  -  Get report by id.
- `threatdown-cli reports list`  -  Get all reports of the OneView account.
- `threatdown-cli reports update`  -  Update report.

**rid**  -  Manage rid

- `threatdown-cli rid export`  -  Export RID rules
- `threatdown-cli rid export-async`  -  Export RID rules asynchronously
- `threatdown-cli rid search`  -  Search RID rules
- `threatdown-cli rid search-groupby`  -  Search grouped RID

**schedules**  -  Scheduled scans allow you to automate scans to run based on a certain schedule, rather than manually triggering a scan.

- `threatdown-cli schedules create`  -  Create schedule. you must use 'Add groups for schedule' to add a group route.
- `threatdown-cli schedules delete`  -  Delete a schedule by its schedule id.
- `threatdown-cli schedules get`  -  Get schedule by the schedule id.
- `threatdown-cli schedules groups`  -  Add groups to a schedule.
- `threatdown-cli schedules list`  -  Get schedules. You must create a separate request 'Get schedule by id' to get the assigned groups of a schedule.
- `threatdown-cli schedules search`  -  Get schedules of multiple accounts
- `threatdown-cli schedules update`  -  Update schedule. You must use 'add groups for schedule' to update group route.

**sites**  -  Create and manage sites through APIs.

- `threatdown-cli sites by-account`  -  Get one site by the unique field nebula account_id
- `threatdown-cli sites create`  -  Create a new OneView site
- `threatdown-cli sites delete`  -  Delete a OneView site
- `threatdown-cli sites get`  -  Get one site by its unique identifier
- `threatdown-cli sites list`  -  Get a list of all your managed sites
- `threatdown-cli sites mdr-get`  -  Get a site's MDR settings
- `threatdown-cli sites mdr-set`  -  Set a site's MDR settings
- `threatdown-cli sites secret`  -  Returns the site (child account) level tamper protection password for a OneView site.
- `threatdown-cli sites update`  -  Update a OneView site

**software**  -  Manage software

- `threatdown-cli software export`  -  Export assets software data. You can export across multiple accounts.
- `threatdown-cli software export-async`  -  Export assets software asynchronously
- `threatdown-cli software search`  -  Search assets software across all your managed account.
- `threatdown-cli software search-groupby`  -  Search assets software grouped by a specific field across all your managed account.

**subscriptions**  -  Subscriptions allow you to enable and manage ThreatDown security services for your customers by setting terms, volume and duration.

- `threatdown-cli subscriptions addons-get`  -  [v2 - À la carte] Get site's add-on subscriptions
- `threatdown-cli subscriptions addons-update`  -  Use this endpoint to add or modify a current site's add-on subscription * Please note the product values are not static
- `threatdown-cli subscriptions all`  -  Get all subscriptions data by site id.
- `threatdown-cli subscriptions availability-v2`  -  Use this endpoint to retrieve the available subscriptions for a given MSP account **Field descriptions** **products** |
- `threatdown-cli subscriptions availability-v3`  -  Use this endpoint to retrieve the available **Bundle** subscriptions for a given MSP account **Field descriptions**
- `threatdown-cli subscriptions create`  -  Use this endpoint to create a subscription for one of your manages site.
- `threatdown-cli subscriptions create-v2`  -  Use this endpoint to create a subscription for one of your managed sites.
- `threatdown-cli subscriptions create-v3`  -  Use this endpoint to create a subscription for one of your managed sites.
- `threatdown-cli subscriptions delete`  -  Delete your site subscriptions. This will mark the subscription as expired
- `threatdown-cli subscriptions entitlements`  -  Get all entitlements
- `threatdown-cli subscriptions expiry-report`  -  Get information about your customer subscriptions
- `threatdown-cli subscriptions extend`  -  Extend a trial for an existing site.
- `threatdown-cli subscriptions get`  -  Get subscription data by site id.
- `threatdown-cli subscriptions get-v2`  -  Get active subscriptions for an existing site.
- `threatdown-cli subscriptions get-v3`  -  Use this endpoint to fetch the subscription for one of your managed sites.
- `threatdown-cli subscriptions quantity`  -  Get information about the usage of sites
- `threatdown-cli subscriptions update`  -  Use this endpoint to upgrade/downgrade or change term types or seats
- `threatdown-cli subscriptions update-v2`  -  Use this endpoint to add or modify a current site subscription * The user may add or remove a trial through this api *

**suspicious-activity**  -  Suspicious Activities are found using Endpoint Detection and Response (EDR).

- `threatdown-cli suspicious-activity close`  -  Suspicious Activity operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/sa/close
- `threatdown-cli suspicious-activity details`  -  Fetch details of suspicious activity.
- `threatdown-cli suspicious-activity list`  -  Fetch suspicious activity records of the account.
- `threatdown-cli suspicious-activity open`  -  Suspicious Activity operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/sa/open
- `threatdown-cli suspicious-activity process-graph`  -  Fetch process graph of suspicious activity.
- `threatdown-cli suspicious-activity remediate`  -  Suspicious Activity operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/sa/remediate

**syslog**  -  Syslog

- `threatdown-cli syslog create`  -  Create new syslog config record.
- `threatdown-cli syslog get`  -  Get syslog config record by account id and config id.
- `threatdown-cli syslog list`  -  Get syslog config records by account id.
- `threatdown-cli syslog machine-enable`  -  Set syslog machine enabled/disabled.
- `threatdown-cli syslog machines-assign`  -  Assign syslog config machines.
- `threatdown-cli syslog search`  -  Search syslog configs by multiple accounts.
- `threatdown-cli syslog update`  -  Update syslog config by account_id and config id.

**usage**  -  Usage

- `threatdown-cli usage changelogs`  -  Get entitlement history for an endpoint
- `threatdown-cli usage histogram`  -  Search usage histogram
- `threatdown-cli usage logs`  -  Search logs of usage
- `threatdown-cli usage logs-summary`  -  Search summary of usage
- `threatdown-cli usage logs-summary-export`  -  Export usage data
- `threatdown-cli usage two-tier-logs-summary-export`  -  Two tier usage summary export

**users**  -  Provision and manage OneView users through APIs.

- `threatdown-cli users create`  -  Create a new OneView User
- `threatdown-cli users delete`  -  Delete one user by its id
- `threatdown-cli users get`  -  Get user by ID
- `threatdown-cli users list`  -  Get all users
- `threatdown-cli users resend-invite`  -  Resend user invite
- `threatdown-cli users update`  -  Update data about a user

**vulnerability-assessment**  -  Vulnerability Assessment

- `threatdown-cli vulnerability-assessment cve-stats`  -  Get vulnerability assessment CVE statistics for accounts
- `threatdown-cli vulnerability-assessment details`  -  Get vulnerability assessment details for accounts
- `threatdown-cli vulnerability-assessment report`  -  Get vulnerability assessment report for accounts
- `threatdown-cli vulnerability-assessment software-stats`  -  Get vulnerability assessment software statistics for accounts
- `threatdown-cli vulnerability-assessment summary`  -  Get vulnerability assessment summary for accounts

**webhooks**  -  ThreatDown can send webhook events that notify your application any time an event happens on your account.

- `threatdown-cli webhooks create`  -  Use this api endpoint to create a webhook subscription for a given account.
- `threatdown-cli webhooks delete`  -  Delete a webhook subscription if you want to stop receiving notifications.
- `threatdown-cli webhooks get`  -  Retrieve a subscription by its unique identifier.
- `threatdown-cli webhooks list`  -  Retrieve all webhook subscriptions of your account or your managed accounts.
- `threatdown-cli webhooks update`  -  Update a subscription by its unique identifier.


**customer**


- `threatdown-cli customer report`  -  Everything about one customer site in one output: health, stale machines, top threats, isolated endpoints and pending jobs.


**fleet**


- `threatdown-cli fleet stale`  -  Every endpoint not seen in N hours across all your sites, with site name and exact hours since last seen.

- `threatdown-cli fleet health`  -  One ranked row per site with total, online, stale, infected, suspicious, isolated, reboot-required, unprotected, scan-overdue and update-available counts plus an attention score.

- `threatdown-cli fleet sync`  -  Pull sites, endpoints, detections and jobs from every managed site into one local database.


**respond**


- `threatdown-cli respond isolate`  -  Isolate endpoints by host name with a target preview and a 25-endpoint blast-radius guard; sibling commands respond restore, respond scan, respond reboot and respond update work the same way.

- `threatdown-cli respond pending`  -  Jobs that timed out, expired or failed, plus jobs still waiting after --min-age (default 1h), grouped by site and command with a re-issue hint.


**seats**


- `threatdown-cli seats reconcile`  -  Per-site seat allocations compared with deployed and recently seen endpoints, with expiry flags.


**threats**


- `threatdown-cli threats spread`  -  Every threat in a window ranked by reach (distinct sites and endpoints), by hash or threat name; add --min-sites 2 for cross-site threats only.

- `threatdown-cli threats queue`  -  Prioritized list of infected or suspicious endpoints with online and isolated state and a suggested response.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
threatdown-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Monday dark-machine sweep

```bash
threatdown-cli fleet stale --hours 24 --agent
```

Lists every endpoint not seen in 24 hours with its site and hours since last seen.

### Narrow a large site list

```bash
threatdown-cli sites list --agent --select company_name,account_id
```

Pulls only the site name and Nebula account id from the full site payload; pass those ids to --account-ids on the generated search commands.

### Contain an infected laptop safely

```bash
threatdown-cli respond isolate --host LAPTOP-042
```

Resolves the exact host name across all sites and prints a preview; nothing is sent until you add --confirm (or the global --yes). More than 25 targets (--max-targets) is refused, a host name that matches more than one endpoint is refused unless you narrow it with --site or pass --all-matches, the global --dry-run is not the preview, and issuing needs the execute OAuth scope.

### Release an endpoint from isolation

```bash
threatdown-cli respond restore --host LAPTOP-042
```

Ends network isolation (command.edr.restore; it does not restore quarantined files) and skips endpoints that are not isolated. respond scan runs a threat scan (--remove quarantines, --scan-type ThreatScan|HyperScan), respond reboot restarts, respond update refreshes protection definitions.

### Refresh the local copy, then triage

```bash
threatdown-cli fleet sync --since 7d
```

Required before threats spread, threats queue and customer report, which read only this copy; then run threats queue --since 7d --agent --select queue.host_name,queue.kind,queue.suggested_command.

### Find jobs that never ran

```bash
threatdown-cli respond pending --since 7d --agent
```

Shows timed-out, expired and failed jobs plus jobs still waiting after --min-age (default 1h), grouped by site with a re-issue command.

## Auth Setup

OneView uses OAuth2 client credentials. Create an API client in OneView (Integrate), then set THREATDOWN_CLIENT_ID and THREATDOWN_CLIENT_SECRET. The CLI exchanges them at https://api.threatdown.com/oneview/oauth2/token with HTTP Basic client authentication and caches the bearer token. It requests the read, write and execute scopes by default; set THREATDOWN_OAUTH_SCOPE (for example 'read write') to request fewer. The respond commands need the execute scope. The cached token is used only for the same client ID and scope: change THREATDOWN_OAUTH_SCOPE or the client and the CLI requests a new token, and a process with no client secret refuses to borrow another client's cached token. A 403 means either the OAuth scope or the creating user's OneView permission is missing. For a reporting-only MCP server, set THREATDOWN_MCP_READ_ONLY=1: it drops the write tools and refuses every non-GET call, and endpoints diag-log, through threatdown_execute.

Run `threatdown-cli doctor` to verify setup.

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
  threatdown-cli account-info --agent --select account_id,nebula_parent_account_id
  ```
- **Previewable**  -  `--dry-run` shows the request without sending (for `respond` verbs the real preview is the command without `--confirm`)
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

- Use `--home <dir>` for one invocation, or set `THREATDOWN_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `THREATDOWN_CONFIG_DIR`, `THREATDOWN_DATA_DIR`, `THREATDOWN_STATE_DIR`, `THREATDOWN_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `THREATDOWN_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `threatdown-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "threatdown": {
        "command": "threatdown-mcp",
        "env": {
          "THREATDOWN_HOME": "/srv/threatdown"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `THREATDOWN_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `THREATDOWN_HOME`, or `doctor` will not find credentials left under the former root.

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
threatdown-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "threatdown-cli learnings confirm 12"] }
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
       materially more, record the divergence via `threatdown-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `threatdown-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `threatdown-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
threatdown-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
threatdown-cli teach \
  --query "$QUERY" \
  --resource-type <type> \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
threatdown-cli teach-playbook \
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
threatdown-cli playbook amend \
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

`threatdown-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `THREATDOWN_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
threatdown-cli feedback "the --since flag is inclusive but docs say exclusive"
threatdown-cli feedback --stdin < notes.txt
threatdown-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `THREATDOWN_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `THREATDOWN_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
threatdown-cli profile save briefing --json
threatdown-cli --profile briefing account-info
threatdown-cli profile list --json
threatdown-cli profile show briefing
threatdown-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Async Jobs

`respond <verb> --wait` is separate from the flags below: it polls for up to `--wait-timeout` (default 2m) and warns, without failing, when jobs are still running; follow up with `threatdown-cli respond pending`.

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

1. **Empty, `help`, or `--help`** → show `threatdown-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add threatdown-mcp --env THREATDOWN_CLIENT_ID=<your-client-id> --env THREATDOWN_CLIENT_SECRET=<your-client-secret> -- threatdown-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which threatdown-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   threatdown-cli fleet health --agent
   ```
4. If ambiguous, drill into subcommand help, e.g. `threatdown-cli respond isolate --help`.
