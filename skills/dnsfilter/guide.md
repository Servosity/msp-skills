# DNSFilter CLI

**Every DNSFilter API operation, plus one-command answers to 'why is this site blocked for this client' and 'which clients are behind on the roaming agent'.**

Manage organizations, sites, policies, allow and block lists, roaming clients and reports from the terminal or an agent. why-blocked and allow close the daily blocked-site ticket loop; agents drift and agents stale give an MSP a cross-client view of the roaming-agent fleet from a local SQLite mirror.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `dnsfilter-cli` and `dnsfilter-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.ps1 | iex
   ```
3. Verify: `dnsfilter-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=dnsfilter). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`. On Windows, unblock the downloaded file with `Unblock-File <binary>.exe` in PowerShell and put its folder on your `PATH` (the installer uses `%LOCALAPPDATA%\Programs\msp-skills`).

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/dnsfilter --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/dnsfilter --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `dnsfilter-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the dnsfilter skill from https://github.com/Servosity/msp-skills/tree/main/skills/dnsfilter. The skill defines how its required CLI (`dnsfilter-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download `dnsfilter-mcp.mcpb` from the [latest release](https://github.com/Servosity/msp-skills/releases?q=dnsfilter).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `DNSFILTER_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. The one bundle carries binaries for macOS (Apple Silicon and Intel), Linux x64 and Windows x64; on Windows on Arm, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "dnsfilter": {
      "command": "dnsfilter-mcp",
      "env": {
        "DNSFILTER_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

DNSFilter uses an API key sent as the raw value of the Authorization header (no Bearer prefix). Create one in the DNSFilter dashboard under Account Settings, API Tokens, in your MSP organization so the cross-client commands can see your client organizations. Set DNSFILTER_API_KEY to the bare token (`export DNSFILTER_API_KEY=<token>` on macOS / Linux, `$env:DNSFILTER_API_KEY = "<token>"` in PowerShell), or save it once by piping it to `dnsfilter-cli auth set-token`, which reads stdin only (`printf '%s\n' "$DNSFILTER_API_KEY" | dnsfilter-cli auth set-token`; PowerShell: `$env:DNSFILTER_API_KEY | dnsfilter-cli auth set-token`).

## Quick Start

```bash
# Confirm the CLI is installed, without calling the API.
dnsfilter-cli doctor --dry-run

# Mirror client orgs, sites, policies and the roaming-client fleet locally so fleet commands run offline.
dnsfilter-cli sync --resources organizations,networks,policies,user-agents

# Diagnose a blocked-site ticket: effective policy, deciding rule, recent query-log hits.
dnsfilter-cli why-blocked example.com --org "Acme Dental"

# Preview the allow-list change; add --confirm to apply it.
dnsfilter-cli allow example.com --org "Acme Dental" --note "TKT-4821"

# See which clients have roaming agents behind the latest release.
dnsfilter-cli agents drift --agent

```

## Unique Features

### Diagnose and fix a block
- **`why-blocked`**  -  Name the site or roaming-client policy and rule that blocks or allows a domain for a client org, plus query-log hits. Does not evaluate user, collection, LAN-subnet, Universal List or AppAware rules.

  _Reach for this first on any 'site is blocked for client X' ticket: it resolves the effective policy (including the current scheduled-policy slot) and checks the query log for the policy that really blocked it._

  ```bash
  dnsfilter-cli why-blocked example.com --org "Acme Dental" --agent
  ```
- **`allow`**  -  Add a domain to the allow list of the policies blocking it for a client (from the query log, else the site or roaming-client assignment). Previews by default; --confirm --note writes, then reads back.

  _Use after why-blocked confirms a legitimate site is blocked. --force is needed for a threat-categorized domain, --include-global to touch a global policy, and policies the key cannot edit are skipped._

  ```bash
  dnsfilter-cli allow example.com --org "Acme Dental" --note "TKT-4821"
  ```
- **`unblocks triage`**  -  Review pending end-user unblock requests grouped by domain across every client, with threat flags, age, and DNSFilter's own already-covered check. Needs unblock requests enabled on the account.

  _Use to work the unblock-request queue in one pass; act on a group with unblock-requests decision unblock-requests <id> --decision <action>._

  ```bash
  dnsfilter-cli unblocks triage --agent
  ```

### Fleet health across every client
- **`agents drift`**  -  See which client orgs have roaming clients behind the latest agent release, or on a specific bad build, with auto-update state per org. Joins the synced fleet with the vendor's live release feed.

  _Use after an agent release goes bad, or in the weekly fleet pass. Run dnsfilter-cli sync --resources organizations,networks,user-agents first; the release feed is read live so a rollback shows up immediately._

  ```bash
  dnsfilter-cli agents drift --bad-version 3.1.0 --agent
  ```
- **`agents stale`**  -  List roaming clients that stopped checking in or are unprotected or bypassed, grouped by client org with age buckets. Reads the local mirror; sync first.

  _Use for the weekly check of endpoints that are silently unfiltered. Run dnsfilter-cli sync --resources organizations,networks,user-agents first._

  ```bash
  dnsfilter-cli agents stale --older-than 7d --agent
  ```

### Policy hygiene across tenants
- **`lists where`**  -  Find every policy across every client whose allow or block list holds a domain or a parent domain, with counts of the sites and roaming clients it governs. Reads the local mirror; sync first.

  _Use to check precedent before allowing a domain, or to measure exposure when a domain makes the news. Run dnsfilter-cli sync --resources organizations,networks,policies,user-agents first; an empty mirror returns an empty result._

  ```bash
  dnsfilter-cli lists where example.com --agent
  ```
- **`policy-audit`**  -  Flag allow-listed domains now categorized as threats, unblocked security categories, and allowed unknown domains; with --baseline <policy-id>, also drift from that baseline policy.

  _Use in the recurring policy hygiene review; allow-list entries go bad as threat categorization changes._

  ```bash
  dnsfilter-cli policy-audit --org "Acme Dental" --baseline 1201 --agent
  ```

## Recipes

### Roaming client versions as compact JSON

```bash
dnsfilter-cli user-agents all --all --agent --select attributes.hostname,attributes.agent_version,attributes.last_sync
```

Narrow the large roaming-client payload to hostname, version and last check-in across every page before handing it to an agent.

### Precedent check before allowing a domain

```bash
dnsfilter-cli lists where example.com --agent
```

After a sync, shows every client policy that already allows or blocks the domain or a parent domain.

### Weekly silent-endpoint check

```bash
dnsfilter-cli agents stale --older-than 7d --agent
```

After a sync, roaming clients that have not checked in for a week or are unprotected, grouped by client.

### Work the unblock queue

```bash
dnsfilter-cli unblocks triage --agent
```

Pending unblock requests grouped by domain with threat flags and DNSFilter's already-covered check.

### Policy hygiene against a baseline

```bash
dnsfilter-cli policy-audit --org "Acme Dental" --baseline 1201 --agent
```

Threat-categorized allow-list entries, unblocked security categories, unknown-domain settings, and drift from policy 1201.

## Usage

Run `dnsfilter-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `DNSFILTER_CONFIG_DIR`, `DNSFILTER_DATA_DIR`, `DNSFILTER_STATE_DIR`, or `DNSFILTER_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `DNSFILTER_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export DNSFILTER_HOME=/srv/dnsfilter
dnsfilter-cli doctor
```

On Windows (PowerShell):

```powershell
$env:DNSFILTER_HOME = "C:\ProgramData\dnsfilter"
dnsfilter-cli doctor
```

Under `DNSFILTER_HOME=/srv/dnsfilter`, the four dirs resolve to `/srv/dnsfilter/config`, `/srv/dnsfilter/data`, `/srv/dnsfilter/state`, and `/srv/dnsfilter/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "dnsfilter": {
      "command": "dnsfilter-mcp",
      "env": {
        "DNSFILTER_HOME": "/srv/dnsfilter"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `DNSFILTER_DATA_DIR` overrides an explicit `--home` for that kind. Use `DNSFILTER_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `DNSFILTER_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `dnsfilter-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### agent-local-user-bulk-deletes

Manage agent local user bulk deletes

- **`dnsfilter-cli agent-local-user-bulk-deletes counts`** - Gets an agent local user bulk delete count by bulk delete id.
- **`dnsfilter-cli agent-local-user-bulk-deletes create`** - Create an agent local user bulk delete.
- **`dnsfilter-cli agent-local-user-bulk-deletes show`** - Gets an agent local user bulk delete by id.

### agent-local-users

Manage agent local users

- **`dnsfilter-cli agent-local-users all`** - Gets ALL agent local users associated to an user organization with basic information.
- **`dnsfilter-cli agent-local-users counts`** - Gets counts of agent local users by policy assignment status.
- **`dnsfilter-cli agent-local-users destroy`** - Deletes a agent local user.
- **`dnsfilter-cli agent-local-users index`** - Gets the agent local users associated to an organization with basic information.
- **`dnsfilter-cli agent-local-users show`** - Gets basic information of the specified agent local user.
- **`dnsfilter-cli agent-local-users update`** - Updates an agent local user with the specified data.

### agent-local-users-csv-export

Manage agent local users csv export

- **`dnsfilter-cli agent-local-users-csv-export create`** - Creates an agent local users CSV export record to track export of agent local users.
- **`dnsfilter-cli agent-local-users-csv-export show`** - Shows an agent local users CSV export.

### api-keys

Manage api keys

- **`dnsfilter-cli api-keys create`** - Creates a new api key.
- **`dnsfilter-cli api-keys destroy`** - Removes the api key.
- **`dnsfilter-cli api-keys index`** - Gets the list of api keys associated with the current user.
- **`dnsfilter-cli api-keys show`** - Displays an api key.

### application-categories

Manage application categories

- **`dnsfilter-cli application-categories index`** - List extant application categories.
- **`dnsfilter-cli application-categories show`** - Gets basic information of a specific application category.

### applications

Get information about available applications for the allow and block lists.

- **`dnsfilter-cli applications all`** - List applications basic information, including deleted ones.
- **`dnsfilter-cli applications index`** - List extant applications basic information.
- **`dnsfilter-cli applications show`** - Gets basic information of a specific application.

### billing

Manage billing

- **`dnsfilter-cli billing address-show`** - Show the billing address for the given organization.
- **`dnsfilter-cli billing address-update`** - Update the billing address for the given organization.
- **`dnsfilter-cli billing create`** - Register a newly created payment method in database.
- **`dnsfilter-cli billing show`** - List basic billing information of the user organization registered in Stripe platform.

### block-pages

Manage block pages

- **`dnsfilter-cli block-pages all`** - Gets ALL block pages associated with the current user.
- **`dnsfilter-cli block-pages create`** - Creates a new block page.
- **`dnsfilter-cli block-pages destroy`** - Deletes a block page.
- **`dnsfilter-cli block-pages index`** - Gets the list of block pages associated with the current user.
- **`dnsfilter-cli block-pages show`** - Gets the specified block page.
- **`dnsfilter-cli block-pages update`** - Updates a block_page with the specified data.

### categories

Manage categories

- **`dnsfilter-cli categories all`** - List all categories including internal categories.
- **`dnsfilter-cli categories index`** - List categories basic information.
- **`dnsfilter-cli categories show`** - Gets basic information of a specific category.

### collections

Manage collections


### current-user

Manage current user

- **`dnsfilter-cli current-user user-current-show`** - Fetch information about the currently authenticated user.
- **`dnsfilter-cli current-user user-current-update`** - Update attributes for the currently authenticated user.
- **`dnsfilter-cli current-user user-organization-ui-settings-suppress-license-warning`** - Returns the suppression date and relevant information.
- **`dnsfilter-cli current-user user-ui-settings-show`** - Returns the current user's UI settings.
- **`dnsfilter-cli current-user user-ui-settings-update`** - Updates current user's UI settings.

### cyber-sight

Manage cyber sight

- **`dnsfilter-cli cyber-sight csv-exports-create`** - Creates a cybersight csv export record to track export of cybersight report info.
Invalid report filters and invalid included_columns are ignored; the CSV is generated
using any valid filters and columns that remain.

Filters (query/body) by report type:
  activity_logs: start_at, end_at, org_uuids, msp_uuid, excluded_org_uuids, category_ids, user_agent_uuids,
    agent_local_user_uuids, weekdays_only, search_id, size, sort_direction, activity_type_ids,
    app_executable_path, app_name, app_window_title, web_full_url, web_host.
  top_websites: start_at, end_at, org_uuids, msp_uuid, excluded_org_uuids, category_ids, user_agent_uuids,
    agent_local_user_uuids, weekdays_only, search_id, size, sort_direction, web_full_url, web_host.
  top_applications: start_at, end_at, org_uuids, msp_uuid, excluded_org_uuids, category_ids,
    user_agent_uuids, agent_local_user_uuids, weekdays_only, search_id, size, sort_direction, app_name,
    app_executable_path, app_window_title.
  top_categories: start_at, end_at, org_uuids, msp_uuid, excluded_org_uuids, category_ids,
    user_agent_uuids, agent_local_user_uuids, weekdays_only, search_id, size, sort_direction, threats_only, web_categories.
  top_streaming: start_at, end_at, org_uuids, msp_uuid, excluded_org_uuids, category_ids,
    user_agent_uuids, agent_local_user_uuids, weekdays_only, search_id, size, sort_direction, app_name.
  top_risky_users: start_at, end_at, org_uuids, msp_uuid, excluded_org_uuids, category_ids,
    user_agent_uuids, agent_local_user_uuids, weekdays_only, search_id, size, sort_direction.
  top_active_clients: start_at, end_at, org_uuids, msp_uuid, excluded_org_uuids, category_ids,
    user_agent_uuids, agent_local_user_uuids, weekdays_only, search_id, size, sort_direction.
  top_ai_usage: start_at, end_at, ai_tool (required), org_uuids, msp_uuid, excluded_org_uuids,
    user_agent_uuids, agent_local_user_uuids, weekdays_only, search_id, size, sort_direction.
    When ai_tool=web: web_full_url, web_host.
    When ai_tool=app: app_name, app_executable_path, app_window_title.
- **`dnsfilter-cli cyber-sight csv-exports-show`** - Shows a CyberSight csv export.

### dashboards

Manage dashboards

- **`dnsfilter-cli dashboards available`** - Dashboards available to the current user.
- **`dnsfilter-cli dashboards create`** - Create a dashboard.
- **`dnsfilter-cli dashboards destroy`** - Delete a dashboard.
- **`dnsfilter-cli dashboards index`** - Dasbboards for use by the current user. The user's default dashboard will
be returned first, the rest alphabetically by name.
- **`dnsfilter-cli dashboards link`** - Linking a dashboard will add it to the list for the current user.
- **`dnsfilter-cli dashboards make-private`** - This will remove access to the dashboard for other users.
- **`dnsfilter-cli dashboards make-public`** - This will make it available to other users who are members in an
organization the owning user is a member of.
- **`dnsfilter-cli dashboards owned`** - Dashboards owned by the current user.
- **`dnsfilter-cli dashboards unlink`** - Unlinking a dashboard will remove it from the list of dashboards for the
current user.
- **`dnsfilter-cli dashboards update`** - Update a dashboard.

### dictionary

Manage dictionary

- **`dnsfilter-cli dictionary lookups-ai-applications`** - Return a list of AI Applications and websites.
- **`dnsfilter-cli dictionary lookups-cyber-sight-activity-types`** - Returns all available CyberSightReports activity types.
- **`dnsfilter-cli dictionary lookups-qp-methods`** - List QP Methods
- **`dnsfilter-cli dictionary lookups-vpn-settings-state-types`** - Returns all available VPN Settings State types.

### distributors

Manage distributors

- **`dnsfilter-cli distributors health`** - Allows testing API reachability and validation of the authentication header.

NOTE: This endpoint supports the read-only pre-shared secret key.
- **`dnsfilter-cli distributors index`** - Allows to fetch basic information about a Distributor and its parent organization.
- **`dnsfilter-cli distributors msps-cancel`** - Cancels the specified MSP parent organization and all of its sub-organizations.
- **`dnsfilter-cli distributors msps-create`** - Creates an MSP and its parent organization.
- **`dnsfilter-cli distributors msps-index`** - Allows to get the list of MSPs that matches the search criteria.

Returns all non-canceled ones by default if no search filters are
specified.

Note: endpoint does pagination, be sure to fetch all available pages to
get the full list.
- **`dnsfilter-cli distributors msps-reactivate`** - Reactivates the specified MSP parent organization and all of its sub-organization if specified.
- **`dnsfilter-cli distributors msps-show`** - Allows to fetch basic information about an MSP and its parent organization.
- **`dnsfilter-cli distributors msps-update`** - Updates the specified MSP and its parent organization data.
- **`dnsfilter-cli distributors msps-upsert-skus`** - Adds or removes the SKUs available to an entitlements-managed MSP post-creation.
Returns the updated MSP. Triggers the license cascade for removed licenses.
- **`dnsfilter-cli distributors organizations-add-sku`** - Helper endpoint to add a single SKU to the specified MSP parent
organization or sub-organization instead of doing a full update that
should contain all SKUs.
- **`dnsfilter-cli distributors organizations-cancel`** - Cancels the specified MSP sub-organization.

Note: See _Organization Cancellation_ for important details.
- **`dnsfilter-cli distributors organizations-create`** - Creates an MSP sub-organization.
- **`dnsfilter-cli distributors organizations-index`** - Allows to get the list of organizations that matches the search criteria,
returns all non-canceled ones by default if no search filters are
specified. Note: endpoint does pagination, be sure to fetch all available
pages to get the full list.
- **`dnsfilter-cli distributors organizations-reactivate`** - Reactivates the specified MSP sub-organization.

Note: See _Organization Cancellation_ for important details.
- **`dnsfilter-cli distributors organizations-remove-sku`** - Helper endpoint to remove a single SKU to the specified MSP parent
organization or sub-organization instead of doing a full update that
should contain all SKUs.
- **`dnsfilter-cli distributors organizations-show`** - Allows to fetch basic information about an MSP parent organization or sub-organization.
- **`dnsfilter-cli distributors organizations-update`** - Updates the specified MSP sub-organization data.
- **`dnsfilter-cli distributors reports-covered-users-summary-by-organization`** - Allows to fetch covered users information by organization for the given
date range.

Maximum days in date range is limited to 31.

NOTE: This endpoint supports the read-only pre-shared secret key.
- **`dnsfilter-cli distributors reports-usage-by-organization`** - Allows to fetch user usage information per organization.
NOTE: This endpoint supports the read-only pre-shared secret key.
- **`dnsfilter-cli distributors reports-usage-by-sku`** - Allows to fetch user usage information per SKU.

NOTE: it might be possible that the covered_users and estimated_users
values do not match the total_requests / 6000 math, since these values
are the sum of already rounded up values of organizations with the given
SKU.

NOTE: This endpoint supports the read-only pre-shared secret key.
- **`dnsfilter-cli distributors skus-available`** - Returns the list of SKUs available to the distributor's customers.
- **`dnsfilter-cli distributors update`** - Updates the specified support Distributor information.
- **`dnsfilter-cli distributors users-add-membership`** - Adds/creates a new user membership with an organization.
- **`dnsfilter-cli distributors users-create`** - Creates a new user alongside a membership with an organization.
- **`dnsfilter-cli distributors users-index`** - Allows to get the list of users that matches the search criteria. Note:
endpoint does pagination, be sure to fetch all available pages to get the
full list.
- **`dnsfilter-cli distributors users-remove-membership`** - Removes/deletes an existing user membership with an organization.
- **`dnsfilter-cli distributors users-reset-password-url`** - Creates a new reset password URL for the specified user.

The URL is valid for 432000 seconds (5 days).
- **`dnsfilter-cli distributors users-send-reset-password-email`** - Sends a reset password email to the specified user.
- **`dnsfilter-cli distributors users-show`** - Allows to fetch basic information about an user.
- **`dnsfilter-cli distributors users-update-membership`** - Updates an existing user membership with an organization.

### domains

Manage domains

- **`dnsfilter-cli domains bulk-lookup`** - Gets the domains and categories associated for each FQDN of the specified list.
- **`dnsfilter-cli domains suggest-threat`** - Mark a FQDN to verify its threat categorization.

Request example: `/v1/domains/suggest_threat?fqdn=XXXX&notes&test-report&categories=1,222,989`
- **`dnsfilter-cli domains user-lookup`** - Gets all the domains associated for a particular FQDN.

### enterprise-connections

Manage enterprise connections

- **`dnsfilter-cli enterprise-connections create`** - Create an enterprise connection.

Note: To set your organization's custom login url for enterprise
connections, you need to set the 'unique_id' attribute on the
organization itself. See the PATCH /v1/organizations endpoint.
- **`dnsfilter-cli enterprise-connections destroy`** - Delete an enterprise connection.
- **`dnsfilter-cli enterprise-connections index`** - Gets information for all the enterprise connections for the given organization.
- **`dnsfilter-cli enterprise-connections show`** - Gets information for the specified enterprise connection.
- **`dnsfilter-cli enterprise-connections update`** - Update an enterprise connection.

Note: To set your organization's custom login url for enterprise
connections, you need to set the 'unique_id' attribute on the
organization itself. See the PATCH /v1/organizations endpoint.

### invoices

Manage invoices

- **`dnsfilter-cli invoices current`** - Gets user estimated invoice for next month.
- **`dnsfilter-cli invoices index`** - Gets user associated invoices, most recent first.
- **`dnsfilter-cli invoices show`** - Gets basic information of the specified invoice.

### ip-addresses

Manage ip addresses

- **`dnsfilter-cli ip-addresses all`** - Gets ALL user associated IP addresses basic information.
- **`dnsfilter-cli ip-addresses create`** - Creates a new IP address with the specified data.
- **`dnsfilter-cli ip-addresses destroy`** - Removes an IP address.
- **`dnsfilter-cli ip-addresses index`** - Gets user associated IP addresses basic information.
- **`dnsfilter-cli ip-addresses myip`** - Responds with the requester reported IP address.
- **`dnsfilter-cli ip-addresses show`** - Gets basic information of the specified IP address.
- **`dnsfilter-cli ip-addresses update`** - Updates an IP address with the specified data.
- **`dnsfilter-cli ip-addresses verify`** - Verification call to ensure IP address is not already in the system.

### mac-addresses

Manage mac addresses

- **`dnsfilter-cli mac-addresses all`** - Gets ALL MAC addresses associated to an user with basic information.
- **`dnsfilter-cli mac-addresses create`** - Creates a new MAC address with the specified data.
- **`dnsfilter-cli mac-addresses destroy`** - Deletes a MAC address.
- **`dnsfilter-cli mac-addresses index`** - Gets the MAC addresses associated to an organization with basic information.
- **`dnsfilter-cli mac-addresses show`** - Gets basic information of the specified MAC address.
- **`dnsfilter-cli mac-addresses update`** - Updates a MAC address with the specified data.

### metrics

Manage metrics

- **`dnsfilter-cli metrics organization-usage`** - Gets usage information for the given Organization over the given date
range. Maximum date range is 365 days.
- **`dnsfilter-cli metrics organization-usage-detailed`** - Gets usage information for the given organization.

This differs from +organization_usage+ in the way +user_count+/+wifi_count+ are fetched from the DB,
also these endpoints returns a roaming client count.

### networks

Manage networks

- **`dnsfilter-cli networks all`** - Gets ALL networks associated to the user with its basic information.
- **`dnsfilter-cli networks bulk-create`** - Bulk create multiple networks with the specified data.
- **`dnsfilter-cli networks bulk-create-status`** - Bulk create status.
- **`dnsfilter-cli networks bulk-destroy`** - Bulk destroy multiple networks.
- **`dnsfilter-cli networks bulk-destroy-status`** - Bulk destroy status.
- **`dnsfilter-cli networks bulk-update`** - Bulk update multiple networks with the specified data.
- **`dnsfilter-cli networks bulk-update-status`** - Bulk update status
- **`dnsfilter-cli networks counts`** - Gets counts of networks for each network status.
- **`dnsfilter-cli networks create`** - Creates a new network with the specified data.
- **`dnsfilter-cli networks destroy`** - Deletes a network.
- **`dnsfilter-cli networks geo`** - Gets user's associated networks with geo information only.
- **`dnsfilter-cli networks index`** - Gets user's associated networks with basic information.
- **`dnsfilter-cli networks lookup`** - Gets basic information of a network based on its associated IP addresses.
- **`dnsfilter-cli networks msp`** - Gets MSP user associated networks with basic information.
- **`dnsfilter-cli networks msp-all`** - Gets ALL networks associated to the MSP user with its basic information.
- **`dnsfilter-cli networks show`** - Gets basic information of the specified network.
- **`dnsfilter-cli networks subnets-all`** - List site subnets with basic information.
- **`dnsfilter-cli networks update`** - Updates a network with the specified data.

### networks-csv-export

Manage networks csv export

- **`dnsfilter-cli networks-csv-export create`** - Creates a networks csv export record to track export of networks.
- **`dnsfilter-cli networks-csv-export show`** - Shows a networks csv export.

### notes

Manage notes

- **`dnsfilter-cli notes domain-show`** - Gets associated notes from a specific resource.
- **`dnsfilter-cli notes domain-update`** - Updates notes for a resource with the specified data.

### organizations

Manage organizations

- **`dnsfilter-cli organizations all`** - Gets all organizations basic information.
- **`dnsfilter-cli organizations bulk-update`** - Updates one or more organizations with the specified data.
- **`dnsfilter-cli organizations cancel`** - Sets an organization as 'Canceled'.
- **`dnsfilter-cli organizations create`** - Creates a new organization with the specified data.
- **`dnsfilter-cli organizations destroy`** - Deletes an MSP customer.
- **`dnsfilter-cli organizations index`** - Gets user's associated organizations basic information.
- **`dnsfilter-cli organizations promote-to-msp`** - Promote an organization to a MSP.
- **`dnsfilter-cli organizations settings`** - Gets basic information of the specified organization.
- **`dnsfilter-cli organizations show`** - Gets basic information of the specified organization.
- **`dnsfilter-cli organizations update`** - Updates an organization with the specified data.

### policies

Manage policies

- **`dnsfilter-cli policies add-allowed-application`** - Adds a single application to the allow list.
- **`dnsfilter-cli policies add-blacklist-category`** - Adds a single category to the blocklist.
- **`dnsfilter-cli policies add-blacklist-domain`** - Adds a single domain to the blocklist.
- **`dnsfilter-cli policies add-blocked-application`** - Adds a single application to the block list.
- **`dnsfilter-cli policies add-whitelist-domain`** - Adds a single domain to the allowlist.
- **`dnsfilter-cli policies all`** - Gets ALL user associated policies basic information.
- **`dnsfilter-cli policies application`** - Gets policies information for a specific application.
- **`dnsfilter-cli policies application-update`** - Updates a policy with the specified application data.
- **`dnsfilter-cli policies bulk-actions-add-allowlist-domains`** - Adds one or more domains to one or more policy's allow list.
- **`dnsfilter-cli policies bulk-actions-add-blocklist-domains`** - Adds one or more domains to one or more policy's block list.
- **`dnsfilter-cli policies bulk-actions-remove-allowlist-domains`** - Removes one or more domains from one or more policy's allow list.
- **`dnsfilter-cli policies bulk-actions-remove-blocklist-domains`** - Removes one or more domains from one or more policy's block list.
- **`dnsfilter-cli policies create`** - Creates a new policy with the specified data.
- **`dnsfilter-cli policies destroy`** - Deletes a policy.
- **`dnsfilter-cli policies index`** - Gets user associated policies basic information.
- **`dnsfilter-cli policies remove-allowed-application`** - Removes a single application from the allow list.
- **`dnsfilter-cli policies remove-blacklist-category`** - Removes a single category from the blocklist.
- **`dnsfilter-cli policies remove-blacklist-domain`** - Removes a single domain from the blocklist.
- **`dnsfilter-cli policies remove-blocked-application`** - Removes a single application from the block list.
- **`dnsfilter-cli policies remove-whitelist-domain`** - Removes a single domain from the allowlist.
- **`dnsfilter-cli policies show`** - Gets basic information of the specified policy.
- **`dnsfilter-cli policies update`** - Updates a policy with the specified data.

### policy-ips

Manage policy ips

- **`dnsfilter-cli policy-ips index`** - Gets user associated policies IPs basic information from database.
- **`dnsfilter-cli policy-ips show`** - Gets basic information of the specified Policy IP.

### psa-integrations

Manage psa integrations

- **`dnsfilter-cli psa-integrations`** - Creates a new PSA integration, if one does not exist and returns the redirect link.

### scheduled-policies

Manage scheduled policies

- **`dnsfilter-cli scheduled-policies all`** - Gets ALL scheduled policies associated with the current user.
- **`dnsfilter-cli scheduled-policies create`** - Creates a new scheduled policy.
- **`dnsfilter-cli scheduled-policies destroy`** - Removes a scheduled policy from the database.
- **`dnsfilter-cli scheduled-policies index`** - Gets the list of scheduled policies associated with the current user.
- **`dnsfilter-cli scheduled-policies show`** - Gets the specified scheduled policy.
- **`dnsfilter-cli scheduled-policies update`** - Updates a scheduled policy in the database with the specified data.

### scheduled-report-previews

Manage scheduled report previews

- **`dnsfilter-cli scheduled-report-previews create`** - Creates a scheduled report preview, triggering the background generation.
- **`dnsfilter-cli scheduled-report-previews show`** - Gets the specified scheduled report preview.

### scheduled-reports

Manage scheduled reports

- **`dnsfilter-cli scheduled-reports bulk-update`** - Pauses or resumes several schedules in one call  -  the grid's multi-select Active toggle. The
single-schedule equivalent is PATCH /v2/scheduled_reports/{id} carrying `active` alone.

Partial success is the contract: each schedule is saved independently, so one failure does not
roll back the rest. The response carries one entry per id sent, in the order sent.
- **`dnsfilter-cli scheduled-reports create`** - Creates a scheduled report.
- **`dnsfilter-cli scheduled-reports create-scheduledreports`** - Creates a schedule with the options the drawer exposes: a name, a report type, external
recipients, and the daily/quarterly cadences and day-of-month the older endpoint had no fields for.
- **`dnsfilter-cli scheduled-reports destroy`** - Deletes the scheduled report.
- **`dnsfilter-cli scheduled-reports destroy-scheduledreports`** - Deletes the schedule and its recipients outright. To stop deliveries without losing the schedule,
PATCH `active` to false instead  -  that is what the grid's pause control does.
- **`dnsfilter-cli scheduled-reports index`** - Get all scheduled reports associated with current user's organizations.
- **`dnsfilter-cli scheduled-reports index-scheduledreports`** - Returns a page of scheduled reports for the schedules grid, scoped to the organizations the caller
administers. Defaults to Next Delivery ascending.
- **`dnsfilter-cli scheduled-reports show`** - Gets the specified scheduled report.
- **`dnsfilter-cli scheduled-reports show-scheduledreports`** - Everything the Schedule Report drawer renders, including the internal/external recipient split and
the day-of-week or day-of-month the cadence pins the send to.
- **`dnsfilter-cli scheduled-reports update`** - Updates a scheduled report.
- **`dnsfilter-cli scheduled-reports update-scheduledreports`** - Also the pause/resume endpoint: the grid's Active toggle is a PATCH carrying `active` alone.
Pausing never touches next_report_date. Resuming a schedule whose next_report_date is before today
reschedules it to the cadence's first occurrence from today, as for a new schedule (no catch-up
send); a date of today or later is kept.

### traffic-reports

Manage traffic reports

- **`dnsfilter-cli traffic-reports qps`** - Gets the total number of queries per second in a period of time for sites.
- **`dnsfilter-cli traffic-reports qps-active-agents`** - Gets the total number of queries per second in a period of time
(maximum 20 minutes) for roaming clients.
- **`dnsfilter-cli traffic-reports qps-active-organizations`** - Gets the total number of queries per second in a period of time
(maximum 20 minutes) for organizations.
- **`dnsfilter-cli traffic-reports query-logs`** - Gets the query raw logs in a period of time.
- **`dnsfilter-cli traffic-reports top-agents`** - Gets the top agents in a period of time.
- **`dnsfilter-cli traffic-reports top-application-categories`** - Gets the top application categories domains in a period of time.
- **`dnsfilter-cli traffic-reports top-categories`** - Gets the top categories domains in a period of time.
- **`dnsfilter-cli traffic-reports top-collections`** - Gets the top collections in a period of time.
- **`dnsfilter-cli traffic-reports top-domains`** - Gets the top requested domains in a period of time.
- **`dnsfilter-cli traffic-reports top-networks`** - Gets the top networks in a period of time.
- **`dnsfilter-cli traffic-reports top-organizations-requests`** - Gets the top organizations by number of requests.
- **`dnsfilter-cli traffic-reports top-users`** - Gets the top users in a period of time.
- **`dnsfilter-cli traffic-reports total-applications-agents-stats`** - Gets the statistics of number of requests for roaming clients
by application in a period of time.
- **`dnsfilter-cli traffic-reports total-applications-collections-stats`** - Gets the statistics of number of requests for collections
by application in a period of time.
- **`dnsfilter-cli traffic-reports total-applications-networks-stats`** - Gets the statistics of number of requests for sites by application in a period of time.
- **`dnsfilter-cli traffic-reports total-applications-stats`** - Gets the statistics of number of requests by application in a period of time.
- **`dnsfilter-cli traffic-reports total-applications-users-stats`** - Gets the statistics of number of requests for users by
application in a period of time.
- **`dnsfilter-cli traffic-reports total-categories`** - Gets the total number of requests by category for sites in a period of time.
- **`dnsfilter-cli traffic-reports total-categories-organizations`** - Gets the total number of requests by category for organizations in a period of time.
- **`dnsfilter-cli traffic-reports total-category-stats`** - Gets the total number of stats for a category in a period of time.
- **`dnsfilter-cli traffic-reports total-client-stats`** - Gets the organization network, users and client stats.
- **`dnsfilter-cli traffic-reports total-deployments`** - Gets the organization deployments information.
- **`dnsfilter-cli traffic-reports total-domain-stats`** - Gets the total number of stats for a domain in a period of time.
- **`dnsfilter-cli traffic-reports total-domains`** - Gets the total number of requests by domain for sites in a period of time.
- **`dnsfilter-cli traffic-reports total-domains-organizations`** - Gets the total number of requests by domain for organizations in a period of time.
- **`dnsfilter-cli traffic-reports total-domains-users`** - Gets the total number of requests by domain for users in a period of time.
- **`dnsfilter-cli traffic-reports total-organizations-requests`** - Gets the total number of requests for organizations in a period of time.
- **`dnsfilter-cli traffic-reports total-organizations-stats`** - Gets the total number of stats for organizations in a period of time.
- **`dnsfilter-cli traffic-reports total-requests`** - Gets the total number of requests for sites in a period of time.
- **`dnsfilter-cli traffic-reports total-requests-agents`** - Gets the total number of requests for roaming clients in a period of time.
- **`dnsfilter-cli traffic-reports total-requests-collections`** - Gets the total number of requests for collections in a period of time.
- **`dnsfilter-cli traffic-reports total-requests-geo`** - Gets the total number of requests for organizations in a period of time
for the heatmap on overview.
- **`dnsfilter-cli traffic-reports total-requests-organizations`** - Gets the total number of requests for organizations in a period of time.
- **`dnsfilter-cli traffic-reports total-requests-users`** - Gets the total number of requests for users in a period of time.
- **`dnsfilter-cli traffic-reports total-roaming-clients`** - Gets the roaming clients information.
- **`dnsfilter-cli traffic-reports total-threats`** - Gets the total number of threats for sites in a period of time.
- **`dnsfilter-cli traffic-reports total-threats-organizations`** - Gets the total number of threats for organizations in a period of time.
- **`dnsfilter-cli traffic-reports total-threats-users`** - Gets the total number of threats for users in a period of time.

### trials

Manage trials

- **`dnsfilter-cli trials`** - Create trial.

### unblock-requests

Admin queue for DNS unblock requests. Feeds the two dashboard grids: Active Requests
(awaiting a decision) and Request History (resolved). Gated behind the :enable_unblock_requests feature flag.

- **`dnsfilter-cli unblock-requests counts`** - Returns the Active and History tallies for the tab badges, scoped identically to index.
- **`dnsfilter-cli unblock-requests index`** - Returns a page of unblock requests for the selected tab, scoped to the organizations the caller can
see the queue for (policy managers and read-only members alike). Active Requests default to Time of
Attempt ascending; Request History defaults to Resolved Date descending. Each row carries
`can_action`, which is false when the caller may view the request but not decide it (read-only).

Column filters. Each filter is a top-level param carrying either a bare value (uses the
column's default operator) or an object: `field[value]=…&field[operator]=…`. All filters AND together,
and combine with `search`/`tab`. An unsupported operator for a field returns 422.
- **`dnsfilter-cli unblock-requests pending-count`** - Sidebar notification badge count. Returns the total number of pending unblock requests across the
organizations the caller can see the queue for  -  MSP admins get the aggregate across all sub-orgs, and
read-only members get the tally for the queue they can view  -  for the sidebar dot and numeric badge.
Reflects auto-expiry: a request past its expiry window is not counted even
before the sweep runs. Returns 0 (never null) when there are none.
- **`dnsfilter-cli unblock-requests show`** - Everything the Approve/Deny drawer renders for a single unblock request: request context, the
allow-list targets (the request's policy list + the org Universal Allow List) with each target's
blast radius and whether the domain is already covered (evaluated at call time), and the threat
flag. A request that is no longer pending (already actioned by another admin, or auto-expired)
returns 409 so the caller can distinguish it from an unknown id (404).

### unblock-requests-csv-export

Async CSV export for the two Unblock Request grids. One endpoint serves both tabs via the
`tab` param. create snapshots the exact filtered / searched / sorted view (as an ordered uuid set,
capped at a server-side row cap) and enqueues the CSV build; show polls until the presigned
URL (or an error) is ready. It reuses the grid's query pipeline verbatim through
the list endpoint's filter logic, so the export always matches what the admin sees, and is gated behind the
:enable_unblock_requests feature flag exactly like the grid.

- **`dnsfilter-cli unblock-requests-csv-export create`** - Snapshots the currently filtered / searched / sorted view for the selected tab (capped at
a server-side row cap rows, in the grid's sort order) and kicks off the async CSV build.
Returns the tracking record immediately  -  poll show for the download URL. The export set is captured
at request time (point-in-time); the row data is read when the job runs.

Column filters (domain, organization, site, policy, category, time_of_attempt, logged_in_user, email,
request_reason, and the History-only actioned_by, resolved_date, status) are accepted exactly as on
the grid  -  see UnblockRequestsControllerindex.
- **`dnsfilter-cli unblock-requests-csv-export show`** - Returns the tracking record with the presigned `url` once the build finished (or `error` if it
failed), and 202 with a "processing" status while the job is still running.

### user-agent-bulk-deletes

Manage user agent bulk deletes

- **`dnsfilter-cli user-agent-bulk-deletes counts`** - Gets a user agent bulk delete counts by id.
- **`dnsfilter-cli user-agent-bulk-deletes create`** - Create a user agent bulk delete.
- **`dnsfilter-cli user-agent-bulk-deletes show`** - Gets a user agent bulk delete by id.

### user-agent-bulk-updates

Manage user agent bulk updates

- **`dnsfilter-cli user-agent-bulk-updates counts`** - Gets a user agent bulk delete counts by id.
- **`dnsfilter-cli user-agent-bulk-updates create`** - Create a user agent bulk update.
- **`dnsfilter-cli user-agent-bulk-updates has-mixed`** - Gets the value of whether each of the attributes of the elected user agents are mixed.
- **`dnsfilter-cli user-agent-bulk-updates show`** - Gets a user agent bulk update by id.

### user-agent-cleanups

Manage user agent cleanups

- **`dnsfilter-cli user-agent-cleanups create`** - Creates a user agent cleanup to track bulk deletion of user agents.
- **`dnsfilter-cli user-agent-cleanups show`** - Gets the specific user agent cleanup.
- **`dnsfilter-cli user-agent-cleanups update`** - Updates a user agent cleanup.

### user-agent-csv-exports

Manage user agent csv exports

- **`dnsfilter-cli user-agent-csv-exports create`** - Creates a user agent csv export record to track export of user agents.
- **`dnsfilter-cli user-agent-csv-exports show`** - Gets the specific user agent csv export.

### user-agent-releases

Manage user agent releases

- **`dnsfilter-cli user-agent-releases index`** - Gets a list of latest user agent releases for each unique combination of
agent_type, architecture, release_channels, and white label.
- **`dnsfilter-cli user-agent-releases relay`** - Gets a list of latest relay releases for each unique combination of
architecture, release_channels, and white label.

### user-agents

Manage user agents

- **`dnsfilter-cli user-agents all`** - Gets a list of ALL user agents with basic information.
- **`dnsfilter-cli user-agents counts`** - Gets counts of user agents for each status.
- **`dnsfilter-cli user-agents csv`** - Gets CSV of organization's user agents.
- **`dnsfilter-cli user-agents dequeue-uninstall`** - Removes user agent from uninstall queue.
- **`dnsfilter-cli user-agents destroy`** - Soft deletes an user agent.
- **`dnsfilter-cli user-agents index`** - Gets a list of user agents with basic information.
- **`dnsfilter-cli user-agents show`** - Gets the specified user agent with basic information.
- **`dnsfilter-cli user-agents tags`** - Gets ALL tags used by user agents on a network or organization.
- **`dnsfilter-cli user-agents uninstall-pin`** - Gets Uninstall pin for organization's user agents.
- **`dnsfilter-cli user-agents update`** - Updates an user agent with the specified data.

### users

Manage users

- **`dnsfilter-cli users all`** - Gets ALL users basic information.
- **`dnsfilter-cli users change-password`** - Changes the password for the currently authenticated user.
- **`dnsfilter-cli users index`** - Gets users basic information.
- **`dnsfilter-cli users show`** - Gets basic information of the specified user.


### agents


- **`dnsfilter-cli agents drift`** - See which client orgs have roaming clients behind the latest agent release, or on a specific bad build, with auto-update state per org. Joins the synced fleet with the vendor's live release feed.

- **`dnsfilter-cli agents stale`** - List roaming clients that stopped checking in or are unprotected or bypassed, grouped by client org with age buckets. Reads the local mirror; sync first.


### allow


- **`dnsfilter-cli allow`** - Add a domain to the allow list of the policies that blocked it for a client (from the query log, else the site or roaming-client assignment). Previews by default; writes only with --confirm --note, then reads each policy back.


### lists


- **`dnsfilter-cli lists where`** - Find every policy across every client whose allow or block list contains a domain or one of its parent domains, with counts of the sites and roaming clients it governs. Reads the local mirror; sync first.


### policy-audit


- **`dnsfilter-cli policy-audit`** - Flag allow-listed domains now categorized as threats, unblocked security categories, and allowed unknown domains; with --baseline <policy-id>, also drift from that baseline policy.


### unblocks


- **`dnsfilter-cli unblocks triage`** - Review pending end-user unblock requests grouped by domain across every client, with threat flags, age, and DNSFilter's own already-covered check. Needs unblock requests enabled on the account.


### why-blocked


- **`dnsfilter-cli why-blocked`** - Name the site or roaming-client policy and the rule that blocks (or allows) a domain for one client org, with recent query-log hits showing the policy that actually applied. User, collection and LAN-subnet assignments, Universal Lists and AppAware rules are not evaluated.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`dnsfilter-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`dnsfilter-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`dnsfilter-cli learnings list`** - Inspect taught rows
- **`dnsfilter-cli learnings forget <query>`** - Undo a teach
- **`dnsfilter-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`dnsfilter-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`dnsfilter-cli teach-pattern`** - Install a query/resource template up front
- **`dnsfilter-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `DNSFILTER_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `dnsfilter-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
dnsfilter-cli agent-local-user-bulk-deletes counts --id 550e8400-e29b-41d4-a716-446655440000

# JSON for scripting and agents
dnsfilter-cli agent-local-user-bulk-deletes counts --id 550e8400-e29b-41d4-a716-446655440000 --json
# Filter to specific fields
dnsfilter-cli agent-local-user-bulk-deletes counts --id 550e8400-e29b-41d4-a716-446655440000 --json --select count

# Dry run  -  show the request without sending
dnsfilter-cli agent-local-user-bulk-deletes counts --id 550e8400-e29b-41d4-a716-446655440000 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
dnsfilter-cli agent-local-user-bulk-deletes counts --id 550e8400-e29b-41d4-a716-446655440000 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **No hidden prompts** - API writes and deletes run immediately; preview them with `--dry-run`. `allow` only writes with `--confirm`; `--yes` is only needed for local `profile delete` and cache purges
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
dnsfilter-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `dnsfilter-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/dnsfilter-cli/config.toml` (`%USERPROFILE%\.config\dnsfilter-cli\config.toml` on Windows); `--home`, `DNSFILTER_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `DNSFILTER_API_KEY` | per_call | Yes | Set to your API credential. |
| `DNSFILTER_BASE_URL` | config | No | Override the API base URL (default `https://api.dnsfilter.com`). |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `dnsfilter-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `dnsfilter-cli doctor` to check credentials
- Verify the environment variable is set: `echo $DNSFILTER_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the resource's `index` or `all` command (e.g. `dnsfilter-cli organizations all`) to see available items

### API-specific
- **Not sure which credential the CLI is using**  -  Run dnsfilter-cli auth status; it reports the credential source (env:DNSFILTER_API_KEY or the config file) without calling the API.
- **HTTP 401 {"error":"Not Authorized"}**  -  Set DNSFILTER_API_KEY to the raw key from Account Settings, API Tokens (no Bearer prefix) and run dnsfilter-cli doctor.
- **HTTP 429 Too Many Requests**  -  The CLI honors Retry-After; for large fleets sync fewer resources per run, e.g. dnsfilter-cli sync --resources user-agents.
- **agents drift, agents stale or lists where prints an empty list**  -  Populate the mirror first: dnsfilter-cli sync --resources organizations,networks,policies,user-agents.
- **sync warns on distributors resources**  -  Distributor endpoints need a distributor key; MSP keys can ignore the warning or sync only MSP resources with --resources.
- **allow wrote nothing**  -  allow only previews by default; re-run with --confirm --note "<ticket>" to apply.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Pipedream DNSFilter components**](https://github.com/PipedreamHQ/pipedream)  -  JavaScript (11738 stars)
- [**Metorial DNSFilter integration**](https://github.com/metorial/metorial)  -  TypeScript (3366 stars)
- [**DNSFilterAPI (PowerShell)**](https://github.com/christaylorcodes/DNSFilterAPI)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
