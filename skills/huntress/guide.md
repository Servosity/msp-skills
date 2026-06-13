# Huntress CLI

**Every Huntress endpoint, plus fleet-wide incident, coverage, and billing rollups the API can't.**

huntress-cli absorbs the full Huntress API  -  organizations, agents, incident reports, remediations, signals, escalations, identities, external recon, reports, invoices, reseller subscriptions, and SIEM ES|QL  -  with agent-native output (--json, --select, typed exit codes). Then it transcends the read-mostly, per-org API: fleet-incidents gives one age-sorted queue across every client org, coverage-gaps rolls up posture exposure, blast-radius correlates an indicator across the whole fleet, and drift/mttr/handoff turn repeated syncs into history the live API throws away.

## Install

The recommended path installs both the `huntress-cli` binary and the `pp-huntress` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install huntress
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install huntress --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install huntress --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install huntress --agent claude-code
npx -y @mvanhorn/printing-press-library install huntress --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/huntress/cmd/huntress-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/huntress-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install huntress --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-huntress --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-huntress --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install huntress --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/huntress-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `HUNTRESS_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/huntress/cmd/huntress-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "huntress": {
      "command": "huntress-mcp",
      "env": {
        "HUNTRESS_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Huntress uses HTTP Basic auth: set HUNTRESS_API_KEY and HUNTRESS_API_SECRET (minted in Account Settings, or reseller-level for multi-account partners). The CLI composes the Base64 Authorization header for you. Most data is read-only; resolve/approve/reject and CRUD commands need a credential with write scope.

## Quick Start

```bash
# Confirm auth works and see which account you're keyed into.
huntress-cli account --json

# Mirror every entity into the local store so the fleet commands have data to join.
huntress-cli sync

# The morning sweep: every open critical across all client orgs, oldest first.
huntress-cli fleet-incidents --severity critical --status sent --sort age --json

# Where are agents stale or unhealthy across the fleet.
huntress-cli coverage-gaps --stale-days 7 --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Fleet rollups across every org
- **`fleet-incidents`**  -  One unified, age-sorted incident queue across every client organization, with org names joined in  -  the morning-sweep view the dashboard can't give.

  _Reach for this when an agent needs the single cross-tenant 'what's on fire everywhere' queue instead of paging org-by-org._

  ```bash
  huntress-cli fleet-incidents --severity critical --status sent --sort age --json
  ```
- **`coverage-gaps`**  -  Flags orgs and agents with stale callbacks, unhealthy Defender/firewall, or outdated EDR versions  -  a fleet posture exposure report.

  _Use before a weekly posture review or onboarding audit to find protection gaps before an incident lands on an unmonitored host._

  ```bash
  huntress-cli coverage-gaps --stale-days 7 --json
  ```
- **`canary-watch`**  -  Surfaces only ransomware-canary and foothold incidents in a time window  -  the highest-signal early-ransomware indicators, fleet-wide.

  _Reach for this as a high-signal early-warning sweep when you want only the indicators that precede ransomware._

  ```bash
  huntress-cli canary-watch --window 24h --json
  ```
- **`stale-agents`**  -  Lists agents whose last callback exceeds a threshold  -  decommissioned-but-billed machines or broken installs.

  _Use to clean up the fleet and feed billing-reconcile with the list of agents that stopped reporting._

  ```bash
  huntress-cli stale-agents --days 14 --platform windows --json
  ```
- **`fleet-summary`**  -  One-screen fleet top-line: total orgs, agents, open criticals, oldest unactioned critical, orgs below coverage threshold, stale agents.

  _Run this first each shift  -  it tells an agent which detail command (fleet-incidents, coverage-gaps, stale-agents) to reach for next._

  ```bash
  huntress-cli fleet-summary --agent
  ```

### Cross-entity correlation
- **`blast-radius`**  -  Given an indicator (external IP, file hash, or foothold signature), finds every agent, org, and incident that matches it  -  instant correlation during incident response.

  _Reach for this mid-incident to answer 'where else does this indicator appear across my whole fleet' in one call._

  ```bash
  huntress-cli blast-radius --indicator 203.0.113.7 --json
  ```
- **`triage-age`**  -  SLA aging report: open incidents bucketed by hours-open, broken out by org and severity, with breaches flagged.

  _Use to enforce response SLAs across tenants and surface the oldest unactioned criticals first._

  ```bash
  huntress-cli triage-age --buckets 4,24,72 --json
  ```
- **`org-scorecard`**  -  Per-client QBR scorecard: agent count, coverage percent, open and closed incidents, MTTR, and a posture grade  -  one client's security story.

  _Use to assemble a client quarterly-business-review summary in one command instead of stitching several endpoints._

  ```bash
  huntress-cli org-scorecard --org 4821 --json
  ```
- **`incident-detail`**  -  See one incident fully enriched  -  its remediations, the affected agent, and the org name  -  in a single lookup.

  _Reach for this when triaging a specific incident: one call replaces the incident/remediation/agent/org fan-out._

  ```bash
  huntress-cli incident-detail 123456 --agent
  ```

### Partner ops and billing
- **`billing-reconcile`**  -  Compares invoiced and subscribed seat counts against actually deployed agent counts per org and surfaces the delta.

  _Run at monthly close to catch decommissioned-but-billed seats and under-billed new deployments._

  ```bash
  huntress-cli billing-reconcile --json
  ```
- **`reseller-rollup`**  -  Per-account roll-up for resellers: invoice total, subscribed seats, and deployed agent count side by side.

  _Live-only: calls the API directly and requires reseller-scoped credentials (standard account keys get 401; no sync needed). Use at month close for multi-account resellers; for per-org drift inside one account use billing-reconcile instead._

  ```bash
  huntress-cli reseller-rollup --json
  ```

### Local history that compounds
- **`drift`**  -  Diffs the current sync against the prior snapshot: new and removed agents, status flips, new criticals, and version changes.

  _Use after each sync to see what changed across the fleet without re-reading every record._

  ```bash
  huntress-cli drift --entity agents --json
  ```
- **`mttr`**  -  Computes mean time-to-resolve for incidents from sent-to-resolved timestamps, grouped by org or severity.

  _Use for QBR and SOC performance reporting where mean response time is the headline number._

  ```bash
  huntress-cli mttr --group-by org --since 30d --json
  ```
- **`handoff`**  -  Shift-change report of what changed (new criticals, resolutions, escalations) since a timestamp, ready to paste into a handoff note.

  _Use at shift change so the next analyst sees exactly what moved during the prior shift._

  ```bash
  huntress-cli handoff --since 8h --json
  ```

## Recipes


### Cross-tenant critical queue

```bash
huntress-cli fleet-incidents --severity critical --status sent --sort age --json
```

One age-sorted queue of every open critical across all client orgs.

### Posture exposure sweep

```bash
huntress-cli coverage-gaps --stale-days 7 --json
```

Orgs and agents with stale callbacks or unhealthy Defender/firewall, rolled up per org.

### Indicator correlation during IR

```bash
huntress-cli blast-radius --indicator 203.0.113.7 --json
```

Every agent, org, and incident touching an external IP, hash, or foothold.

### Trim verbose fleet-incident payloads

```bash
huntress-cli fleet-incidents --status sent --agent --select organization_name,severity,hours_open,indicator_types
```

fleet-incidents joins org names and returns a wide row per incident; --agent with --select returns only the fields you need so agents don't burn context.

### Billing true-up at month close

```bash
huntress-cli billing-reconcile --json
```

Invoiced/subscribed seats versus actually deployed agents per org, with the delta.

## Usage

Run `huntress-cli --help` for the full command reference and flag list.

## Commands

### account

Operations about Accounts

- **`huntress-cli account`** - Shows details of the top-level Huntress Account associated with your API credentials.

### accounts

Operations about Accounts

- **`huntress-cli accounts creation-parameters`** - Create a new account under the reseller associated with the supplied API credential.
- **`huntress-cli accounts delete-v1-id`** - Marks the account as disabled and will be deleted after 10 days from initial request.

**Please Note:** This is irreversible and will uninstall all of the agents for this account, as well as completing other similar operations. 
[Contact support](https://support.huntress.io/hc/en-us) if this was done unintentionally.
- **`huntress-cli accounts get-v1`** - Shows all accounts associated with your API credentials.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli accounts get-v1-id`** - Shows the details of a specific account which your API credentials grant access to.
- **`huntress-cli accounts update-parameters`** - Updates the details of a specific account.

### actor

Operations about Actors

- **`huntress-cli actor`** - Shows details of the entities associated with the supplied API credentials. It will only return the fields relevant to the current credentials.
For more information on User management, see [Product Support](https://support.huntress.io/hc/en-us/articles/4404012574227-Adding-and-Managing-Huntress-Users)

### agents

Operations about Agents

- **`huntress-cli agents get-v1`** - Shows Agents associated with your account.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli agents get-v1-id`** - Shows details on a single Agent associated with your account.

### escalations

Operations about Escalations

- **`huntress-cli escalations get-v1`** - Shows Escalations associated with your account.
Additional details for a specific escalation can be obtained by using the **GET Escalation** endpoint.

Escalations are used to notify Huntress account administrators that a situation requires their attention.
Below are some common use cases:
 - The Huntress security platform is unable to send incident reports to your PSA system and we need you to reconfigure the integration.
 - Security Operation Centers (SOC) suspect that an application being flagged as malicious is a false positive, and we want to get your authorization to allow-list the application moving forward.
 - A potential threat flagged by Managed Defender requires additional information (file path details, etc.) in order for Huntress to provide actionable assisted remediation steps.
 - A login event occurred from an unexpected country or VPN, and Huntress would like partner feedback on whether that event should be expected or unauthorized.

 Though Escalations are not incident reports, they do have severities (low, high, critical) associated with them that dictate an expected response time.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli escalations get-v1-id`** - Shows details on a single Escalation associated with your account.

### external-ports

Manage external ports

- **`huntress-cli external-ports get-v1`** - Shows external port records from External Recon scans associated with your account.
- **`huntress-cli external-ports get-v1-id`** - Shows details on a single external port record associated with your account.

### identities

Operations about Identities

- **`huntress-cli identities get-v1`** - Shows Identities associated with your account.

**Note:** This endpoint will also return a `pagination` key on the root level.
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli identities get-v1-id`** - Shows details on a single Identity associated with your account.

### incident-reports

Operations about Incident Reports

- **`huntress-cli incident-reports get-v1`** - Shows Incident Reports associated with your account.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli incident-reports get-v1-id`** - Shows details on a single Incident Report associated with your account.

### invoices

Operations about Invoices

- **`huntress-cli invoices get-v1`** - Shows Invoices associated with your account.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli invoices get-v1-id`** - Shows details on a single Invoice associated with your account.

### known-vpns

Operations about Known VPNs

- **`huntress-cli known-vpns`** - Returns the list of VPN and proxy operators recognized by Huntress.

Use any value from this list as the `vpn` parameter when creating an Unwanted Access Rule.

### memberships

Manage memberships

- **`huntress-cli memberships creation-parameters`** - This endpoint allows you to invite a user to join your organization or
account.  A user will often be a person you wish to grant access to,
but it could also represent a team, an automated system, or any other
type of actor.

If an organization ID is provided, the user will be invited to that
organization. If not, they will be invited to the account associated
with this API credential. Note that while the sample return value
includes both an organization and an account for completeness, in
practice, only one or the other will be included.

Note that this is technically creating a Membership Invitation - the
actual membership won't be created until the user accepts the
invitation.
- **`huntress-cli memberships delete-v1-id`** - Deletes a single Membership associated with your account or organization. Does not delete the user associated with the membership.
- **`huntress-cli memberships get-v1`** - Shows a list of memberships.

By default, this endpoint returns both account and organization
memberships, but if an organization ID is supplied, it will return
only organization memberships, instead.

The example return value shows both an organization and an account, but
a given membership will only have one or the other.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli memberships get-v1-id`** - Shows details on a single Membership associated with your account or organization.
- **`huntress-cli memberships update-parameters`** - Update a User's membership

### organizations

Operations about Organizations

- **`huntress-cli organizations creation-parameters`** - Create an Organization
- **`huntress-cli organizations delete-v1-id`** - Deletes the specified Organization.

**Please note:** This will remove the organization and associated configurations across the Huntress Platform, including Managed SAT. For more information, see our [offboarding guide](https://support.huntress.io/hc/en-us/articles/51332785737235-Huntress-Product-Offboarding-Guide).
- **`huntress-cli organizations get-v1`** - Shows details of Organizations belonging to the account associated with your API credentials.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli organizations get-v1-id`** - Shows details on a single Organization associated with your account.
- **`huntress-cli organizations update-parameters`** - Update an Organization

### reports

Manage reports

- **`huntress-cli reports get-v1`** - Shows Summary Reports associated with your account.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli reports get-v1-id`** - Shows details on a single Summary Report associated with your account.

### reseller

Operations for Reseller-level API credentials. These are mostly the same endpoints available in the rest of the API. However, the account ID is included in the URL, so that you can specify which account's resources you want to access.

- **`huntress-cli reseller get-v1-invoices`** - Shows Invoices associated with the current reseller.

**Note:** To see the details of a given invoice, you will
probably want to also fetch the associated Account Usage Line Items and
Organization Usage Line Items.

**Note:** This endpoint will also return a `pagination` key on the root
level. Please refer to the [pagination
section](https://api.huntress.io/docs#pagination) within our docs for
more information.
- **`huntress-cli reseller get-v1-invoices-id`** - Shows a specific Reseller Invoice associated with the current
reseller.

Note: To see the details of this invoice, you will probably
want to also fetch the associated Account Usage Line Items and
Organization Usage Line Items.
- **`huntress-cli reseller get-v1-invoices-id-account-usage-line-items`** - Shows a list of Account Usage Line Items.

This list provides a detailed breakdown of product usage per account from a given invoice.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli reseller get-v1-invoices-id-organization-usage-line-items`** - Shows a list of Organization Usage Line Items.

This list provides a detailed breakdown of product usage per organization from a given invoice.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli reseller get-v1-subscriptions`** - Shows subscriptions associated with the current reseller's managed accounts.

**Note:** This endpoint will also return a `pagination` key on the root
level. Please refer to the [pagination
section](https://api.huntress.io/docs#pagination) within our docs for
more information.
- **`huntress-cli reseller get-v1-subscriptions-id`** - Shows details on a single subscription associated with the current reseller's managed accounts.
- **`huntress-cli reseller subscription-creation-parameters`** - Creates a subscription for a product on a reseller-managed account.

**Note:** This endpoint only allows the creation of subscriptions that
use the default terms, conditions, and pricing. Please contact your
account admin for any terms that are not covered by our standard API.
- **`huntress-cli reseller subscription-update-parameters`** - Updates a subscription associated with the current reseller's managed accounts.

For **approved** subscriptions: updates minimum, billing_interval, and purchase_order.

For **active** subscriptions: toggles `auto_renew` and/or adds units via `additional_units` (with optional `purchase_order`).
- **`huntress-cli reseller subscription-upgrade-parameters`** - Upgrades an active subscription by creating a new subscription with a
higher minimum and/or price tier, replacing the existing one.

This is modeled as a sub-resource because the operation creates a new
subscription record rather than modifying the existing one in place.

### siem

Query your SIEM logs programmatically using <a href="https://support.huntress.io/hc/en-us/articles/30113222043155-Searching-Logs-ESQL">ES|QL (Elasticsearch Query Language)</a>.

- **`huntress-cli siem`** - Execute an ESQL query against your SIEM logs and receive paginated JSON results.

This endpoint uses POST so that the ESQL query string can be sent in the request body
rather than as a URL query parameter, avoiding URL length limits for complex queries.

Queries must begin with `FROM logs`. Results are limited to 200 rows per page.
If `next_page_token` is present, pass it as `page_token` in a subsequent request
(with the same `range_start` and `range_end`) to retrieve the next page.

**Response**

Returns a JSON object with two top-level keys:

- `logs`  -  Array of objects. Each object represents one log record. Keys are ECS field
  names (e.g. `event.provider`, `host.hostname`). The fields present depend on the columns
  selected by your ESQL query (e.g. a `KEEP` command). With no column selection, all
  available ECS fields are returned.

- `pagination`  -  Object. Contains `next_page_token` (string) when additional results are
  available; empty object `{}` when all results have been returned. Pass `next_page_token`
  as `page_token` in your next request to retrieve the following page.

### signals

Operations about Signals

- **`huntress-cli signals get-v1`** - Shows details of Signals belonging to the account associated with your API credentials.

Signals are used to highlight interesting user or system behaviors that an analyst can reference during a cyber investigation.
A detected Signal could be as broad and low fidelity as the detection of a command line user running whoami, or it could be as specific and high fidelity as detecting a known malware file.

**Note:** This endpoint will also return a `pagination` key on the root level.  
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli signals get-v1-id`** - Shows details of a single Signal belonging to the account associated with your API credentials.

Signals are used to highlight interesting user or system behaviors that an analyst can reference during a cyber investigation.
A detected Signal could be as broad and low fidelity as the detection of a command line user running whoami, or it could be as specific and high fidelity as detecting a known malware file.

### unwanted-access-rules

Operations about Unwanted Access Rules

- **`huntress-cli unwanted-access-rules creation-parameters`** - Creates a new Unwanted Access Rule associated with your account, an organization, or a specific identity.

**Rule logic.** Provide exactly one of `country_code`, `vpn`, or `logic`:
  - Omit `logic` and supply `country_code` or `vpn` to create a `standard` rule that matches a single value.
  - Set `logic` as `catchall` (with `category`) to create a catchall rule that matches every value in the category. Catchalls must be `unauthorized` and may only be scoped to the account or an organization. An account or organization may have at most one catchall per category.
  - Set `logic` as `catchall_exception` (with `category`) to create an exception that opts an organization out of an account-level catchall. Exceptions must be `expected`, may only be scoped to an organization, and must omit `starts_at`/`expires_at`.

**Scope.** The rule scope is determined by the IDs supplied: provide `identity_id` to scope the rule to a single identity, `organization_id` to scope it to an organization. Omitting both scopes the rule at the account level.
- **`huntress-cli unwanted-access-rules delete-v1-id`** - Deletes a single Unwanted Access Rule associated with your account. Standard, catchall, and catchall exception rules can all be deleted through this endpoint.
- **`huntress-cli unwanted-access-rules get-v1`** - Shows Unwanted Access Rules associated with your account.

Unwanted Access Rules govern how Huntress responds to identity access attempts matching specific attributes. Each rule targets a category (country or vpn) and declares a determination  -  `expected` or `unauthorized`  -  at the account, organization, or identity scope.

**Note:** This endpoint will also return a `pagination` key on the root level.
Please refer to the [pagination section](https://api.huntress.io/docs#pagination) within our docs for more information.
- **`huntress-cli unwanted-access-rules get-v1-id`** - Shows details on a single Unwanted Access Rule associated with your account.
- **`huntress-cli unwanted-access-rules update-parameters`** - Updates the schedule and notes on an existing Unwanted Access Rule. The rule's category, value, scope, logic, and type cannot be changed.

Standard, catchall, and catchall exception rules can all be updated, but catchall and catchall exception rules must keep `starts_at` and `expires_at` nil. Passing a value for those fields on those rules will return a 422.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
huntress-cli account

# JSON for scripting and agents
huntress-cli account --json

# Filter to specific fields
huntress-cli account --json --select id,name,status

# Dry run  -  show the request without sending
huntress-cli account --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
huntress-cli account --agent
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
huntress-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/huntress-reference-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `HUNTRESS_API_KEY` | per_call | Yes |  |
| `HUNTRESS_API_SECRET` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `huntress-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `huntress-cli doctor` to check credentials
- Verify the environment variable is set: `echo $HUNTRESS_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set both HUNTRESS_API_KEY and HUNTRESS_API_SECRET; the header is Base64(key:secret). Run `huntress-cli doctor`.
- **Fleet commands return empty**  -  Run `huntress-cli sync` first  -  fleet-incidents/coverage-gaps/drift read the local store, not the live API.
- **drift or mttr shows nothing**  -  These need at least two syncs of history; run `sync` again after some time has passed.
- **List truncates at 10 rows**  -  Default page is 10; pass `--limit 500` or `--all` to walk every page via next_page_token.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**huntress-mcp-server**](https://github.com/DynamicEndpoints/huntress-mcp-server)  -  TypeScript
- [**PSHuntress**](https://github.com/joshuabennett-com/PSHuntress)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
