---
name: syncro
description: "Every Syncro PSA and RMM workflow in your terminal, plus a local database, offline search, and cross-entity reports no other Syncro tool has. Trigger phrases: `list syncro tickets`, `show uninvoiced time in syncro`, `syncro ar aging`, `which syncro assets are missing patches`, `triage stale syncro tickets`, `use syncro`, `run syncro`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Syncro"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - syncro-cli
    install:
      - kind: go
        bins: [syncro-cli]
        module: github.com/mvanhorn/printing-press-library/library/project-management/syncro/cmd/syncro-cli
---

# Syncro  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `syncro-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install syncro --cli-only
   ```
2. Verify: `syncro-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/syncro/cmd/syncro-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

syncro-cli mirrors your Syncro tenant into a local SQLite store so tickets, customers, assets, invoices, and RMM alerts become joinable, searchable, and fast. On top of full API coverage it adds reports Syncro never rendered: uninvoiced time, invoice drift, AR aging, customer profitability, SLA aging triage, and fleet-wide patch gaps. Built for agents with --json, --select, --dry-run, typed exit codes, and adaptive backoff for Syncro's 180-requests-per-minute limit. The cross-entity reports read the local store, so run `sync` first; some (uninvoiced, drift, margin, patch-gaps, aging) also need sub-resources like timer entries, patches, and comments to be synced.

## When to Use This CLI

Choose syncro-cli when an agent or technician needs to query, report on, or update a Syncro tenant from the command line. It is the right tool for cross-entity questions (uninvoiced time, AR aging, patch gaps, alert noise) that Syncro's UI and single-call API cannot answer, and for scripted bulk operations that would otherwise hit the rate limit.

## Unique Capabilities

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

## Command Reference

**appointment-types**  -  Manage appointment types

- `syncro-cli appointment-types create`  -  Required permission: Global Admin
- `syncro-cli appointment-types delete`  -  Required permission: Global Admin
- `syncro-cli appointment-types get`  -  Retrieves an Appointment Type by ID
- `syncro-cli appointment-types list`  -  Returns a paginated list of Appointment Types
- `syncro-cli appointment-types update`  -  Updates an existing Appointment Type by ID

**appointments**  -  Manage appointments

- `syncro-cli appointments create`  -  No special permissions required.
- `syncro-cli appointments delete`  -  No special permissions required.
- `syncro-cli appointments get`  -  No special permissions required.
- `syncro-cli appointments list`  -  Required permission: Appointments - View All (see-own never restricted)
- `syncro-cli appointments update`  -  Updates an existing Appointment by ID

**callerid**  -  Manage callerid

- `syncro-cli callerid`  -  Get Caller ID

**canned-responses**  -  Manage canned responses

- `syncro-cli canned-responses create`  -  Required permission: Ticket Canned Responses - Manage
- `syncro-cli canned-responses delete`  -  Required permission: Ticket Canned Responses - Manage
- `syncro-cli canned-responses list`  -  Required permission: Ticket Canned Responses - Manage
- `syncro-cli canned-responses list-cannedresponses`  -  Required permission: Ticket Canned Responses - Manage Single-Customer Users can only access own canned responses.
- `syncro-cli canned-responses update`  -  Required permission: Ticket Canned Responses - Manage

**contacts**  -  Manage contacts

- `syncro-cli contacts create`  -  Required permission: Customers - Edit Single-Customer Users can only access own contacts.
- `syncro-cli contacts delete`  -  Required permission: Customers - Edit Single-Customer Users can only access own contacts.
- `syncro-cli contacts get`  -  Required permission: Customers - View Detail Single-Customer Users can only access own contacts.
- `syncro-cli contacts list`  -  Required permission: Customers - View Detail Single-Customer Users can only access own contacts.
- `syncro-cli contacts update`  -  Required permission: Customers - Edit Single-Customer Users can only access own contacts.

**contracts**  -  Manage contracts

- `syncro-cli contracts create`  -  Required permission: Contracts - Edit
- `syncro-cli contracts delete`  -  Required permission: Contracts - Delete
- `syncro-cli contracts get`  -  Required permission: Contracts - Edit
- `syncro-cli contracts list`  -  Required permission: Contracts - List/Search
- `syncro-cli contracts update`  -  Required permission: Contracts - Edit

**customer-assets**  -  Manage customer assets

- `syncro-cli customer-assets create`  -  Required permission: Assets - Create Single-Customer Users can only access own assets.
- `syncro-cli customer-assets get`  -  Required permission: Assets - View Details Single-Customer Users can only access own assets.
- `syncro-cli customer-assets list`  -  Required permission: Assets - List/Search Single-Customer Users can only access own assets.
- `syncro-cli customer-assets list-customerassets`  -  Required permission: Assets - Assets - List/Search.
- `syncro-cli customer-assets update`  -  Required permission: Assets - Edit Updating only policy_folder_id requires Assets - Policy Change.

**customers**  -  Manage customers

- `syncro-cli customers create`  -  Required permission: Customers - Create
- `syncro-cli customers delete`  -  Required permission: Customers - Delete
- `syncro-cli customers get`  -  Required permission: Customers - View Detail Single-Customer Users can only access own customer (self).
- `syncro-cli customers list`  -  Required permission: Customers - List/Search Single-Customer Users can only access own customer (self).
- `syncro-cli customers list-autocomplete`  -  Returns a paginated list of customers for autocomplete query
- `syncro-cli customers list-latest`  -  Required permission: Customers - Edit Single-Customer Users can only access own customer (self).
- `syncro-cli customers update`  -  Required permission: Customers - Edit Single-Customer Users can only access own customer (self).

**estimates**  -  Manage estimates

- `syncro-cli estimates create`  -  Required permission: Estimates - Create
- `syncro-cli estimates delete`  -  Required permission: Estimates - Delete
- `syncro-cli estimates get`  -  Required permission: Estimates - View Details
- `syncro-cli estimates list`  -  Required permission: Estimates - List/Search
- `syncro-cli estimates update`  -  Required permission: Estimates - Edit

**invoices**  -  Manage invoices

- `syncro-cli invoices create`  -  Required permission: Invoices - Create
- `syncro-cli invoices delete`  -  Returns 200 even if the delete fails
- `syncro-cli invoices get`  -  Required permission: Invoices - View Details
- `syncro-cli invoices list`  -  Required permission: Invoices - List/Search
- `syncro-cli invoices update`  -  This updates an existing Invoice, all parameters overwrite existing params

**items**  -  Manage items

- `syncro-cli items`  -  Required permission: Parts Orders - List/Search

**leads**  -  Manage leads

- `syncro-cli leads create`  -  Required permission: None
- `syncro-cli leads get`  -  Required permission: Leads - List/Search
- `syncro-cli leads list`  -  Required permission: Leads - List/Search
- `syncro-cli leads update`  -  Updates an existing Lead by ID

**line-items**  -  Manage line items

- `syncro-cli line-items`  -  Returns a paginated list of Line Items

**me**  -  Manage me

- `syncro-cli me`  -  Returns the current user

**new-ticket-forms**  -  Manage new ticket forms

- `syncro-cli new-ticket-forms get`  -  Required permission: Tickets - Create
- `syncro-cli new-ticket-forms list`  -  Required permission: Ticket Workflows - Manage

**otp-login**  -  Manage otp login

- `syncro-cli otp-login`  -  Authorize a User with One Time Password

**payment-methods**  -  Manage payment methods

- `syncro-cli payment-methods`  -  All Users except Single Customer Users may use this action.

**payments**  -  Manage payments

- `syncro-cli payments create`  -  Required permission: Payments - Create
- `syncro-cli payments get`  -  Required permission: Payments - View List
- `syncro-cli payments list`  -  Required permission: Payments - View List

**policy-folders**  -  Manage policy folders

- `syncro-cli policy-folders create`  -  Required permission: Policies - Create Syncro accounts only.
- `syncro-cli policy-folders delete`  -  Required permission: Policies - Delete Syncro accounts only.
- `syncro-cli policy-folders get`  -  Required permission: Policies - List/Search Syncro accounts only.
- `syncro-cli policy-folders list`  -  Required permission: Policies - List/Search Syncro accounts only.
- `syncro-cli policy-folders update`  -  Required permission: Policies - Edit Syncro accounts only.

**portal-users**  -  Manage portal users

- `syncro-cli portal-users create`  -  Required permission: Global Admin
- `syncro-cli portal-users create-portalusers`  -  Creates an Invitation for a Portal User
- `syncro-cli portal-users delete`  -  Required permission: Global Admin
- `syncro-cli portal-users list`  -  Returns a paginated list of Portal Users
- `syncro-cli portal-users update`  -  Updates an existing Portal User by ID

**products**  -  Manage products

- `syncro-cli products create`  -  Required permission: Products - Create
- `syncro-cli products get`  -  Required permission: Products - List/Search
- `syncro-cli products list`  -  Required permission: Products - List/Search
- `syncro-cli products list-barcode`  -  Required permission: Products - List/Search
- `syncro-cli products list-categories`  -  Returns a paginated list of Product Categories
- `syncro-cli products update`  -  Required permission: Products - Edit

**purchase-orders**  -  Manage purchase orders

- `syncro-cli purchase-orders create`  -  Required permission: Purchase Orders - Edit
- `syncro-cli purchase-orders get`  -  Required permission: Purchase Orders - View Details
- `syncro-cli purchase-orders list`  -  Required permission: Purchase Orders - List/Search

**remote_search**  -  Manage remote search

- `syncro-cli remote-search`  -  Additional permissions required depending on search results type: - Customer, Contact, Asset

**rmm-alerts**  -  Manage rmm alerts

- `syncro-cli rmm-alerts create`  -  Required permission: RMM Alerts - Create Single-Customer Users can only access own RMM Alerts.
- `syncro-cli rmm-alerts delete`  -  Required permission: RMM Alerts - Delete Single-Customer Users can only access own RMM Alerts.
- `syncro-cli rmm-alerts get`  -  Required permission: RMM Alerts - List Single-Customer Users can only access own RMM Alerts.
- `syncro-cli rmm-alerts list`  -  Required permission: RMM Alerts - List Single-Customer Users can only access own RMM Alerts.

**schedules**  -  Manage schedules

- `syncro-cli schedules create`  -  Required permission: Recurring Invoices - New
- `syncro-cli schedules delete`  -  Required permission: Recurring Invoices - Delete
- `syncro-cli schedules get`  -  Required permission: Recurring Invoices - List
- `syncro-cli schedules list`  -  Required permission: Recurring Invoices - List
- `syncro-cli schedules update`  -  Required permission: Recurring Invoices - Edit

**settings**  -  Manage settings

- `syncro-cli settings list`  -  Returns a list of Account Settings
- `syncro-cli settings list-printing`  -  Returns Printing Settings
- `syncro-cli settings list-tabs`  -  Returns Tabs Settings

**ticket-blueprints**  -  Manage ticket blueprints

- `syncro-cli ticket-blueprints`  -  Required permission

**ticket-comments**  -  Manage ticket comments

- `syncro-cli ticket-comments`  -  Required permissions: 'Tickets - View Details' or 'Tickets - View 'Their Ticket' Details (assigned to them)

**ticket-timers**  -  Manage ticket timers

- `syncro-cli ticket-timers list`  -  Required permission: Ticket Timers - Overview
- `syncro-cli ticket-timers update`  -  Update the billable property of a Ticket Timer

**tickets**  -  Manage tickets

- `syncro-cli tickets create`  -  Required permission: Tickets - Create Single-Customer Users can only access own tickets.
- `syncro-cli tickets delete`  -  Required permission: Tickets - Delete Single-Customer Users can only access own tickets.
- `syncro-cli tickets get`  -  Required permissions: 'Tickets - View Details' or 'Tickets - View 'Their Ticket' Details (assigned to them)
- `syncro-cli tickets list`  -  Required permission: Tickets - List/Search Single-Customer Users can only access own tickets.
- `syncro-cli tickets list-settings`  -  Returns Tickets Settings
- `syncro-cli tickets update`  -  Required permission: Tickets - Edit Single-Customer Users can only access own tickets.

**timelogs**  -  Manage timelogs

- `syncro-cli timelogs list`  -  Users with permission 'Timelogs - Manage' may see timelogs for any/all users. Otherwise, results scoped to current user.
- `syncro-cli timelogs list-last`  -  Users with permission 'Timelogs - Manage' may see timelogs for any/all users. Otherwise, results scoped to current user.
- `syncro-cli timelogs update`  -  Users with permission 'Timelogs - Manage' may see timelogs for any/all users. Otherwise, results scoped to current user.

**user-devices**  -  Manage user devices

- `syncro-cli user-devices create`  -  Creates a User Device
- `syncro-cli user-devices get`  -  Retrieves an existing User Device by UUID
- `syncro-cli user-devices update`  -  Updates an existing User Device by UUID

**users**  -  Manage users

- `syncro-cli users get`  -  Retrieves an existing User by ID
- `syncro-cli users list`  -  Returns a paginated list of Users

**vendors**  -  Manage vendors

- `syncro-cli vendors create`  -  Required permission: Vendors - New
- `syncro-cli vendors get`  -  Required permission: Vendors - View Details
- `syncro-cli vendors list`  -  Required permission: Vendors - List
- `syncro-cli vendors update`  -  Updates an existing Vendor page by ID

**wiki-pages**  -  Manage wiki pages

- `syncro-cli wiki-pages create`  -  Required permission: Documentation - Create
- `syncro-cli wiki-pages delete`  -  Required permission: Documentation - Delete
- `syncro-cli wiki-pages get`  -  Required permission: Documentation - Allow Usage
- `syncro-cli wiki-pages list`  -  Required permission: Documentation - Allow Usage
- `syncro-cli wiki-pages update`  -  Required permission: Documentation - Edit


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
syncro-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Set SYNCRO_SUBDOMAIN to your account subdomain and SYNCRO_API_KEY to a token created under Admin > API > API Tokens (Custom Permissions). The CLI sends it as an Authorization: Bearer header; if a tenant rejects that, Syncro also accepts the token as an api_key query parameter.

Run `syncro-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  syncro-cli appointment-types list --agent --select id,name,status
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
syncro-cli feedback "the --since flag is inclusive but docs say exclusive"
syncro-cli feedback --stdin < notes.txt
syncro-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/syncro-cli/feedback.jsonl`. They are never POSTed unless `SYNCRO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SYNCRO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
syncro-cli profile save briefing --json
syncro-cli --profile briefing appointment-types list
syncro-cli profile list --json
syncro-cli profile show briefing
syncro-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `syncro-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/project-management/syncro/cmd/syncro-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add syncro-mcp -- syncro-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which syncro-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   syncro-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `syncro-cli <command> --help`.
