# Sophos Central CLI

**The Sophos Central partner, organization and tenant APIs, routed to the right tenant and region automatically, plus a cross-tenant triage queue, fleet tamper remediation, a multi-tenant SIEM collector and hostname-based containment.**

Covers the Sophos Central (now Sophos Fusion) partner, organization and tenant REST APIs: endpoints, alerts, policies, exclusions, Live Discover, XDR queries, SIEM, account health check, licensing and audit events. whoami, the X-Partner-ID / X-Organization-ID / X-Tenant-ID headers and each tenant's regional host are handled for you. fleet refresh fills a local store for every customer so fleet triage, fleet health and fleet gaps answer cross-tenant questions offline; fleet tamper, fleet siem-collect and fleet revenue work live across tenants, and contain isolates a device by hostname without looking up the tenant first.

Learn more at [Sophos Central](https://developer.sophos.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `sophos-central-cli` and `sophos-central-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=sophos-central). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/sophos-central --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/sophos-central --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `sophos-central-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the sophos-central skill from https://github.com/Servosity/msp-skills/tree/main/skills/sophos-central. The skill defines how its required CLI (`sophos-central-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=sophos-central).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SOPHOS_CENTRAL_CLIENT_ID` and `SOPHOS_CENTRAL_CLIENT_SECRET` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "sophos-central": {
      "command": "sophos-central-mcp",
      "env": {
        "SOPHOS_CENTRAL_CLIENT_ID": "<your-client-id>",
        "SOPHOS_CENTRAL_CLIENT_SECRET": "<your-client-secret>"
      }
    }
  }
}
```

</details>

## Authentication

Create an API credential in Sophos Central (Partner Dashboard > Settings & Policies > API Credentials for an MSP, or Global Settings > API Credentials Management for an organization or single tenant) and export SOPHOS_CENTRAL_CLIENT_ID and SOPHOS_CENTRAL_CLIENT_SECRET. The CLI mints an OAuth token from id.sophos.com, calls whoami to learn whether the credential is a partner, organization or tenant, and adds the matching ID header and regional host to every request. Partner and organization credentials choose a customer for tenant-scoped commands with --tenant <tenant-id> (or SOPHOS_CENTRAL_TENANT_ID); the fleet commands walk every tenant unless --tenant or SOPHOS_CENTRAL_TENANT_ID is set, in which case they are limited to those IDs, so unset it before fleet runs. After the first token is minted the client ID and the token are cached in the CLI's credentials file (a secret taken from the environment is never written there). A client ID in the environment always wins over a different saved one: the saved token is not sent, and the new token is kept in memory. After rotating the secret of the same credential, run 'sophos-central-cli auth logout' so a fresh token is minted. For the MCP server set both variables in its environment, plus SOPHOS_CENTRAL_TENANT_ID when a partner or organization credential should reach one tenant's endpoints and alerts.

## Quick Start

```bash
# Check that credentials are configured and Sophos Central is reachable
sophos-central-cli doctor

# Mint a token and confirm the credential type (partner, organization or tenant) that drives routing
sophos-central-cli whoami

# Partner credentials: list customer tenants and their IDs
sophos-central-cli tenants list --page-size 100

# Fill the local store for every tenant in one rate-limited pass
sophos-central-cli fleet refresh --resources tenants,endpoints,alerts,health,licenses

# Today's cross-tenant work queue from the synced store
sophos-central-cli fleet triage --severity high,medium --stale 14d

```

## Unique Features

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

## Usage

Run `sophos-central-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, and other auth sidecars |
| `state` | Runtime state such as `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `SOPHOS_CENTRAL_CONFIG_DIR`, `SOPHOS_CENTRAL_DATA_DIR`, `SOPHOS_CENTRAL_STATE_DIR`, or `SOPHOS_CENTRAL_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `SOPHOS_CENTRAL_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export SOPHOS_CENTRAL_HOME=/srv/sophos-central
sophos-central-cli doctor
```

Under `SOPHOS_CENTRAL_HOME=/srv/sophos-central`, the four dirs resolve to `/srv/sophos-central/config`, `/srv/sophos-central/data`, `/srv/sophos-central/state`, and `/srv/sophos-central/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `SOPHOS_CENTRAL_DATA_DIR` overrides an explicit `--home` for that kind. Use `SOPHOS_CENTRAL_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `SOPHOS_CENTRAL_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `sophos-central-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### admins

Manage admins

- **`sophos-central-cli admins create-role-assignment`** - Assign a role of principal type "user" to a tenant admin. Any existing assignment is overridden.
- **`sophos-central-cli admins create-tenant`** - Create a tenant admin from a directory user.
- **`sophos-central-cli admins deprovision-by-id`** - Remove an admin by ID.
- **`sophos-central-cli admins get-role-assignment-by-id`** - Get tenant admin role assignment by ID.
- **`sophos-central-cli admins get-tenant-by-id`** - Get tenant admin details by ID.
- **`sophos-central-cli admins list-role-assignments`** - Get the list of role assignments for given admin.
- **`sophos-central-cli admins list-tenant`** - List all tenant admins.
- **`sophos-central-cli admins remove-role-assignment-by-id`** - Remove role assignment from an admin.

### alerts

List open alerts, search alerts, and perform actions such as resolving alerts.

- **`sophos-central-cli alerts act-on`** - Take an action on a specific alert.
- **`sophos-central-cli alerts get`** - Get details of a specific alert.
- **`sophos-central-cli alerts list`** - Get alerts matching criteria in query parameters.
- **`sophos-central-cli alerts search`** - Get alerts matching request.

### allowed_items

Manage allowed items. These are always treated as non-malicious.

- **`sophos-central-cli allowed-items delete-by-id`** - Deletes the specified allowed item.
- **`sophos-central-cli allowed-items get`** - Get all allowed items.
- **`sophos-central-cli allowed-items get-by-id`** - Get an allowed item by ID.
- **`sophos-central-cli allowed-items patch-by-id`** - Update an allowed item.
- **`sophos-central-cli allowed-items post-by-id`** - Exempt an item from conviction.

### audit_events

List and read audit events.

- **`sophos-central-cli audit-events`** - List audit events.

### billing

Manage billing

- **`sophos-central-cli billing`** - Get usage report

### blocked_addresses

Manage blocked IP addresses for a customer.

- **`sophos-central-cli blocked-addresses delete-blocked-address-sets`** - Remove multiple IP addresses or IP address ranges from being blocked.
- **`sophos-central-cli blocked-addresses get-blocked-address-sets`** - Get the list of all currently blocked IP addresses and IP address ranges.
- **`sophos-central-cli blocked-addresses post-blocked-address-sets`** - Allows the user to specify multiple IP addresses or IP address ranges that devices should refuse to communicate with. This feature is only available on Windows devices.

### blocked_items

Manage blocked items. These are always treated as malicious or potentially malicious.

- **`sophos-central-cli blocked-items delete-by-id`** - Deletes the specified blocked item.
- **`sophos-central-cli blocked-items get`** - Get all blocked items.
- **`sophos-central-cli blocked-items get-by-id`** - Get a blocked item by ID.
- **`sophos-central-cli blocked-items post-by-id`** - Block an item from exoneration.

### device_software

Manage device software.

- **`sophos-central-cli device-software`** - Update assigned endpoint device software in bulk.

### endpoint_groups

Manage endpoint groups

- **`sophos-central-cli endpoint-groups add-multiple-endpoints-to-group`** - Add endpoints to your group.
- **`sophos-central-cli endpoint-groups create-tenant`** - Add new endpoint group to the directory.
- **`sophos-central-cli endpoint-groups delete-tenant-by-id`** - Delete endpoint group.
- **`sophos-central-cli endpoint-groups get-tenant-by-id`** - Get endpoint group by ID.
- **`sophos-central-cli endpoint-groups list-tenant`** - Endpoint groups in the directory.
- **`sophos-central-cli endpoint-groups list-tenant-by-group-type`** - Endpoint groups of your specified type in the directory.
- **`sophos-central-cli endpoint-groups list-tenant-endpoints-by-id`** - Returns the endpoints in your specified group.
- **`sophos-central-cli endpoint-groups patch-tenant`** - Update endpoint group.
- **`sophos-central-cli endpoint-groups remove-multiple-endpoints-from-group`** - Remove endpoints from a group.
- **`sophos-central-cli endpoint-groups remove-single-endpoint-from-group`** - Remove endpoint from a group.

### endpoint_tags

Assign tags to endpoints.

- **`sophos-central-cli endpoint-tags`** - Assign and unassign tags to endpoints.

### endpoints

Search for endpoints, view individual endpoint details, delete an endpoint or restore endpoints.

- **`sophos-central-cli endpoints bulk-delete`** - Delete a list of endpoints.
- **`sophos-central-cli endpoints bulk-isolate`** - Turn on or off endpoint isolation for multiple endpoints. We recommend that you wait for a period of time between turning endpoint isolation on and off using this API.
- **`sophos-central-cli endpoints collect-forensic-log`** - Sends a request to the endpoint to collect forensic log.
- **`sophos-central-cli endpoints collect-memory-dump`** - Instructs the target device to generate a memory dump of the specified process or the kernel. The dump is then uploaded to a customer-specified Amazon S3 bucket.
- **`sophos-central-cli endpoints delete`** - Deletes a specified endpoint.
- **`sophos-central-cli endpoints get`** - Get an endpoint based on ID.
- **`sophos-central-cli endpoints get-aap`** - Get Adaptive Attack Protection settings for an endpoint.
- **`sophos-central-cli endpoints get-forensic-log-status`** - Gets the status of the forensic log collection.
- **`sophos-central-cli endpoints get-isolation`** - Get isolation settings for an endpoint.
- **`sophos-central-cli endpoints get-memory-dump-status`** - Gets the status of the memory dump collection.
- **`sophos-central-cli endpoints get-tamper-protection`** - Get Tamper Protection settings for a specified endpoint.
- **`sophos-central-cli endpoints list`** - Get all the endpoints for the specified tenant.
- **`sophos-central-cli endpoints scan`** - Sends a request to the specified endpoint to perform or configure a scan.
- **`sophos-central-cli endpoints set-isolation`** - Update isolation settings for an endpoint.
- **`sophos-central-cli endpoints set-tamper-protection`** - Turns Tamper Protection on or off on an endpoint. Or generates a new Tamper Protection password. Note that Tamper Protection can be turned on for an endpoint only if it has also been turned on globally.
- **`sophos-central-cli endpoints update-aap`** - Update Adaptive Attack Protection settings for an endpoint.
- **`sophos-central-cli endpoints update-check`** - Sends a request to the endpoint to check for Sophos management agent software updates.

### enrichment_pivots

Manage enrichment pivots

- **`sophos-central-cli enrichment-pivots create`** - Create a new enrichment.
- **`sophos-central-cli enrichment-pivots delete`** - Delete enrichment.
- **`sophos-central-cli enrichment-pivots get`** - Get enrichments.
- **`sophos-central-cli enrichment-pivots update`** - Update an existing enrichment.

### event_journal

Event journal settings for a customer's computers and servers.

- **`sophos-central-cli event-journal get-settings`** - Get all event journal settings.
- **`sophos-central-cli event-journal update-settings`** - Update settings for event journal size and disk space limits. If you specify both a maximum disk space and a maximum journal size, the lower of these limits is used.

### exclusions

Manage exclusions

- **`sophos-central-cli exclusions add-intrusion-prevention`** - Add a new Intrusion Prevention exclusion.
- **`sophos-central-cli exclusions add-isolation`** - Adds a new Isolation exclusion.
- **`sophos-central-cli exclusions add-scanning`** - Add a new scanning exclusion.
- **`sophos-central-cli exclusions delete-intrusion-prevention-by-id`** - Delete an Intrusion Prevention exclusion by ID.
- **`sophos-central-cli exclusions delete-isolation-by-id`** - Deletes an Isolation exclusion.
- **`sophos-central-cli exclusions delete-scanning-by-id`** - Deletes a scanning exclusion.
- **`sophos-central-cli exclusions get-intrusion-prevention-by-id`** - Get an Intrusion Prevention exclusion by ID.
- **`sophos-central-cli exclusions get-isolation-by-id`** - Get a single Isolation exclusion by ID.
- **`sophos-central-cli exclusions get-scanning-by-id`** - Get a scanning exclusion by ID.
- **`sophos-central-cli exclusions list-intrusion-prevention`** - Get all Intrusion Prevention exclusions.
- **`sophos-central-cli exclusions list-isolation`** - Get all isolation exclusions.
- **`sophos-central-cli exclusions list-scanning`** - Get all scanning exclusions.
- **`sophos-central-cli exclusions update-intrusion-prevention-by-id`** - Update an Intrusion Prevention exclusion by ID.
- **`sophos-central-cli exclusions update-isolation-by-id`** - Updates an Isolation exclusion by ID.
- **`sophos-central-cli exclusions update-scanning-by-id`** - Update a scanning exclusion by ID.

### exploit_mitigation

Manage Exploit Mitigation settings.

- **`sophos-central-cli exploit-mitigation add-application`** - Exclude a set of file paths from Exploit Mitigation.
- **`sophos-central-cli exploit-mitigation delete-application-by-id`** - Deletes a custom (user-defined) Exploit Mitigation application by ID. Note you can only delete custom applications. A request to delete a system-detected application fails with a 409 Conflict message.
- **`sophos-central-cli exploit-mitigation get-application-by-id`** - Get Exploit Mitigation settings for an application.
- **`sophos-central-cli exploit-mitigation get-applications`** - Get Exploit Mitigation settings for all protected applications.
- **`sophos-central-cli exploit-mitigation get-categories`** - Get Exploit Mitigation categories.
- **`sophos-central-cli exploit-mitigation get-detected-exploit-by-id`** - Get a detected exploit by ID.
- **`sophos-central-cli exploit-mitigation get-detected-exploits`** - Get detected exploits and the number of each detected exploit.
- **`sophos-central-cli exploit-mitigation patch-application-by-id`** - Update Exploit Mitigation settings for an application.

### health_check

Manage health check

- **`sophos-central-cli health-check get`** - Account Health
- **`sophos-central-cli health-check get-historical-scores`** - Get historical health scores.
- **`sophos-central-cli health-check get-regional-scores`** - Get health score for comparison to other organizations in your region.
- **`sophos-central-cli health-check post-snooze`** - Snooze reminders for account health issues for a customer.

### installers

Manage installers

- **`sophos-central-cli installers`** - Get all the endpoint installer links for a tenant.

### licenses

Manage licenses

- **`sophos-central-cli licenses list`** - Get tenant licenses.
- **`sophos-central-cli licenses list-firewall`** - Get firewalls claimed by tenant or managed by the partner.

### live_discover

Run EDR queries on remote endpoints.

- **`sophos-central-cli live-discover get-category`** - Get the details of a category by ID.
- **`sophos-central-cli live-discover get-endpoint-execution-status`** - Get statuses of endpoints in the query run.
- **`sophos-central-cli live-discover get-execution-results-by-id`** - Get the results of a query run.
- **`sophos-central-cli live-discover get-execution-summary`** - Get a query run by ID.
- **`sophos-central-cli live-discover get-query`** - Get a query by ID.
- **`sophos-central-cli live-discover get-query-run-page`** - Get the list of query runs matching the given filters.
- **`sophos-central-cli live-discover list-categories`** - Get all categories, canned as well as custom.
- **`sophos-central-cli live-discover list-queries`** - Get queries matching the given filters.
- **`sophos-central-cli live-discover run-query`** - Run a saved EDR query or an ad hoc query on remote endpoints.

### migrations

Migrate endpoints from one tenant to another.

- **`sophos-central-cli migrations create-receiving-job`** - Start a migration job in the receiving tenant.
- **`sophos-central-cli migrations create-sending-job`** - Start a migration job in the sending tenant.
- **`sophos-central-cli migrations get-endpoints`** - Gets the status of endpoints that are being migrated.
- **`sophos-central-cli migrations get-job`** - Get a single migration job.
- **`sophos-central-cli migrations get-jobs`** - Gets all migration jobs for the tenant.
- **`sophos-central-cli migrations get-settings`** - Get the endpoint migration settings for this tenant.

### org_admins

Manage org admins

- **`sophos-central-cli org-admins create-admin-role-assignment`** - Assign a role to a organization administrator.
- **`sophos-central-cli org-admins create-organization-admin`** - Create a new organization administrator.
- **`sophos-central-cli org-admins delete-organization-admin-by-id`** - Remove an admin by ID.
- **`sophos-central-cli org-admins get-admin-role-assignment-by-id`** - Get organization administrator role assignment by ID.
- **`sophos-central-cli org-admins get-admin-role-assignments`** - Get the list of role assignments for given admin.
- **`sophos-central-cli org-admins get-organization-admin-by-id`** - Get organization administrator details by ID.
- **`sophos-central-cli org-admins get-organization-admins`** - List all organization admins.
- **`sophos-central-cli org-admins remove-admin-role-assignment-by-id`** - Remove role assignment from an admin.

### org_roles

Manage org roles

- **`sophos-central-cli org-roles create-organization-role`** - Create a new organization role.
- **`sophos-central-cli org-roles delete-organization-role-by-id`** - Delete a organization role by ID.
- **`sophos-central-cli org-roles get-organization-role-by-id`** - Get organization role by ID.
- **`sophos-central-cli org-roles get-role-permission-set`** - Get permission set details.
- **`sophos-central-cli org-roles list-organization-roles`** - List all organization roles.
- **`sophos-central-cli org-roles patch-organization-role`** - Patch an existing organization role.

### org_tenants

Manage org tenants

- **`sophos-central-cli org-tenants get`** - Get a tenant by ID.
- **`sophos-central-cli org-tenants list`** - Get all the tenants in the specified organization.

### partner_admins

Create new administrators, update their profile information, enumerate them or view administrator details.

- **`sophos-central-cli partner-admins create`** - Create a new partner administrator.
- **`sophos-central-cli partner-admins create-admin-role-assignment`** - Assign a role to a partner administrator.
- **`sophos-central-cli partner-admins delete-by-id`** - Delete a PDB Admin for Central while internally just revoking PDB access.
- **`sophos-central-cli partner-admins get`** - List all partner admins.
- **`sophos-central-cli partner-admins get-admin-role-assignment-by-id`** - Get partner administrator role assignment by ID.
- **`sophos-central-cli partner-admins get-admin-role-assignments`** - Get the list of role assignments for given admin.
- **`sophos-central-cli partner-admins get-partner`** - Get partner administrator details by ID.
- **`sophos-central-cli partner-admins remove-admin-role-assignment-by-id`** - Remove role assignment from an admin.

### partner_roles

Manage partner roles

- **`sophos-central-cli partner-roles create`** - Create a new partner role.
- **`sophos-central-cli partner-roles delete-by-id`** - Delete a partner role by ID.
- **`sophos-central-cli partner-roles get-by-id`** - Get partner role by ID.
- **`sophos-central-cli partner-roles get-role-permission-set`** - Get permission set details.
- **`sophos-central-cli partner-roles list`** - List all partner roles.
- **`sophos-central-cli partner-roles patch`** - Patch an existing partner role.

### peripherals

Manage peripherals

- **`sophos-central-cli peripherals get-by-id`** - Get a peripheral by ID.
- **`sophos-central-cli peripherals get-by-tenant-id`** - Get all peripherals.

### policies

Manage policies

- **`sophos-central-cli policies clone-base-policy`** - Clone a new policy from the base policy for a policy type.
- **`sophos-central-cli policies clone-policy`** - Clone policy.
- **`sophos-central-cli policies create-policy`** - Create new policy.
- **`sophos-central-cli policies delete-policy`** - Delete policy.
- **`sophos-central-cli policies get`** - Get policies of a tenant.
- **`sophos-central-cli policies get-base-policy`** - Get base policy for a policy type.
- **`sophos-central-cli policies get-policy`** - Policy details.
- **`sophos-central-cli policies get-setting-metadatas`** - Supported metadata.
- **`sophos-central-cli policies get-settings-of-base-policy`** - Get settings of the base policy for a policy type.
- **`sophos-central-cli policies get-settings-of-policy`** - Policy settings.
- **`sophos-central-cli policies get-value-of-setting-item-in-base-policy`** - Get the value of a setting in the base policy for a policy type.
- **`sophos-central-cli policies get-value-of-setting-key-in-policy`** - Get the value of a setting key in a policy.
- **`sophos-central-cli policies reset-base-policy-settings`** - Reset the settings in a base policy.
- **`sophos-central-cli policies reset-policy-settings`** - Reset policy settings.
- **`sophos-central-cli policies reset-setting-in-base-policy`** - Reset a setting in the base policy to its default value.
- **`sophos-central-cli policies reset-setting-in-policy`** - Reset a setting to its default value.
- **`sophos-central-cli policies update-base-policy`** - Update base policy. Note that only settings can be changed.
- **`sophos-central-cli policies update-policy`** - Update policy. Note you can only change the settings for a base policy and roles with only assignment permissions can only change the ChangeUsersOrDevicesPolicyAppliesTo field.
- **`sophos-central-cli policies update-settings-of-base-policy`** - Update settings in the base policy for a policy type.
- **`sophos-central-cli policies update-settings-of-policy`** - Update policy settings.
- **`sophos-central-cli policies update-value-of-setting-item-in-base-policy`** - Update a setting in the base policy.

### provisioning

Get available products and manage default products to use when creating new tenants.

- **`sophos-central-cli provisioning list-default-products`** - List of products to turn on by default when the provisioning request for a tenant doesn't specify them.
- **`sophos-central-cli provisioning list-products`** - List of available products.
- **`sophos-central-cli provisioning patch-default-products`** - Update the list of default products.
- **`sophos-central-cli provisioning reset-default-products`** - Reset the list of default products. For new tenants provisioned after this, Sophos Central turns on the products specified in the provisioning request.

### roles

Manage roles

- **`sophos-central-cli roles create-tenant`** - Create a new tenant role.
- **`sophos-central-cli roles delete-tenant-by-id`** - Delete a tenant role by ID.
- **`sophos-central-cli roles get-permission-set`** - Get permission set details.
- **`sophos-central-cli roles get-tenant-by-id`** - Get tenant role by ID.
- **`sophos-central-cli roles list-tenant`** - List all tenant roles.
- **`sophos-central-cli roles patch-tenant`** - Patch an existing tenant role.

### siem

Manage siem

- **`sophos-central-cli siem alerts`** - Get alert with timestamps within last 24 hours.
- **`sophos-central-cli siem events`** - Get events with timestamps within last 24 hours.

### software

Manage software

- **`sophos-central-cli software add-special-static-package`** - Add a special package by token, supplied by Sophos support. This is a one-way operation.
- **`sophos-central-cli software delete-comment`** - Delete the static package comment.
- **`sophos-central-cli software get-comment`** - Get the static package comment.
- **`sophos-central-cli software get-comments`** - Get all comments.
- **`sophos-central-cli software get-recommended-packages`** - Get all Sophos Recommended packages for the tenant.
- **`sophos-central-cli software get-static-package`** - Get an individual static package.
- **`sophos-central-cli software get-static-packages`** - Get all static packages available for the tenant.
- **`sophos-central-cli software update-comment`** - Add/Update the static package comment.

### tamper_protection

Turn Tamper Protection on or off on an endpoint and view settings.

- **`sophos-central-cli tamper-protection`** - Check whether Tamper Protection is turned on globally.

### tenants

Create new tenants, enumerate them or view tenant details.

- **`sophos-central-cli tenants cancel-pending-amendment`** - Cancel a pending account amendment.
- **`sophos-central-cli tenants create`** - Create a new tenant.
- **`sophos-central-cli tenants get`** - Get a tenant by ID.
- **`sophos-central-cli tenants get-account-amendment-by-id`** - Get account amendment by ID.
- **`sophos-central-cli tenants list`** - List all the tenants for a partner.
- **`sophos-central-cli tenants list-account-amendments`** - List all account amendments.
- **`sophos-central-cli tenants post-account-amendment`** - Make tenant account changes.
- **`sophos-central-cli tenants post-product-selection`** - Update tenant product selection.
- **`sophos-central-cli tenants update`** - Patch a tenant by ID.

### user_groups

Manage user groups

- **`sophos-central-cli user-groups add-multiple-users-to-group`** - Add multiple users to the specified group.
- **`sophos-central-cli user-groups create-tenant`** - Add a new group to the directory.
- **`sophos-central-cli user-groups delete-tenant-by-id`** - Deletes the specified group.
- **`sophos-central-cli user-groups get-tenant-by-id`** - Get group by ID.
- **`sophos-central-cli user-groups list-tenant`** - List groups in the directory.
- **`sophos-central-cli user-groups list-tenant-users-by-id`** - List users in the specified group.
- **`sophos-central-cli user-groups patch-tenant`** - Update a group.
- **`sophos-central-cli user-groups remove-multiple-users-from-group`** - Remove multiple users from a group.
- **`sophos-central-cli user-groups remove-single-user-from-group`** - Update a group to remove a user.

### users

Manage users

- **`sophos-central-cli users add-to-multiple-groups`** - Add a user to multiple groups.
- **`sophos-central-cli users create-tenant`** - Add a new user to the directory.
- **`sophos-central-cli users delete-tenant-by-id`** - Delete a user by ID.
- **`sophos-central-cli users get-tenant-by-id`** - Get a user by ID.
- **`sophos-central-cli users list-tenant`** - List users in the directory.
- **`sophos-central-cli users list-tenant-groups-by-id`** - List groups that a user belongs to.
- **`sophos-central-cli users patch-tenant`** - Update an existing user.
- **`sophos-central-cli users remove-from-multiple-groups`** - Remove a user from multiple groups.
- **`sophos-central-cli users remove-from-single-group`** - Remove user from a group.

### web_control

Manage Web Control settings.

- **`sophos-central-cli web-control add-local-site-exclusion`** - Adds a new local site.
- **`sophos-central-cli web-control clear-web-decryption-excluded-sites`** - Clears the list of websites excluded from SSL/TLS decryption.
- **`sophos-central-cli web-control delete-local-sites-by-id`** - Deletes the specified local site.
- **`sophos-central-cli web-control get-local-site-by-id`** - Get a local site by ID.
- **`sophos-central-cli web-control get-web-decryption-settings`** - Get settings for SSL/TLS decryption of HTTPS websites.
- **`sophos-central-cli web-control list-categories`** - Get all Web Control categories.
- **`sophos-central-cli web-control list-local-sites-by-tenant-id`** - Get all local sites.
- **`sophos-central-cli web-control list-web-decryption-excluded-sites`** - List of websites excluded from SSL/TLS decryption.
- **`sophos-central-cli web-control update-local-site-setting-by-id`** - Update a local site definition.
- **`sophos-central-cli web-control update-web-decryption-excluded-sites`** - Add and remove websites excluded from SSL/TLS decryption.
- **`sophos-central-cli web-control update-web-decryption-settings`** - Update settings for SSL/TLS decryption of HTTPS websites.

### whoami

Whoami API.

- **`sophos-central-cli whoami`** - Returns information about the caller.

### xdr

Manage xdr

- **`sophos-central-cli xdr cancel-query-run`** - Cancel a query run by ID.
- **`sophos-central-cli xdr get-category`** - Get the details of a category.
- **`sophos-central-cli xdr get-query`** - Get the details of a query.
- **`sophos-central-cli xdr get-query-results`** - Get the paged results for a query run.
- **`sophos-central-cli xdr get-query-run`** - Get a query run by ID.
- **`sophos-central-cli xdr get-query-runs`** - Get the list of query runs matching the given filters.
- **`sophos-central-cli xdr list-categories`** - Fetch all categories, built-in as well as custom.
- **`sophos-central-cli xdr list-queries`** - List queries by a page size related to the tenant filtered by the parameters.
- **`sophos-central-cli xdr start-query-run`** - Run a query against the Sophos Data Lake, passing the SQL code as the value of a field in the request JSON. The schema reference is available [here](https://docs.sophos.com/central/References/schemas/index.html?schema=xdr_schema_docs).


### contain


- **`sophos-central-cli contain`** - Isolate (contain) machines by hostname, IP or endpoint ID without knowing the tenant: resolves the tenant from the fleet refresh index (or searches one --tenant live), previews isolation plus optional scan, forensic log and alert acknowledgement, and runs only with --yes.


### fleet


- **`sophos-central-cli fleet refresh`** - Refresh tenants, endpoints, alerts, account health and licences for every non-trial customer tenant in one run, with a per-tenant ok/forbidden/error ledger (add --include-trial for trial tenants).

- **`sophos-central-cli fleet triage`** - One ranked work queue across all customers: open high and medium alerts, bad or suspicious endpoints, tamper protection off, isolated or stale devices.

- **`sophos-central-cli fleet health`** - Rank customers worst-first by Account Health Check score with sub-scores and the change since the previous fleet refresh.

- **`sophos-central-cli fleet gaps`** - Per-customer security and commercial gaps: low protection, partial tamper protection, risky exclusions, unencrypted or unlicensed devices, licence over-usage or expiry.

- **`sophos-central-cli fleet revenue`** - Rank customers by monthly Sophos revenue for a billing month (partner credential only) and compare billed quantities with deployed endpoints counted by the last fleet refresh.

- **`sophos-central-cli fleet tamper`** - Find supported endpoints with tamper protection off across non-trial customers and, with --yes, turn it back on; tenants whose tenant-wide tamper setting is off are reported, not changed.

- **`sophos-central-cli fleet siem-collect`** - Pull SIEM events and alerts for every tenant from each tenant's saved cursor as NDJSON, warning when a cursor fell outside the 24-hour window (CLI and cron only; not exposed as an MCP tool).




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`sophos-central-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`sophos-central-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`sophos-central-cli learnings list`** - Inspect taught rows
- **`sophos-central-cli learnings forget <query>`** - Undo a teach
- **`sophos-central-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`sophos-central-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`sophos-central-cli teach-pattern`** - Install a query/resource template up front
- **`sophos-central-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `SOPHOS_CENTRAL_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `sophos-central-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
sophos-central-cli alerts list

# JSON for scripting and agents
sophos-central-cli alerts list --json
# Filter to specific fields
sophos-central-cli alerts list --json --select allowedActions,category,description

# Dry run  -  show the request without sending
sophos-central-cli alerts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
sophos-central-cli alerts list --agent
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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
sophos-central-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `sophos-central-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/sophos-central-cli/config.toml`; `--home`, `SOPHOS_CENTRAL_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SOPHOS_CENTRAL_CLIENT_ID` | auth_flow_input | Yes | Client ID of a Sophos Central API credential |
| `SOPHOS_CENTRAL_CLIENT_SECRET` | auth_flow_input | Yes | Client secret of the same credential |
| `SOPHOS_CENTRAL_TENANT_ID` | selector | No | Tenant for tenant-scoped commands with a partner or organization credential (same as --tenant); also narrows fleet commands |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `sophos-central-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `sophos-central-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SOPHOS_CENTRAL_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **HTTP 400 tenantRequired on endpoints, alerts or policies commands**  -  Partner and organization credentials must pick a customer: add --tenant <tenant-id> or export SOPHOS_CENTRAL_TENANT_ID (IDs from 'tenants list', or 'org-tenants list' for an organization credential).
- **A customer is missing from fleet output or listed as forbidden**  -  Trial tenants are skipped unless --include-trial is passed; a 403 means the customer has not enabled Partner Assistance in its console. Both are recorded, never fatal.
- **A fleet command printed results but exited non-zero**  -  Some tenant failed for a reason other than an expected 403 (server error, timeout, incomplete listing); check tenants_with_unexpected_errors and tenants[].resources, then rerun for that tenant with --tenant <tenant-id>.
- **fleet triage, health or gaps return nothing**  -  They read the local store: run 'sophos-central-cli fleet refresh' first, and check SOPHOS_CENTRAL_TENANT_ID is unset so it does not narrow the fleet.
- **HTTP 429 or slow fleet commands**  -  Sophos enforces 100 calls/minute per credential, account and IP; the CLI paces at 1.5 req/s by default. Lower it with --rate-limit 1 when other tools share the credential.
- **Token request fails with HTTP 401 INVALID_CLIENT, or a rotated credential is ignored**  -  Check SOPHOS_CENTRAL_CLIENT_ID and SOPHOS_CENTRAL_CLIENT_SECRET, then run 'sophos-central-cli auth logout' so the previously saved credential is not reused.
- **--dry-run shows api.central.sophos.com for a tenant command**  -  Expected: the regional host and X-Tenant-ID are chosen when the request is sent. For contain and fleet tamper, run without --yes to see the real plan.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Sophos-Central-SIEM-Integration**](https://github.com/sophos/Sophos-Central-SIEM-Integration)  -  Python (142 stars)
- [**sophos-central-api-connector**](https://github.com/sophos/sophos-central-api-connector)  -  Python (32 stars)
- [**PS.Machine_Health**](https://github.com/sophos/PS.Machine_Health)  -  PowerShell (14 stars)
- [**sophos-mcp**](https://github.com/Aaronjacobs000/sophos-mcp)  -  TypeScript (12 stars)
- [**SophosCentral (PowerShell)**](https://github.com/simon-r-watson/SophosCentral)  -  PowerShell (12 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
