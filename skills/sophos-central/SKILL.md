---
name: sophos-central
description: "Use when the user asks to check Sophos Central across every customer: a ranked triage queue of alerts and unhealthy machines, Account Health Check rankings, security and licence gaps, tamper protection off, partner billing against deployment, or to isolate a machine by hostname without knowing its tenant. Covers the Sophos Central partner, organization and tenant APIs with tenant and region routing handled for you. Trigger phrases: `check sophos alerts across all customers`, `which sophos endpoints have tamper protection off`, `isolate this machine in sophos`, `sophos health check scores for my tenants`, `reconcile sophos partner billing`, `use sophos central`, `run sophos-central`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Sophos Central"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - sophos-central-cli
    install:
      - kind: script
        bins: [sophos-central-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.ps1
---

# Sophos Central  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `sophos-central-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.ps1 | iex
   ```
3. Verify: `sophos-central-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `sophos-central-cli` and `sophos-central-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Covers the Sophos Central (now Sophos Fusion) partner, organization and tenant REST APIs: endpoints, alerts, policies, exclusions, Live Discover, XDR queries, SIEM, account health check, licensing and audit events. whoami, the X-Partner-ID / X-Organization-ID / X-Tenant-ID headers and each tenant's regional host are handled for you. fleet refresh fills a local store for every customer so fleet triage, fleet health and fleet gaps answer cross-tenant questions offline; fleet tamper, fleet siem-collect and fleet revenue work live across tenants, and contain isolates a device by hostname without looking up the tenant first.

## When to Use This CLI

Use this CLI when an agent or MSP technician needs to read or act on Sophos Central (Sophos Fusion) data: endpoints, alerts, policies, exclusions, Live Discover or XDR queries, SIEM events, health checks, licences or partner billing. It is the right tool for cross-tenant questions (which customers have unhealthy devices, tamper protection off, low health scores, billing variance) after fleet refresh has filled the local store, and for isolating a device by hostname across customers.

## Anti-triggers

Do not use this CLI for:
- Do not use it for Sophos Firewall device configuration (XG/XGS local API); use the firewall's own API or Sophos Firewall Manager.
- Do not use it for Sophos Cases or Detections, classic REST or Fusion XDR GraphQL; those APIs are not included.
- Do not use it to manage Sophos Email, Mobile, DNS Protection, Wireless or Cloud Optix; those APIs are not included.
- Do not use it for non-Sophos EDR products; use the matching vendor connector.

## Unique Capabilities

### Cross-tenant visibility
- **`fleet refresh`**  -  Refresh tenants, endpoints, alerts, account health and licences for every non-trial customer tenant in one run, with a per-tenant ok/forbidden/error ledger (add --include-trial for trial tenants).

  _Run it first: fleet triage, fleet health, fleet gaps and contain's hostname lookup answer from the store it fills; the other fleet commands call the API on each run._

  ```bash
  sophos-central-cli fleet refresh --resources tenants,endpoints,alerts,health,licenses
  ```
- **`fleet triage`**  -  One ranked work queue across all customers: open high and medium alerts, bad or suspicious endpoints, tamper protection off, isolated or stale devices.

  _Use it for the morning check of what needs attention today across every tenant._

  ```bash
  sophos-central-cli fleet triage --severity high,medium --stale 14d --agent
  ```

### Posture and revenue
- **`fleet health`**  -  Rank customers worst-first by Account Health Check score with sub-scores and the change since the previous fleet refresh.

  _Use it before a QBR or weekly review to see which customers' posture is slipping._

  ```bash
  sophos-central-cli fleet health --limit 10
  ```
- **`fleet gaps`**  -  Per-customer security and commercial gaps: low protection, partial tamper protection, risky exclusions, unencrypted or unlicensed devices, licence over-usage or expiry.

  _Use it to find fixable risks and upsell opportunities per customer._

  ```bash
  sophos-central-cli fleet gaps --threshold 80 --kind security
  ```
- **`fleet revenue`**  -  Rank customers by monthly Sophos revenue for a billing month (partner credential only) and compare billed quantities with deployed endpoints counted by the last fleet refresh.

  _Use it at month-end to reconcile Sophos billing against what is deployed; it calls the billing API live and needs a partner credential._

  ```bash
  sophos-central-cli fleet revenue --month 2026-09 --limit 20
  ```

### Response at fleet scale
- **`contain`**  -  Isolate (contain) machines by hostname, IP or endpoint ID without knowing the tenant: resolves the tenant from the fleet refresh index (or searches one --tenant live), previews isolation plus optional scan, forensic log and alert acknowledgement, and runs only with --yes.

  _Use it during an incident to isolate or quarantine a device when you have a hostname, not a tenant; run fleet refresh beforehand so the hostname index exists._

  ```bash
  sophos-central-cli contain WS-FIN-07 --scan --forensic-log --ack-alerts
  ```
- **`fleet tamper`**  -  Find supported endpoints with tamper protection off across non-trial customers and, with --yes, turn it back on; tenants whose tenant-wide tamper setting is off are reported, not changed.

  _Use it for the weekly tamper-protection hygiene pass; it previews unless --yes is given and reads live, not from the store._

  ```bash
  sophos-central-cli fleet tamper
  ```
- **`fleet siem-collect`**  -  Pull SIEM events and alerts for every tenant from each tenant's saved cursor as NDJSON, warning when a cursor fell outside the 24-hour window (CLI and cron only; not exposed as an MCP tool).

  _Use it from cron to feed a SIEM for all customers without losing data silently._

  ```bash
  sophos-central-cli fleet siem-collect --stream both
  ```

## Command Reference

**admins**  -  Manage admins

- `sophos-central-cli admins create-role-assignment`  -  Assign a role of principal type 'user' to a tenant admin. Any existing assignment is overridden.
- `sophos-central-cli admins create-tenant`  -  Create a tenant admin from a directory user.
- `sophos-central-cli admins deprovision-by-id`  -  Remove an admin by ID.
- `sophos-central-cli admins get-role-assignment-by-id`  -  Get tenant admin role assignment by ID.
- `sophos-central-cli admins get-tenant-by-id`  -  Get tenant admin details by ID.
- `sophos-central-cli admins list-role-assignments`  -  Get the list of role assignments for given admin.
- `sophos-central-cli admins list-tenant`  -  List all tenant admins.
- `sophos-central-cli admins remove-role-assignment-by-id`  -  Remove role assignment from an admin.

**alerts**  -  List open alerts, search alerts, and perform actions such as resolving alerts.

- `sophos-central-cli alerts act-on`  -  Take an action on a specific alert.
- `sophos-central-cli alerts get`  -  Get details of a specific alert.
- `sophos-central-cli alerts list`  -  Get alerts matching criteria in query parameters.
- `sophos-central-cli alerts search`  -  Get alerts matching request.

**allowed_items**  -  Manage allowed items. These are always treated as non-malicious.

- `sophos-central-cli allowed-items delete-by-id`  -  Deletes the specified allowed item.
- `sophos-central-cli allowed-items get`  -  Get all allowed items.
- `sophos-central-cli allowed-items get-by-id`  -  Get an allowed item by ID.
- `sophos-central-cli allowed-items patch-by-id`  -  Update an allowed item.
- `sophos-central-cli allowed-items post-by-id`  -  Exempt an item from conviction.

**audit_events**  -  List and read audit events.

- `sophos-central-cli audit-events`  -  List audit events.

**billing**  -  Manage billing

- `sophos-central-cli billing`  -  Get usage report

**blocked_addresses**  -  Manage blocked IP addresses for a customer.

- `sophos-central-cli blocked-addresses delete-blocked-address-sets`  -  Remove multiple IP addresses or IP address ranges from being blocked.
- `sophos-central-cli blocked-addresses get-blocked-address-sets`  -  Get the list of all currently blocked IP addresses and IP address ranges.
- `sophos-central-cli blocked-addresses post-blocked-address-sets`  -  Allows the user to specify multiple IP addresses or IP address ranges that devices should refuse to communicate with.

**blocked_items**  -  Manage blocked items. These are always treated as malicious or potentially malicious.

- `sophos-central-cli blocked-items delete-by-id`  -  Deletes the specified blocked item.
- `sophos-central-cli blocked-items get`  -  Get all blocked items.
- `sophos-central-cli blocked-items get-by-id`  -  Get a blocked item by ID.
- `sophos-central-cli blocked-items post-by-id`  -  Block an item from exoneration.

**device_software**  -  Manage device software.

- `sophos-central-cli device-software`  -  Update assigned endpoint device software in bulk.

**endpoint_groups**  -  Manage endpoint groups

- `sophos-central-cli endpoint-groups add-multiple-endpoints-to-group`  -  Add endpoints to your group.
- `sophos-central-cli endpoint-groups create-tenant`  -  Add new endpoint group to the directory.
- `sophos-central-cli endpoint-groups delete-tenant-by-id`  -  Delete endpoint group.
- `sophos-central-cli endpoint-groups get-tenant-by-id`  -  Get endpoint group by ID.
- `sophos-central-cli endpoint-groups list-tenant`  -  Endpoint groups in the directory.
- `sophos-central-cli endpoint-groups list-tenant-by-group-type`  -  Endpoint groups of your specified type in the directory.
- `sophos-central-cli endpoint-groups list-tenant-endpoints-by-id`  -  Returns the endpoints in your specified group.
- `sophos-central-cli endpoint-groups patch-tenant`  -  Update endpoint group.
- `sophos-central-cli endpoint-groups remove-multiple-endpoints-from-group`  -  Remove endpoints from a group.
- `sophos-central-cli endpoint-groups remove-single-endpoint-from-group`  -  Remove endpoint from a group.

**endpoint_tags**  -  Assign tags to endpoints.

- `sophos-central-cli endpoint-tags`  -  Assign and unassign tags to endpoints.

**endpoints**  -  Search for endpoints, view individual endpoint details, delete an endpoint or restore endpoints.

- `sophos-central-cli endpoints bulk-delete`  -  Delete a list of endpoints.
- `sophos-central-cli endpoints bulk-isolate`  -  Turn on or off endpoint isolation for multiple endpoints.
- `sophos-central-cli endpoints collect-forensic-log`  -  Sends a request to the endpoint to collect forensic log.
- `sophos-central-cli endpoints collect-memory-dump`  -  Instructs the target device to generate a memory dump of the specified process or the kernel.
- `sophos-central-cli endpoints delete`  -  Deletes a specified endpoint.
- `sophos-central-cli endpoints get`  -  Get an endpoint based on ID.
- `sophos-central-cli endpoints get-aap`  -  Get Adaptive Attack Protection settings for an endpoint.
- `sophos-central-cli endpoints get-forensic-log-status`  -  Gets the status of the forensic log collection.
- `sophos-central-cli endpoints get-isolation`  -  Get isolation settings for an endpoint.
- `sophos-central-cli endpoints get-memory-dump-status`  -  Gets the status of the memory dump collection.
- `sophos-central-cli endpoints get-tamper-protection`  -  Get Tamper Protection settings for a specified endpoint.
- `sophos-central-cli endpoints list`  -  Get all the endpoints for the specified tenant.
- `sophos-central-cli endpoints scan`  -  Sends a request to the specified endpoint to perform or configure a scan.
- `sophos-central-cli endpoints set-isolation`  -  Update isolation settings for an endpoint.
- `sophos-central-cli endpoints set-tamper-protection`  -  Turns Tamper Protection on or off on an endpoint. Or generates a new Tamper Protection password.
- `sophos-central-cli endpoints update-aap`  -  Update Adaptive Attack Protection settings for an endpoint.
- `sophos-central-cli endpoints update-check`  -  Sends a request to the endpoint to check for Sophos management agent software updates.

**enrichment_pivots**  -  Manage enrichment pivots

- `sophos-central-cli enrichment-pivots create`  -  Create a new enrichment.
- `sophos-central-cli enrichment-pivots delete`  -  Delete enrichment.
- `sophos-central-cli enrichment-pivots get`  -  Get enrichments.
- `sophos-central-cli enrichment-pivots update`  -  Update an existing enrichment.

**event_journal**  -  Event journal settings for a customer's computers and servers.

- `sophos-central-cli event-journal get-settings`  -  Get all event journal settings.
- `sophos-central-cli event-journal update-settings`  -  Update settings for event journal size and disk space limits.

**exclusions**  -  Manage exclusions

- `sophos-central-cli exclusions add-intrusion-prevention`  -  Add a new Intrusion Prevention exclusion.
- `sophos-central-cli exclusions add-isolation`  -  Adds a new Isolation exclusion.
- `sophos-central-cli exclusions add-scanning`  -  Add a new scanning exclusion.
- `sophos-central-cli exclusions delete-intrusion-prevention-by-id`  -  Delete an Intrusion Prevention exclusion by ID.
- `sophos-central-cli exclusions delete-isolation-by-id`  -  Deletes an Isolation exclusion.
- `sophos-central-cli exclusions delete-scanning-by-id`  -  Deletes a scanning exclusion.
- `sophos-central-cli exclusions get-intrusion-prevention-by-id`  -  Get an Intrusion Prevention exclusion by ID.
- `sophos-central-cli exclusions get-isolation-by-id`  -  Get a single Isolation exclusion by ID.
- `sophos-central-cli exclusions get-scanning-by-id`  -  Get a scanning exclusion by ID.
- `sophos-central-cli exclusions list-intrusion-prevention`  -  Get all Intrusion Prevention exclusions.
- `sophos-central-cli exclusions list-isolation`  -  Get all isolation exclusions.
- `sophos-central-cli exclusions list-scanning`  -  Get all scanning exclusions.
- `sophos-central-cli exclusions update-intrusion-prevention-by-id`  -  Update an Intrusion Prevention exclusion by ID.
- `sophos-central-cli exclusions update-isolation-by-id`  -  Updates an Isolation exclusion by ID.
- `sophos-central-cli exclusions update-scanning-by-id`  -  Update a scanning exclusion by ID.

**exploit_mitigation**  -  Manage Exploit Mitigation settings.

- `sophos-central-cli exploit-mitigation add-application`  -  Exclude a set of file paths from Exploit Mitigation.
- `sophos-central-cli exploit-mitigation delete-application-by-id`  -  Deletes a custom (user-defined) Exploit Mitigation application by ID. Note you can only delete custom applications.
- `sophos-central-cli exploit-mitigation get-application-by-id`  -  Get Exploit Mitigation settings for an application.
- `sophos-central-cli exploit-mitigation get-applications`  -  Get Exploit Mitigation settings for all protected applications.
- `sophos-central-cli exploit-mitigation get-categories`  -  Get Exploit Mitigation categories.
- `sophos-central-cli exploit-mitigation get-detected-exploit-by-id`  -  Get a detected exploit by ID.
- `sophos-central-cli exploit-mitigation get-detected-exploits`  -  Get detected exploits and the number of each detected exploit.
- `sophos-central-cli exploit-mitigation patch-application-by-id`  -  Update Exploit Mitigation settings for an application.

**health_check**  -  Manage health check

- `sophos-central-cli health-check get`  -  Account Health
- `sophos-central-cli health-check get-historical-scores`  -  Get historical health scores.
- `sophos-central-cli health-check get-regional-scores`  -  Get health score for comparison to other organizations in your region.
- `sophos-central-cli health-check post-snooze`  -  Snooze reminders for account health issues for a customer.

**installers**  -  Manage installers

- `sophos-central-cli installers`  -  Get all the endpoint installer links for a tenant.

**licenses**  -  Manage licenses

- `sophos-central-cli licenses list`  -  Get tenant licenses.
- `sophos-central-cli licenses list-firewall`  -  Get firewalls claimed by tenant or managed by the partner.

**live_discover**  -  Run EDR queries on remote endpoints.

- `sophos-central-cli live-discover get-category`  -  Get the details of a category by ID.
- `sophos-central-cli live-discover get-endpoint-execution-status`  -  Get statuses of endpoints in the query run.
- `sophos-central-cli live-discover get-execution-results-by-id`  -  Get the results of a query run.
- `sophos-central-cli live-discover get-execution-summary`  -  Get a query run by ID.
- `sophos-central-cli live-discover get-query`  -  Get a query by ID.
- `sophos-central-cli live-discover get-query-run-page`  -  Get the list of query runs matching the given filters.
- `sophos-central-cli live-discover list-categories`  -  Get all categories, canned as well as custom.
- `sophos-central-cli live-discover list-queries`  -  Get queries matching the given filters.
- `sophos-central-cli live-discover run-query`  -  Run a saved EDR query or an ad hoc query on remote endpoints.

**migrations**  -  Migrate endpoints from one tenant to another.

- `sophos-central-cli migrations create-receiving-job`  -  Start a migration job in the receiving tenant.
- `sophos-central-cli migrations create-sending-job`  -  Start a migration job in the sending tenant.
- `sophos-central-cli migrations get-endpoints`  -  Gets the status of endpoints that are being migrated.
- `sophos-central-cli migrations get-job`  -  Get a single migration job.
- `sophos-central-cli migrations get-jobs`  -  Gets all migration jobs for the tenant.
- `sophos-central-cli migrations get-settings`  -  Get the endpoint migration settings for this tenant.

**org_admins**  -  Manage org admins

- `sophos-central-cli org-admins create-admin-role-assignment`  -  Assign a role to a organization administrator.
- `sophos-central-cli org-admins create-organization-admin`  -  Create a new organization administrator.
- `sophos-central-cli org-admins delete-organization-admin-by-id`  -  Remove an admin by ID.
- `sophos-central-cli org-admins get-admin-role-assignment-by-id`  -  Get organization administrator role assignment by ID.
- `sophos-central-cli org-admins get-admin-role-assignments`  -  Get the list of role assignments for given admin.
- `sophos-central-cli org-admins get-organization-admin-by-id`  -  Get organization administrator details by ID.
- `sophos-central-cli org-admins get-organization-admins`  -  List all organization admins.
- `sophos-central-cli org-admins remove-admin-role-assignment-by-id`  -  Remove role assignment from an admin.

**org_roles**  -  Manage org roles

- `sophos-central-cli org-roles create-organization-role`  -  Create a new organization role.
- `sophos-central-cli org-roles delete-organization-role-by-id`  -  Delete a organization role by ID.
- `sophos-central-cli org-roles get-organization-role-by-id`  -  Get organization role by ID.
- `sophos-central-cli org-roles get-role-permission-set`  -  Get permission set details.
- `sophos-central-cli org-roles list-organization-roles`  -  List all organization roles.
- `sophos-central-cli org-roles patch-organization-role`  -  Patch an existing organization role.

**org_tenants**  -  Manage org tenants

- `sophos-central-cli org-tenants get`  -  Get a tenant by ID.
- `sophos-central-cli org-tenants list`  -  Get all the tenants in the specified organization.

**partner_admins**  -  Create new administrators, update their profile information, enumerate them or view administrator details.

- `sophos-central-cli partner-admins create`  -  Create a new partner administrator.
- `sophos-central-cli partner-admins create-admin-role-assignment`  -  Assign a role to a partner administrator.
- `sophos-central-cli partner-admins delete-by-id`  -  Delete a PDB Admin for Central while internally just revoking PDB access.
- `sophos-central-cli partner-admins get`  -  List all partner admins.
- `sophos-central-cli partner-admins get-admin-role-assignment-by-id`  -  Get partner administrator role assignment by ID.
- `sophos-central-cli partner-admins get-admin-role-assignments`  -  Get the list of role assignments for given admin.
- `sophos-central-cli partner-admins get-partner`  -  Get partner administrator details by ID.
- `sophos-central-cli partner-admins remove-admin-role-assignment-by-id`  -  Remove role assignment from an admin.

**partner_roles**  -  Manage partner roles

- `sophos-central-cli partner-roles create`  -  Create a new partner role.
- `sophos-central-cli partner-roles delete-by-id`  -  Delete a partner role by ID.
- `sophos-central-cli partner-roles get-by-id`  -  Get partner role by ID.
- `sophos-central-cli partner-roles get-role-permission-set`  -  Get permission set details.
- `sophos-central-cli partner-roles list`  -  List all partner roles.
- `sophos-central-cli partner-roles patch`  -  Patch an existing partner role.

**peripherals**  -  Manage peripherals

- `sophos-central-cli peripherals get-by-id`  -  Get a peripheral by ID.
- `sophos-central-cli peripherals get-by-tenant-id`  -  Get all peripherals.

**policies**  -  Manage policies

- `sophos-central-cli policies clone-base-policy`  -  Clone a new policy from the base policy for a policy type.
- `sophos-central-cli policies clone-policy`  -  Clone policy.
- `sophos-central-cli policies create-policy`  -  Create new policy.
- `sophos-central-cli policies delete-policy`  -  Delete policy.
- `sophos-central-cli policies get`  -  Get policies of a tenant.
- `sophos-central-cli policies get-base-policy`  -  Get base policy for a policy type.
- `sophos-central-cli policies get-policy`  -  Policy details.
- `sophos-central-cli policies get-setting-metadatas`  -  Supported metadata.
- `sophos-central-cli policies get-settings-of-base-policy`  -  Get settings of the base policy for a policy type.
- `sophos-central-cli policies get-settings-of-policy`  -  Policy settings.
- `sophos-central-cli policies get-value-of-setting-item-in-base-policy`  -  Get the value of a setting in the base policy for a policy type.
- `sophos-central-cli policies get-value-of-setting-key-in-policy`  -  Get the value of a setting key in a policy.
- `sophos-central-cli policies reset-base-policy-settings`  -  Reset the settings in a base policy.
- `sophos-central-cli policies reset-policy-settings`  -  Reset policy settings.
- `sophos-central-cli policies reset-setting-in-base-policy`  -  Reset a setting in the base policy to its default value.
- `sophos-central-cli policies reset-setting-in-policy`  -  Reset a setting to its default value.
- `sophos-central-cli policies update-base-policy`  -  Update base policy. Note that only settings can be changed.
- `sophos-central-cli policies update-policy`  -  Update policy.
- `sophos-central-cli policies update-settings-of-base-policy`  -  Update settings in the base policy for a policy type.
- `sophos-central-cli policies update-settings-of-policy`  -  Update policy settings.
- `sophos-central-cli policies update-value-of-setting-item-in-base-policy`  -  Update a setting in the base policy.

**provisioning**  -  Get available products and manage default products to use when creating new tenants.

- `sophos-central-cli provisioning list-default-products`  -  List of products to turn on by default when the provisioning request for a tenant doesn't specify them.
- `sophos-central-cli provisioning list-products`  -  List of available products.
- `sophos-central-cli provisioning patch-default-products`  -  Update the list of default products.
- `sophos-central-cli provisioning reset-default-products`  -  Reset the list of default products.

**roles**  -  Manage roles

- `sophos-central-cli roles create-tenant`  -  Create a new tenant role.
- `sophos-central-cli roles delete-tenant-by-id`  -  Delete a tenant role by ID.
- `sophos-central-cli roles get-permission-set`  -  Get permission set details.
- `sophos-central-cli roles get-tenant-by-id`  -  Get tenant role by ID.
- `sophos-central-cli roles list-tenant`  -  List all tenant roles.
- `sophos-central-cli roles patch-tenant`  -  Patch an existing tenant role.

**siem**  -  Manage siem

- `sophos-central-cli siem alerts`  -  Get alert with timestamps within last 24 hours.
- `sophos-central-cli siem events`  -  Get events with timestamps within last 24 hours.

**software**  -  Manage software

- `sophos-central-cli software add-special-static-package`  -  Add a special package by token, supplied by Sophos support. This is a one-way operation.
- `sophos-central-cli software delete-comment`  -  Delete the static package comment.
- `sophos-central-cli software get-comment`  -  Get the static package comment.
- `sophos-central-cli software get-comments`  -  Get all comments.
- `sophos-central-cli software get-recommended-packages`  -  Get all Sophos Recommended packages for the tenant.
- `sophos-central-cli software get-static-package`  -  Get an individual static package.
- `sophos-central-cli software get-static-packages`  -  Get all static packages available for the tenant.
- `sophos-central-cli software update-comment`  -  Add/Update the static package comment.

**tamper_protection**  -  Turn Tamper Protection on or off on an endpoint and view settings.

- `sophos-central-cli tamper-protection`  -  Check whether Tamper Protection is turned on globally.

**tenants**  -  Create new tenants, enumerate them or view tenant details.

- `sophos-central-cli tenants cancel-pending-amendment`  -  Cancel a pending account amendment.
- `sophos-central-cli tenants create`  -  Create a new tenant.
- `sophos-central-cli tenants get`  -  Get a tenant by ID.
- `sophos-central-cli tenants get-account-amendment-by-id`  -  Get account amendment by ID.
- `sophos-central-cli tenants list`  -  List all the tenants for a partner.
- `sophos-central-cli tenants list-account-amendments`  -  List all account amendments.
- `sophos-central-cli tenants post-account-amendment`  -  Make tenant account changes.
- `sophos-central-cli tenants post-product-selection`  -  Update tenant product selection.
- `sophos-central-cli tenants update`  -  Patch a tenant by ID.

**user_groups**  -  Manage user groups

- `sophos-central-cli user-groups add-multiple-users-to-group`  -  Add multiple users to the specified group.
- `sophos-central-cli user-groups create-tenant`  -  Add a new group to the directory.
- `sophos-central-cli user-groups delete-tenant-by-id`  -  Deletes the specified group.
- `sophos-central-cli user-groups get-tenant-by-id`  -  Get group by ID.
- `sophos-central-cli user-groups list-tenant`  -  List groups in the directory.
- `sophos-central-cli user-groups list-tenant-users-by-id`  -  List users in the specified group.
- `sophos-central-cli user-groups patch-tenant`  -  Update a group.
- `sophos-central-cli user-groups remove-multiple-users-from-group`  -  Remove multiple users from a group.
- `sophos-central-cli user-groups remove-single-user-from-group`  -  Update a group to remove a user.

**users**  -  Manage users

- `sophos-central-cli users add-to-multiple-groups`  -  Add a user to multiple groups.
- `sophos-central-cli users create-tenant`  -  Add a new user to the directory.
- `sophos-central-cli users delete-tenant-by-id`  -  Delete a user by ID.
- `sophos-central-cli users get-tenant-by-id`  -  Get a user by ID.
- `sophos-central-cli users list-tenant`  -  List users in the directory.
- `sophos-central-cli users list-tenant-groups-by-id`  -  List groups that a user belongs to.
- `sophos-central-cli users patch-tenant`  -  Update an existing user.
- `sophos-central-cli users remove-from-multiple-groups`  -  Remove a user from multiple groups.
- `sophos-central-cli users remove-from-single-group`  -  Remove user from a group.

**web_control**  -  Manage Web Control settings.

- `sophos-central-cli web-control add-local-site-exclusion`  -  Adds a new local site.
- `sophos-central-cli web-control clear-web-decryption-excluded-sites`  -  Clears the list of websites excluded from SSL/TLS decryption.
- `sophos-central-cli web-control delete-local-sites-by-id`  -  Deletes the specified local site.
- `sophos-central-cli web-control get-local-site-by-id`  -  Get a local site by ID.
- `sophos-central-cli web-control get-web-decryption-settings`  -  Get settings for SSL/TLS decryption of HTTPS websites.
- `sophos-central-cli web-control list-categories`  -  Get all Web Control categories.
- `sophos-central-cli web-control list-local-sites-by-tenant-id`  -  Get all local sites.
- `sophos-central-cli web-control list-web-decryption-excluded-sites`  -  List of websites excluded from SSL/TLS decryption.
- `sophos-central-cli web-control update-local-site-setting-by-id`  -  Update a local site definition.
- `sophos-central-cli web-control update-web-decryption-excluded-sites`  -  Add and remove websites excluded from SSL/TLS decryption.
- `sophos-central-cli web-control update-web-decryption-settings`  -  Update settings for SSL/TLS decryption of HTTPS websites.

**whoami**  -  Whoami API.

- `sophos-central-cli whoami`  -  Returns information about the caller.

**xdr**  -  Manage xdr

- `sophos-central-cli xdr cancel-query-run`  -  Cancel a query run by ID.
- `sophos-central-cli xdr get-category`  -  Get the details of a category.
- `sophos-central-cli xdr get-query`  -  Get the details of a query.
- `sophos-central-cli xdr get-query-results`  -  Get the paged results for a query run.
- `sophos-central-cli xdr get-query-run`  -  Get a query run by ID.
- `sophos-central-cli xdr get-query-runs`  -  Get the list of query runs matching the given filters.
- `sophos-central-cli xdr list-categories`  -  Fetch all categories, built-in as well as custom.
- `sophos-central-cli xdr list-queries`  -  List queries by a page size related to the tenant filtered by the parameters.
- `sophos-central-cli xdr start-query-run`  -  Run a query against the Sophos Data Lake, passing the SQL code as the value of a field in the request JSON.


**contain**


- `sophos-central-cli contain`  -  Isolate (contain) machines by hostname, IP or endpoint ID without knowing the tenant: resolves the tenant from the fleet refresh index (or searches one --tenant live), previews isolation plus optional scan, forensic log and alert acknowledgement, and runs only with --yes.


**fleet**


- `sophos-central-cli fleet refresh`  -  Refresh tenants, endpoints, alerts, account health and licences for every non-trial customer tenant in one run, with a per-tenant ok/forbidden/error ledger (add --include-trial for trial tenants).

- `sophos-central-cli fleet triage`  -  One ranked work queue across all customers: open high and medium alerts, bad or suspicious endpoints, tamper protection off, isolated or stale devices.

- `sophos-central-cli fleet health`  -  Rank customers worst-first by Account Health Check score with sub-scores and the change since the previous fleet refresh.

- `sophos-central-cli fleet gaps`  -  Per-customer security and commercial gaps: low protection, partial tamper protection, risky exclusions, unencrypted or unlicensed devices, licence over-usage or expiry.

- `sophos-central-cli fleet revenue`  -  Rank customers by monthly Sophos revenue for a billing month (partner credential only) and compare billed quantities with deployed endpoints counted by the last fleet refresh.

- `sophos-central-cli fleet tamper`  -  Find supported endpoints with tamper protection off across non-trial customers and, with --yes, turn it back on; tenants whose tenant-wide tamper setting is off are reported, not changed.

- `sophos-central-cli fleet siem-collect`  -  Pull SIEM events and alerts for every tenant from each tenant's saved cursor as NDJSON, warning when a cursor fell outside the 24-hour window (CLI and cron only; not exposed as an MCP tool).




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
sophos-central-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning triage across all customers

```bash
sophos-central-cli fleet triage --severity high,medium --agent --select items.tenant_name,items.kind,items.subject,items.next_command,blind_tenants,synced_at
```

Ranked queue from the store filled by fleet refresh, narrowed to the fields an agent needs, keeping blind_tenants so unreadable customers stay visible and synced_at (null until the first fleet refresh) so an empty store is not mistaken for an all-clear.

### Unhealthy endpoints for one customer

```bash
sophos-central-cli endpoints list --tenant 6f3c4a2e-1b9d-4e8f-a7c2-5d0e9b1f3a64 --health-status bad,suspicious --agent --select items.hostname,items.health.overall,items.tamperProtectionEnabled
```

Tenant-scoped list routed to the customer's regional host; --select keeps the large endpoint payload small.

### Contain a compromised workstation

```bash
sophos-central-cli contain WS-FIN-07 --scan --forensic-log --ack-alerts
```

Previews isolation, scan, forensic log and alert acknowledgement for the tenant that owns the host (needs a prior fleet refresh, or --tenant); add --yes to run it.

### Worst health scores before QBRs

```bash
sophos-central-cli fleet health --limit 10
```

Ranks customers by Account Health Check score with the change since the previous fleet refresh.

### Re-enable tamper protection on supported endpoints

```bash
sophos-central-cli fleet tamper
```

Lists supported endpoints with tamper protection off in each non-trial customer; tenants whose tenant-wide setting is off are reported, not changed. Rerun with --yes to apply.

## Auth Setup

Create an API credential in Sophos Central (Partner Dashboard > Settings & Policies > API Credentials for an MSP, or Global Settings > API Credentials Management for an organization or single tenant) and export SOPHOS_CENTRAL_CLIENT_ID and SOPHOS_CENTRAL_CLIENT_SECRET. The CLI mints an OAuth token from id.sophos.com, calls whoami to learn whether the credential is a partner, organization or tenant, and adds the matching ID header and regional host to every request. Partner and organization credentials choose a customer for tenant-scoped commands with --tenant <tenant-id> (or SOPHOS_CENTRAL_TENANT_ID); the fleet commands walk every tenant unless --tenant or SOPHOS_CENTRAL_TENANT_ID is set, in which case they are limited to those IDs, so unset it before fleet runs. After the first token is minted the client ID and the token are cached in the CLI's credentials file (a secret taken from the environment is never written there). A client ID in the environment always wins over a different saved one: the saved token is not sent, and the new token is kept in memory. After rotating the secret of the same credential, run 'sophos-central-cli auth logout' so a fresh token is minted. For the MCP server set both variables in its environment, plus SOPHOS_CENTRAL_TENANT_ID when a partner or organization credential should reach one tenant's endpoints and alerts.

Run `sophos-central-cli doctor` to verify setup.

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
  sophos-central-cli alerts list --agent --select allowedActions,category,description
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

- Use `--home <dir>` for one invocation, or set `SOPHOS_CENTRAL_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `SOPHOS_CENTRAL_CONFIG_DIR`, `SOPHOS_CENTRAL_DATA_DIR`, `SOPHOS_CENTRAL_STATE_DIR`, `SOPHOS_CENTRAL_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `SOPHOS_CENTRAL_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, and auth sidecars. `state` contains `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `sophos-central-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "sophos-central": {
        "command": "sophos-central-mcp",
        "env": {
          "SOPHOS_CENTRAL_HOME": "/srv/sophos-central"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `SOPHOS_CENTRAL_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `SOPHOS_CENTRAL_HOME`, or `doctor` will not find credentials left under the former root.

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
sophos-central-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "sophos-central-cli learnings confirm 12"] }
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
       materially more, record the divergence via `sophos-central-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `sophos-central-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `sophos-central-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
sophos-central-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
sophos-central-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
sophos-central-cli teach-playbook \
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
sophos-central-cli playbook amend \
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

`sophos-central-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `SOPHOS_CENTRAL_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
sophos-central-cli feedback "the --since flag is inclusive but docs say exclusive"
sophos-central-cli feedback --stdin < notes.txt
sophos-central-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `SOPHOS_CENTRAL_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SOPHOS_CENTRAL_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
sophos-central-cli profile save briefing --json
sophos-central-cli --profile briefing alerts list
sophos-central-cli profile list --json
sophos-central-cli profile show briefing
sophos-central-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `sophos-central-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add sophos-central-mcp -- sophos-central-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which sophos-central-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   sophos-central-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `sophos-central-cli <command> --help`.
