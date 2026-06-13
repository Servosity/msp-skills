# Syncro CLI

**Every Syncro PSA and RMM workflow in your terminal, plus a local database, offline search, and cross-entity reports no other Syncro tool has.**

syncro-cli mirrors your Syncro tenant into a local SQLite store so tickets, customers, assets, invoices, and RMM alerts become joinable, searchable, and fast. On top of full API coverage it adds reports Syncro never rendered: uninvoiced time, invoice drift, AR aging, customer profitability, SLA aging triage, and fleet-wide patch gaps. Built for agents with --json, --select, --dry-run, typed exit codes, and adaptive backoff for Syncro's 180-requests-per-minute limit. The cross-entity reports read the local store, so run `sync` first; some (uninvoiced, drift, margin, patch-gaps, aging) also need sub-resources like timer entries, patches, and comments to be synced.

Learn more at [Syncro](https://docs.syncromsp.com/scripting-apis/).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `syncro-cli` binary and the `pp-syncro` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install syncro
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install syncro --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install syncro --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install syncro --agent claude-code
npx -y @mvanhorn/printing-press-library install syncro --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/syncro/cmd/syncro-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/syncro-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install syncro --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-syncro --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-syncro --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install syncro --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/syncro-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SYNCRO_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/syncro/cmd/syncro-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "syncro": {
      "command": "syncro-mcp",
      "env": {
        "SYNCRO_SUBDOMAIN": "<subdomain>",
        "SYNCRO_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Set SYNCRO_SUBDOMAIN to your account subdomain and SYNCRO_API_KEY to a token created under Admin > API > API Tokens (Custom Permissions). The CLI sends it as an Authorization: Bearer header; if a tenant rejects that, Syncro also accepts the token as an api_key query parameter.

## Quick Start

```bash
# Confirm subdomain, token, and API reachability first.
syncro-cli doctor

# Mirror the tenant into the local SQLite store.
syncro-cli sync --full

# List active tickets straight from the API.
syncro-cli tickets list --status "In Progress" --json

# Triage tickets going stale, computed locally.
syncro-cli tickets aging --no-comment 48h --agent

# Find billable time that was never invoiced.
syncro-cli billing uninvoiced --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Money you are leaving on the table
- **`billing uninvoiced`**  -  Find logged-but-unbilled labor across every customer so no billable time slips.

  _Reach for this to recover revenue: it surfaces money the MSP already earned but never billed._

  ```bash
  syncro-cli billing uninvoiced --agent
  ```
- **`billing drift`**  -  Surface tickets closed long ago that have logged time but were never invoiced.

  _Use after a billing run to catch revenue that silently slips once a ticket closes._

  ```bash
  syncro-cli billing drift --closed-before 14d --agent
  ```
- **`billing ar-aging`**  -  Bucket unpaid invoices into 0-30/30-60/60-90/90+ day aging tiers.

  _The canonical owner finance question  -  pick this to see what cash is aging out._

  ```bash
  syncro-cli billing ar-aging --agent
  ```

### Owner analytics
- **`customers margin`**  -  Compare logged labor against invoiced revenue per customer over a window.

  _Use to find which customers actually make money before renewal conversations._

  ```bash
  syncro-cli customers margin --window 90d --agent
  ```
- **`customers profile`**  -  One-shot cross-entity customer snapshot: open tickets, assets, AR balance, contracts, and latest RMM alerts in a single card.

  _Reach for this when you need full customer context (health, money, risk) before drafting a reply, renewal, or escalation._

  ```bash
  syncro-cli customers profile 12345 --agent
  ```

### Triage that compounds
- **`tickets aging`**  -  List open tickets past SLA with no comment in the last N hours.

  _The daily triage ritual  -  pick this first thing to find tickets going stale._

  ```bash
  syncro-cli tickets aging --no-comment 48h --agent
  ```
- **`snapshot diff`**  -  Diff two retained local sync snapshots across all entities.

  _Use to answer what changed across assets and tickets between two syncs._

  ```bash
  syncro-cli snapshot diff --since 7d --agent
  ```

### Fleet visibility
- **`assets patch-gaps`**  -  Rank assets missing critical patches across every customer.

  _Reach for this for fleet-wide patch posture without N throttled API calls._

  ```bash
  syncro-cli assets patch-gaps --severity critical --agent
  ```
- **`alerts noise`**  -  Rank customers by RMM alert volume over a time window.

  _Use to find the noisy customer draining tech time before it burns out the team._

  ```bash
  syncro-cli alerts noise --window 30d --agent
  ```
- **`alerts orphans`**  -  Surface RMM alerts that never became a ticket within a window.

  _Pick this to catch silent failures and pure noise that slipped past dispatch._

  ```bash
  syncro-cli alerts orphans --window 7d --agent
  ```

## Recipes


### Recover unbilled labor

```bash
syncro-cli billing uninvoiced --agent
```

Lists logged time with no linked invoice grouped by customer (fields: customer_name, uninvoiced_hours, uninvoiced_entries).

### Morning ticket triage

```bash
syncro-cli tickets aging --no-comment 24h --json
```

Open tickets with no comment in 24h, ready to assign or escalate.

### Fleet patch posture

```bash
syncro-cli assets patch-gaps --severity critical --agent --select customer_name,asset_name,missing_patches
```

Cross-customer view of assets with missing/pending patches (fields: customer_name, asset_name, missing_patches).

### Find the noisy customer

```bash
syncro-cli alerts noise --window 30d --json
```

Ranks customers by RMM alert volume over the last month to spot draining accounts.

### Safe customer update

```bash
syncro-cli customers update 123 --business-name "Acme LLC" --dry-run
```

Previews a sparse update that sends only the changed field; use --dry-run to confirm the body before committing.

## Usage

Run `syncro-cli --help` for the full command reference and flag list.

## Commands

### appointment-types

Manage appointment types

- **`syncro-cli appointment-types create`** - Required permission: Global Admin
- **`syncro-cli appointment-types delete`** - Required permission: Global Admin
- **`syncro-cli appointment-types get`** - Retrieves an Appointment Type by ID
- **`syncro-cli appointment-types list`** - Returns a paginated list of Appointment Types
- **`syncro-cli appointment-types update`** - Updates an existing Appointment Type by ID

### appointments

Manage appointments

- **`syncro-cli appointments create`** - No special permissions required.
- **`syncro-cli appointments delete`** - No special permissions required.
- **`syncro-cli appointments get`** - No special permissions required.
- **`syncro-cli appointments list`** - Required permission: Appointments - View All (see-own never restricted)
- **`syncro-cli appointments update`** - Updates an existing Appointment by ID

### callerid

Manage callerid

- **`syncro-cli callerid`** - Get Caller ID

### canned-responses

Manage canned responses

- **`syncro-cli canned-responses create`** - Required permission: Ticket Canned Responses - Manage
- **`syncro-cli canned-responses delete`** - Required permission: Ticket Canned Responses - Manage
- **`syncro-cli canned-responses list`** - Required permission: Ticket Canned Responses - Manage
- **`syncro-cli canned-responses list-cannedresponses`** - Required permission: Ticket Canned Responses - Manage
Single-Customer Users can only access own canned responses.
- **`syncro-cli canned-responses update`** - Required permission: Ticket Canned Responses - Manage

### contacts

Manage contacts

- **`syncro-cli contacts create`** - Required permission: Customers - Edit
Single-Customer Users can only access own contacts.
- **`syncro-cli contacts delete`** - Required permission: Customers - Edit
Single-Customer Users can only access own contacts.
- **`syncro-cli contacts get`** - Required permission: Customers - View Detail
Single-Customer Users can only access own contacts.
- **`syncro-cli contacts list`** - Required permission: Customers - View Detail
Single-Customer Users can only access own contacts.
- **`syncro-cli contacts update`** - Required permission: Customers - Edit
Single-Customer Users can only access own contacts.

### contracts

Manage contracts

- **`syncro-cli contracts create`** - Required permission: Contracts - Edit
- **`syncro-cli contracts delete`** - Required permission: Contracts - Delete
- **`syncro-cli contracts get`** - Required permission: Contracts - Edit
- **`syncro-cli contracts list`** - Required permission: Contracts - List/Search
- **`syncro-cli contracts update`** - Required permission: Contracts - Edit

### customer-assets

Manage customer assets

- **`syncro-cli customer-assets create`** - Required permission: Assets - Create
Single-Customer Users can only access own assets.
- **`syncro-cli customer-assets get`** - Required permission: Assets - View Details
Single-Customer Users can only access own assets.
- **`syncro-cli customer-assets list`** - Required permission: Assets - List/Search
Single-Customer Users can only access own assets.
- **`syncro-cli customer-assets list-customerassets`** - Required permission: Assets - Assets - List/Search.
- **`syncro-cli customer-assets update`** - Required permission: Assets - Edit
Updating only policy_folder_id requires Assets - Policy Change. Updating policy_folder_id with other asset fields requires both Assets - Edit and Assets - Policy Change.
Single-Customer Users can only access own assets.

### customers

Manage customers

- **`syncro-cli customers create`** - Required permission: Customers - Create
- **`syncro-cli customers delete`** - Required permission: Customers - Delete
- **`syncro-cli customers get`** - Required permission: Customers - View Detail
Single-Customer Users can only access own customer (self).
- **`syncro-cli customers list`** - Required permission: Customers - List/Search
Single-Customer Users can only access own customer (self).
- **`syncro-cli customers list-autocomplete`** - Returns a paginated list of customers for autocomplete query
- **`syncro-cli customers list-latest`** - Required permission: Customers - Edit
Single-Customer Users can only access own customer (self).
- **`syncro-cli customers update`** - Required permission: Customers - Edit
Single-Customer Users can only access own customer (self).

### estimates

Manage estimates

- **`syncro-cli estimates create`** - Required permission: Estimates - Create
- **`syncro-cli estimates delete`** - Required permission: Estimates - Delete
- **`syncro-cli estimates get`** - Required permission: Estimates - View Details
- **`syncro-cli estimates list`** - Required permission: Estimates - List/Search
- **`syncro-cli estimates update`** - Required permission: Estimates - Edit

### invoices

Manage invoices

- **`syncro-cli invoices create`** - Required permission: Invoices - Create
- **`syncro-cli invoices delete`** - Returns 200 even if the delete fails
- **`syncro-cli invoices get`** - Required permission: Invoices - View Details
- **`syncro-cli invoices list`** - Required permission: Invoices - List/Search
- **`syncro-cli invoices update`** - This updates an existing Invoice, all parameters overwrite existing params

### items

Manage items

- **`syncro-cli items`** - Required permission: Parts Orders - List/Search

### leads

Manage leads

- **`syncro-cli leads create`** - Required permission: None
- **`syncro-cli leads get`** - Required permission: Leads - List/Search
- **`syncro-cli leads list`** - Required permission: Leads - List/Search
- **`syncro-cli leads update`** - Updates an existing Lead by ID

### line-items

Manage line items

- **`syncro-cli line-items`** - Returns a paginated list of Line Items

### me

Manage me

- **`syncro-cli me`** - Returns the current user

### new-ticket-forms

Manage new ticket forms

- **`syncro-cli new-ticket-forms get`** - Required permission: Tickets - Create
- **`syncro-cli new-ticket-forms list`** - Required permission: Ticket Workflows - Manage

### otp-login

Manage otp login

- **`syncro-cli otp-login`** - Authorize a User with One Time Password

### payment-methods

Manage payment methods

- **`syncro-cli payment-methods`** - All Users except Single Customer Users may use this action.

### payments

Manage payments

- **`syncro-cli payments create`** - Required permission: Payments - Create
- **`syncro-cli payments get`** - Required permission: Payments - View List
- **`syncro-cli payments list`** - Required permission: Payments - View List

### policy-folders

Manage policy folders

- **`syncro-cli policy-folders create`** - Required permission: Policies - Create
Syncro accounts only.
- **`syncro-cli policy-folders delete`** - Required permission: Policies - Delete
Syncro accounts only.
- **`syncro-cli policy-folders get`** - Required permission: Policies - List/Search
Syncro accounts only.
- **`syncro-cli policy-folders list`** - Required permission: Policies - List/Search
Syncro accounts only.
- **`syncro-cli policy-folders update`** - Required permission: Policies - Edit
Syncro accounts only.

### portal-users

Manage portal users

- **`syncro-cli portal-users create`** - Required permission: Global Admin
- **`syncro-cli portal-users create-portalusers`** - Creates an Invitation for a Portal User
- **`syncro-cli portal-users delete`** - Required permission: Global Admin
- **`syncro-cli portal-users list`** - Returns a paginated list of Portal Users
- **`syncro-cli portal-users update`** - Updates an existing Portal User by ID

### products

Manage products

- **`syncro-cli products create`** - Required permission: Products - Create
- **`syncro-cli products get`** - Required permission: Products - List/Search
- **`syncro-cli products list`** - Required permission: Products - List/Search
- **`syncro-cli products list-barcode`** - Required permission: Products - List/Search
- **`syncro-cli products list-categories`** - Returns a paginated list of Product Categories
- **`syncro-cli products update`** - Required permission: Products - Edit

### purchase-orders

Manage purchase orders

- **`syncro-cli purchase-orders create`** - Required permission: Purchase Orders - Edit
- **`syncro-cli purchase-orders get`** - Required permission: Purchase Orders - View Details
- **`syncro-cli purchase-orders list`** - Required permission: Purchase Orders - List/Search

### remote_search

Manage remote search

- **`syncro-cli remote-search`** - Additional permissions required depending on search results type:
- Customer, Contact, Asset: "Customers - List/Search"
- Lead: Leads - List/Search
- Invoice: Invoices - List/Search
- Estimate: Estimates - List/Search
- Ticket: Tickets - List/Search
- Product: Products - List/Search
- Purchase Order, Vendor: Purchase Orders - List/Search
- Report: Reports - View
- Wiki: Documentation - Allow Usage

### rmm-alerts

Manage rmm alerts

- **`syncro-cli rmm-alerts create`** - Required permission: RMM Alerts - Create
Single-Customer Users can only access own RMM Alerts.
- **`syncro-cli rmm-alerts delete`** - Required permission: RMM Alerts - Delete
Single-Customer Users can only access own RMM Alerts.
- **`syncro-cli rmm-alerts get`** - Required permission: RMM Alerts - List
Single-Customer Users can only access own RMM Alerts.
- **`syncro-cli rmm-alerts list`** - Required permission: RMM Alerts - List
Single-Customer Users can only access own RMM Alerts.

### schedules

Manage schedules

- **`syncro-cli schedules create`** - Required permission: Recurring Invoices - New
- **`syncro-cli schedules delete`** - Required permission: Recurring Invoices - Delete
- **`syncro-cli schedules get`** - Required permission: Recurring Invoices - List
- **`syncro-cli schedules list`** - Required permission: Recurring Invoices - List
- **`syncro-cli schedules update`** - Required permission: Recurring Invoices - Edit

### settings

Manage settings

- **`syncro-cli settings list`** - Returns a list of Account Settings
- **`syncro-cli settings list-printing`** - Returns Printing Settings
- **`syncro-cli settings list-tabs`** - Returns Tabs Settings

### ticket-blueprints

Manage ticket blueprints

- **`syncro-cli ticket-blueprints`** - Required permission: Ticket Blueprints - View List
Returns only blueprints available for ticket creation (hidden_for_ticket_creation: false).

### ticket-comments

Manage ticket comments

- **`syncro-cli ticket-comments`** - Required permissions: "Tickets - View Details" or "Tickets - View 'Their Ticket' Details (assigned to them)"

Returns a flat, paginated list of comments across multiple tickets sorted by
ticket_id ASC, created_at DESC. All comments for a given ticket are adjacent
in the response, with the newest comments first within each ticket.

Comments can be scoped to specific tickets using a saved Ticket View
(ticket_search_id) and/or direct ticket filters such as customer_id, status, etc.

Comment-level date filters use the `comment_` prefix to distinguish them from
the ticket-level date filters that share similar names (e.g. created_after).

Single-Customer Users can only access comments for their own tickets.

### ticket-timers

Manage ticket timers

- **`syncro-cli ticket-timers list`** - Required permission: Ticket Timers - Overview
- **`syncro-cli ticket-timers update`** - Update the billable property of a Ticket Timer

### tickets

Manage tickets

- **`syncro-cli tickets create`** - Required permission: Tickets - Create
Single-Customer Users can only access own tickets.
- **`syncro-cli tickets delete`** - Required permission: Tickets - Delete
Single-Customer Users can only access own tickets.
- **`syncro-cli tickets get`** - Required permissions: "Tickets - View Details" or "Tickets - View 'Their Ticket' Details (assigned to them)"
Single-Customer Users can only access own tickets.
- **`syncro-cli tickets list`** - Required permission: Tickets - List/Search
Single-Customer Users can only access own tickets.
- **`syncro-cli tickets list-settings`** - Returns Tickets Settings
- **`syncro-cli tickets update`** - Required permission: Tickets - Edit
Single-Customer Users can only access own tickets.

### timelogs

Manage timelogs

- **`syncro-cli timelogs list`** - Users with permission "Timelogs - Manage" may see timelogs for any/all users.
Otherwise, results scoped to current user.
- **`syncro-cli timelogs list-last`** - Users with permission "Timelogs - Manage" may see timelogs for any/all users.
Otherwise, results scoped to current user.
- **`syncro-cli timelogs update`** - Users with permission "Timelogs - Manage" may see timelogs for any/all users.
Otherwise, results scoped to current user.

### user-devices

Manage user devices

- **`syncro-cli user-devices create`** - Creates a User Device
- **`syncro-cli user-devices get`** - Retrieves an existing User Device by UUID
- **`syncro-cli user-devices update`** - Updates an existing User Device by UUID

### users

Manage users

- **`syncro-cli users get`** - Retrieves an existing User by ID
- **`syncro-cli users list`** - Returns a paginated list of Users

### vendors

Manage vendors

- **`syncro-cli vendors create`** - Required permission: Vendors - New
- **`syncro-cli vendors get`** - Required permission: Vendors - View Details
- **`syncro-cli vendors list`** - Required permission: Vendors - List
- **`syncro-cli vendors update`** - Updates an existing Vendor page by ID

### wiki-pages

Manage wiki pages

- **`syncro-cli wiki-pages create`** - Required permission: Documentation - Create
- **`syncro-cli wiki-pages delete`** - Required permission: Documentation - Delete
- **`syncro-cli wiki-pages get`** - Required permission: Documentation - Allow Usage
- **`syncro-cli wiki-pages list`** - Required permission: Documentation - Allow Usage
- **`syncro-cli wiki-pages update`** - Required permission: Documentation - Edit


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
syncro-cli appointment-types list

# JSON for scripting and agents
syncro-cli appointment-types list --json

# Filter to specific fields
syncro-cli appointment-types list --json --select id,name,status

# Dry run  -  show the request without sending
syncro-cli appointment-types list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
syncro-cli appointment-types list --agent
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

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `SYNCRO_SUBDOMAIN` resolves `{subdomain}`

Base URL: `https://{subdomain}.syncromsp.com/api/v1`

## Health Check

```bash
syncro-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/syncro-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SYNCRO_SUBDOMAIN` | endpoint | Yes |  |
| `SYNCRO_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `syncro-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `syncro-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SYNCRO_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Recreate the token under Admin > API > API Tokens and set SYNCRO_API_KEY; confirm SYNCRO_SUBDOMAIN matches your login URL.
- **429 Too Many Requests**  -  The API caps at 180 requests/minute per IP with no Retry-After; the CLI backs off automatically, but lower concurrency or use the local store via sync for bulk reads.
- **An update blanked out fields I did not touch**  -  Syncro has no PATCH and update sends only the flags you pass (a sparse PUT). If your tenant treats omitted fields as null, pass every field you want to preserve, or GET the record first and re-send its values.
- **Empty results after sync**  -  Run sync --full once to seed the store, then sync uses since_updated_at for incremental updates.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**jrdnr/SyncroMSP**](https://github.com/jrdnr/SyncroMSP)  -  PowerShell (32 stars)
- [**n8n-nodes-syncromsp**](https://github.com/Maelstrom96/n8n-nodes-syncromsp)  -  TypeScript
- [**wm-syncromsp-swagger-client**](https://github.com/watchmanmonitoring/wm-syncromsp-swagger-client)  -  Ruby
- [**syncromsp_api_wrapper**](https://github.com/cabsil22/syncromsp_api_wrapper)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
