# QuickBooks Online CLI

**Every QuickBooks Online Accounting entity, plus an offline SQLite mirror, cross-entity search, and AR/AP aging no SDK or read-only MCP ships.**

QuickBooks tools are thin endpoint mirrors that need a live API call for every question and can't answer cross-entity questions at all. This CLI syncs your books to local SQLite once, then answers who's overdue (ar-aging), what you owe (ap-aging), where the cash is (balances), and what's duplicated (dupes) instantly, offline, with agent-native --json/--select output.

## Install

The recommended path installs both the `quickbooks-cli` binary and the `pp-quickbooks` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install quickbooks
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install quickbooks --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install quickbooks --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install quickbooks --agent claude-code
npx -y @mvanhorn/printing-press-library install quickbooks --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/other/quickbooks/cmd/quickbooks-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/quickbooks-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install quickbooks --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-quickbooks --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-quickbooks --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install quickbooks --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/quickbooks-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `QUICKBOOKS_ACCESS_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/other/quickbooks/cmd/quickbooks-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "quickbooks": {
      "command": "quickbooks-mcp",
      "env": {
        "QUICKBOOKS_ACCESS_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

QuickBooks Online uses OAuth 2.0 and scopes every call to a company 'realm'. Set QUICKBOOKS_ACCESS_TOKEN to a current bearer access token (from the Intuit OAuth 2.0 Playground or minted from a refresh token), and set the company via QUICKBOOKS_REALM_ID + QUICKBOOKS_ENVIRONMENT (production|sandbox) or the full QUICKBOOKS_BASE_URL. Access tokens last about an hour; auth refresh mints a fresh one from QUICKBOOKS_CLIENT_ID/CLIENT_SECRET/REFRESH_TOKEN so you are not pasting tokens hourly. Required scope: com.intuit.quickbooks.accounting.

## Quick Start

```bash
# Confirm the access token and realm reach the API (401 means token/realm not set yet).
quickbooks-cli doctor

# Pull customers, vendors, items, accounts, invoices, payments, bills, and journal entries into local SQLite.
quickbooks-cli sync

# See who owes you, bucketed by age  -  the #1 finance question, answered offline.
quickbooks-cli ar-aging --agent

# Build a collections list: overdue invoices ranked by age times balance.
quickbooks-cli invoices stale --days 60 --agent

# One-box FTS lookup across every synced entity, no API call.
quickbooks-cli search "acme" --agent

# Drop down to the raw QuickBooks query endpoint for anything the typed commands don't cover.
quickbooks-cli query --query "select * from Invoice where Balance > '0' orderby TxnDate" --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Offline financial analytics
- **`ar-aging`**  -  See exactly who owes you and how overdue, bucketed 0-30 / 31-60 / 61-90 / 90+ and rolled up by customer.

  _Reach for this when an agent or user asks 'who owes us money' or 'what's our overdue AR'  -  one offline command instead of paging every invoice live._

  ```bash
  quickbooks-cli ar-aging --agent
  ```
- **`ap-aging`**  -  The payables mirror of AR aging: open bills bucketed by age and rolled up by vendor, so you know what's due and when.

  _Use when the question is 'what do we owe' or 'which vendor bills are overdue' before a payment run._

  ```bash
  quickbooks-cli ap-aging --agent
  ```
- **`balances`**  -  One view of bank/AR/AP account balances plus total customer receivables and total vendor payables.

  _The fastest 'where do we stand financially' snapshot; pick it for a cash-position summary instead of three separate API sweeps._

  ```bash
  quickbooks-cli balances --agent
  ```
- **`invoices stale`**  -  Overdue invoices older than N days, ranked by age times balance so the biggest, oldest debts surface first.

  _Use to build a weekly collections call list  -  highest-leverage receivables first._

  ```bash
  quickbooks-cli invoices stale --days 60 --agent
  ```
- **`payments unapplied`**  -  Payments with money received but not applied to any invoice  -  a reconciliation leak that quietly distorts AR.

  _Reach for this during month-end reconcile to catch cash that never got applied._

  ```bash
  quickbooks-cli payments unapplied --agent
  ```
- **`customers top`**  -  Rank customers by outstanding balance (or lifetime invoiced) to see concentration and collection focus.

  _Use to answer 'who are our biggest receivables' or 'where is AR concentrated'._

  ```bash
  quickbooks-cli customers top --by balance --limit 10 --agent
  ```
- **`vendors spend`**  -  Total billed per vendor over a date window, ranked  -  see where the money goes.

  _Use for spend review or vendor negotiation prep  -  who are our biggest payees this year._

  ```bash
  quickbooks-cli vendors spend --since 2026-01-01 --agent
  ```
- **`aging-delta`**  -  See what changed in receivables and payables since the previous sync  -  who slipped an aging bucket and whose balance grew.

  _Reach for this when asked 'what changed in AR/AP since last week'  -  no live API call can answer it._

  ```bash
  quickbooks-cli aging-delta --agent
  ```
- **`customer-profitability`**  -  Per-customer net position: total invoiced minus payments received, open balance, and average days-to-pay, ranked across the whole book.

  _Use when an agent is asked which customers pay slowest or are worth the relationship  -  net position and days-to-pay in one view._

  ```bash
  quickbooks-cli customer-profitability --agent
  ```
- **`dso`**  -  Rolling DSO plus per-customer average days-to-pay  -  the headline collections-efficiency KPI.

  _Reach for this when asked 'how fast are we collecting'  -  the KPI finance owners track weekly._

  ```bash
  quickbooks-cli dso --days 90 --agent
  ```
- **`cash-forecast`**  -  Projected net cash movement by week: expected invoice inflows minus bill outflows bucketed by due date over the next N weeks.

  _Use when asked 'is it safe to pay this' or 'where is cash heading'  -  scheduled inflows vs outflows at a glance._

  ```bash
  quickbooks-cli cash-forecast --weeks 4 --agent
  ```

### Data hygiene
- **`dupes customers`**  -  Fuzzy-match customers (or vendors) by display name and email to catch duplicate records before they fragment AR/AP.

  _Run during data cleanup to find 'Acme Inc' vs 'Acme, Inc.' before merging._

  ```bash
  quickbooks-cli dupes customers --agent
  ```

### Close & reconciliation hygiene
- **`reconcile`**  -  One month-end hygiene sweep: unapplied payments, duplicate names, unbalanced journal entries, and inactive records on open transactions in a single findings list.

  _Use at close: one command answers 'what is leaking' across every hygiene check._

  ```bash
  quickbooks-cli reconcile --agent
  ```
- **`journal-entries check`**  -  Flag journal entries whose debit and credit lines don't net to zero or that touch suspense accounts.

  _Reach for this during close when the GL won't tie out  -  finds the unbalanced entry in seconds._

  ```bash
  quickbooks-cli journal-entries check --agent
  ```

## Recipes


### Collections call list, biggest first

```bash
quickbooks-cli invoices stale --days 30 --agent --select Id,DocNumber,Balance,DueDate,CustomerRef.name
```

Overdue invoices with just the fields a collector needs, including the nested customer name, ranked by age times balance.

### Receivables aging into a dashboard

```bash
quickbooks-cli ar-aging --json | jq '.buckets'
```

Structured aging buckets you can pipe straight into a report or alert.

### Find a record across the whole book

```bash
quickbooks-cli search "acme" --agent
```

FTS5 lookup across customers, vendors, items, invoices, and more  -  offline, one box.

### Raw query for anything bespoke

```bash
quickbooks-cli query --query "select * from Bill where Balance > '0' orderby DueDate" --agent --select Id,DocNumber,Balance,VendorRef.name
```

Drop to the QuickBooks query endpoint and narrow deeply nested output with --select.

### Create an invoice (flags, or pipe full JSON)

```bash
quickbooks-cli invoices create --doc-number INV-1001 --txn-date 2026-05-01 --due-date 2026-05-31
```

Create from scalar flags for simple cases; for full payloads with Line items and CustomerRef, pipe JSON: cat invoice.json | quickbooks-cli invoices create --stdin.

## Usage

Run `quickbooks-cli --help` for the full command reference and flag list.

## Commands

### accounts

Accounts  -  the chart of accounts

- **`quickbooks-cli accounts create`** - Create an account in the chart of accounts.
- **`quickbooks-cli accounts get`** - Get a single account by Id
- **`quickbooks-cli accounts list`** - List/query accounts
- **`quickbooks-cli accounts update`** - Sparse-update an account (requires Id + SyncToken).

### bills

Bills  -  accounts-payable transactions

- **`quickbooks-cli bills create`** - Create a bill. Requires VendorRef + Line  -  use --stdin / --body-json for the full payload.
- **`quickbooks-cli bills delete`** - Delete a bill (hard delete; requires Id + SyncToken)
- **`quickbooks-cli bills get`** - Get a single bill by Id
- **`quickbooks-cli bills list`** - List/query bills
- **`quickbooks-cli bills update`** - Sparse-update a bill (requires Id + SyncToken).

### changes

Change Data Capture  -  entities changed since a timestamp (powers incremental sync)

- **`quickbooks-cli changes`** - Fetch entities changed since a timestamp (RFC3339), e.g. for incremental sync

### customers

Customers  -  the people and companies you invoice

- **`quickbooks-cli customers create`** - Create a customer. Use --stdin / --body-json for nested fields (email, addresses).
- **`quickbooks-cli customers get`** - Get a single customer by Id
- **`quickbooks-cli customers list`** - List/query customers (SQL-like SELECT against the QBO query endpoint)
- **`quickbooks-cli customers update`** - Sparse-update a customer (requires Id + SyncToken). Use --stdin for full payloads.

### invoices

Invoices  -  accounts-receivable transactions

- **`quickbooks-cli invoices create`** - Create an invoice. Requires Line items + CustomerRef  -  use --stdin / --body-json for the full payload.
- **`quickbooks-cli invoices delete`** - Delete an invoice (hard delete; requires Id + SyncToken)
- **`quickbooks-cli invoices get`** - Get a single invoice by Id
- **`quickbooks-cli invoices list`** - List/query invoices
- **`quickbooks-cli invoices update`** - Sparse-update an invoice (requires Id + SyncToken). Use --stdin for line edits.

### items

Items  -  products and services you sell or buy

- **`quickbooks-cli items create`** - Create an item. Inventory items need account refs  -  use --stdin for those.
- **`quickbooks-cli items get`** - Get a single item by Id
- **`quickbooks-cli items list`** - List/query items
- **`quickbooks-cli items update`** - Sparse-update an item (requires Id + SyncToken).

### journal-entries

Journal entries  -  manual debits and credits

- **`quickbooks-cli journal-entries create`** - Create a journal entry. Requires balanced Line debits/credits  -  use --stdin / --body-json.
- **`quickbooks-cli journal-entries delete`** - Delete a journal entry (hard delete; requires Id + SyncToken)
- **`quickbooks-cli journal-entries get`** - Get a single journal entry by Id
- **`quickbooks-cli journal-entries list`** - List/query journal entries
- **`quickbooks-cli journal-entries update`** - Sparse-update a journal entry (requires Id + SyncToken). Use --stdin for line edits.

### payments

Payments  -  customer payments received against invoices

- **`quickbooks-cli payments create`** - Record a payment. Requires CustomerRef + TotalAmt  -  use --stdin / --body-json for the full payload.
- **`quickbooks-cli payments delete`** - Delete a payment (hard delete; requires Id + SyncToken)
- **`quickbooks-cli payments get`** - Get a single payment by Id
- **`quickbooks-cli payments list`** - List/query payments
- **`quickbooks-cli payments update`** - Sparse-update a payment (requires Id + SyncToken).

### query

Raw QBO query passthrough  -  run any SQL-like SELECT against any entity

- **`quickbooks-cli query`** - Run a raw QBO query, e.g. "select * from Invoice where Balance > '0' orderby TxnDate"

### vendors

Vendors  -  the people and companies you pay

- **`quickbooks-cli vendors create`** - Create a vendor. Use --stdin / --body-json for nested fields.
- **`quickbooks-cli vendors get`** - Get a single vendor by Id
- **`quickbooks-cli vendors list`** - List/query vendors
- **`quickbooks-cli vendors update`** - Sparse-update a vendor (requires Id + SyncToken).


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
quickbooks-cli accounts list

# JSON for scripting and agents
quickbooks-cli accounts list --json

# Filter to specific fields
quickbooks-cli accounts list --json --select id,name,status

# Dry run  -  show the request without sending
quickbooks-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
quickbooks-cli accounts list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
quickbooks-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/quickbooks-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `QUICKBOOKS_ACCESS_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `quickbooks-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `quickbooks-cli doctor` to check credentials
- Verify the environment variable is set: `echo $QUICKBOOKS_ACCESS_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every command**  -  Set QUICKBOOKS_ACCESS_TOKEN to a current bearer token and QUICKBOOKS_REALM_ID to your company id (or QUICKBOOKS_BASE_URL to the full .../v3/company/<realmId> URL).
- **Token expired after about an hour**  -  Run quickbooks-cli auth refresh (needs QUICKBOOKS_CLIENT_ID/CLIENT_SECRET/REFRESH_TOKEN) to mint a fresh access token.
- **Responses come back as XML**  -  The CLI sends Accept: application/json automatically; if you override headers, keep that header so QuickBooks returns JSON.
- **Hit a rate limit (HTTP 429)**  -  Production allows about 500 requests/minute. Run sync once and query the local store for analytics instead of re-hitting the API.
- **Sandbox vs production confusion**  -  Set QUICKBOOKS_ENVIRONMENT=sandbox to target sandbox-quickbooks.api.intuit.com; default is production.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**quickbooks-online-mcp-server**](https://github.com/intuit/quickbooks-online-mcp-server)  -  TypeScript
- [**quickbooks-online-mcp**](https://github.com/LibreChat-AI/quickbooks-online-mcp)  -  TypeScript
- [**quickbooks-mcp-server**](https://github.com/nikhilgy/quickbooks-mcp-server)  -  Python
- [**python-quickbooks**](https://github.com/ej2/python-quickbooks)  -  Python
- [**node-quickbooks**](https://github.com/mcohen01/node-quickbooks)  -  JavaScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
