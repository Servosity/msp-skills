# QuickBooks Online CLI

**Every QuickBooks Online entity, plus an offline SQLite mirror, AR/AP aging and close checks.**

QuickBooks tools are thin endpoint mirrors that need a live API call for every question and can't answer cross-entity questions at all. This CLI syncs your books to local SQLite once, then answers who's overdue (ar-aging), what you owe (ap-aging), where the cash is (balances), and what's duplicated (dupes) instantly, offline, with agent-native --json/--select output.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `quickbooks-cli` and `quickbooks-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.ps1 | iex
   ```
3. Verify: `quickbooks-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=quickbooks). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/quickbooks --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/quickbooks --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `quickbooks-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the quickbooks skill from https://github.com/Servosity/msp-skills/tree/main/skills/quickbooks. The skill defines how its required CLI (`quickbooks-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=quickbooks).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `QUICKBOOKS_ACCESS_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

### Remote MCP over HTTP

`quickbooks-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `QUICKBOOKS_MCP_HTTP_TOKEN` is set, and every request must send `Authorization: Bearer <token>`. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`.

```bash
export QUICKBOOKS_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
quickbooks-mcp --transport http
```

`QUICKBOOKS_MCP_READ_ONLY=1` (exactly `1`) lists only read-only tools: it removes `quickbooks_execute`, `sync`, `import`, `workflow_archive`, `aging_delta`, `report` and the learn-store writers (`teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`, `playbook_amend`, `learnings_confirm`, `learnings_reject`, `learnings_forget`), 14 of the 45 tools, and `quickbooks_read` still refuses every state-changing endpoint.

## Authentication

QuickBooks Online uses OAuth 2.0 and scopes every call to a company 'realm'. Set QUICKBOOKS_ACCESS_TOKEN to a current bearer access token (from the Intuit OAuth 2.0 Playground or minted from a refresh token), and set the company via QUICKBOOKS_REALM_ID + QUICKBOOKS_ENVIRONMENT (production|sandbox) or the full QUICKBOOKS_BASE_URL. Access tokens last about an hour. With QUICKBOOKS_CLIENT_ID, QUICKBOOKS_CLIENT_SECRET and a refresh token (QUICKBOOKS_REFRESH_TOKEN, or the one a refresh saved), every command and the MCP server refresh the access token on their own: before a request when it expires within 60 seconds, and once after a 401. QuickBooks rotates the refresh token on refresh; the new one is saved to the credential store before the request is retried, and the stored one is used before QUICKBOOKS_REFRESH_TOKEN. A CLI run and the MCP server refreshing at the same moment make one token call between them. `auth refresh` refreshes on demand. Required scope: com.intuit.quickbooks.accounting.

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
- **`bills due`**  -  Open bills due in the next N days (and any already overdue), sorted by due date with a per-vendor total, so the weekly payment run is one list.

  _Reach for this before the weekly AP payment run: what must be paid this week, and to whom._

  ```bash
  quickbooks-cli bills due --days 7 --agent
  ```
- **`invoices unsent`**  -  Open invoices QuickBooks has neither emailed nor printed (EmailStatus not EmailSent, PrintStatus not PrintComplete), ranked by balance: billed but never delivered.

  _Use at month-end close to catch revenue that was billed but never reached the customer._

  ```bash
  quickbooks-cli invoices unsent --agent
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

### Financial statements
- **`report pnl`**, **`report balance-sheet`**, **`report cash-flow`**  -  Profit & Loss, Balance Sheet and Statement of Cash Flows straight from the QuickBooks Reports API, accrual basis unless `--cash`.

  _Use for month-end and board packs. `--columns month` or `quarter` gives one amount per period; `rows[]` keeps every header, line and total with `kind` (header, account, total), `depth` and `parent`; `--raw` prints QuickBooks' own Header/Columns/Rows JSON._

  ```bash
  quickbooks-cli report cash-flow --start 2026-01-01 --end 2026-03-31 --columns month --agent
  quickbooks-cli report balance-sheet --start 2026-01-01 --as-of 2026-03-31 --columns month --agent
  quickbooks-cli report pnl --month 2026-03 --raw --json
  ```

  `report cash-flow` returns `metrics` with net cash from operating, investing and financing activities, the net cash change, and cash at the beginning and end of the period (each null when the report has no such line), and `metrics_by_column` with the same per column.

## Recipes

### Collections call list, biggest first

```bash
quickbooks-cli invoices stale --days 30 --agent --select id,doc_number,balance,due_date,customer
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

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `QUICKBOOKS_CONFIG_DIR`, `QUICKBOOKS_DATA_DIR`, `QUICKBOOKS_STATE_DIR`, or `QUICKBOOKS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `QUICKBOOKS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export QUICKBOOKS_HOME=/srv/quickbooks
quickbooks-cli doctor
```

On Windows (PowerShell):

```powershell
$env:QUICKBOOKS_HOME = "C:\ProgramData\quickbooks"
quickbooks-cli doctor
```

Under `QUICKBOOKS_HOME=/srv/quickbooks`, the four dirs resolve to `/srv/quickbooks/config`, `/srv/quickbooks/data`, `/srv/quickbooks/state`, and `/srv/quickbooks/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:QUICKBOOKS_HOME = "C:\srv\quickbooks"
quickbooks-cli doctor
```

Under `$env:QUICKBOOKS_HOME = "C:\srv\quickbooks"`, the four dirs resolve to `C:\srv\quickbooks\config`, `C:\srv\quickbooks\data`, `C:\srv\quickbooks\state`, and `C:\srv\quickbooks\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "quickbooks": {
      "command": "quickbooks-mcp",
      "env": {
        "QUICKBOOKS_HOME": "/srv/quickbooks"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `QUICKBOOKS_DATA_DIR` overrides an explicit `--home` for that kind. Use `QUICKBOOKS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `QUICKBOOKS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `quickbooks-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `quickbooks-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `quickbooks-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `QUICKBOOKS_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `QUICKBOOKS_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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


### aging-delta


- **`quickbooks-cli aging-delta`** - See what changed in receivables and payables since the previous sync  -  who slipped an aging bucket and whose balance grew.


### ap-aging


- **`quickbooks-cli ap-aging`** - The payables mirror of AR aging: open bills bucketed by age and rolled up by vendor, so you know what's due and when.


### ar-aging


- **`quickbooks-cli ar-aging`** - See exactly who owes you and how overdue, bucketed 0-30 / 31-60 / 61-90 / 90+ and rolled up by customer.


### balances


- **`quickbooks-cli balances`** - One view of bank/AR/AP account balances plus total customer receivables and total vendor payables.


### bills


- **`quickbooks-cli bills due`** - Open bills due in the next N days (and any already overdue), sorted by due date with a per-vendor total, so the weekly payment run is one list.


### cash-forecast


- **`quickbooks-cli cash-forecast`** - Projected net cash movement by week: expected invoice inflows minus bill outflows bucketed by due date over the next N weeks.


### customer-profitability


- **`quickbooks-cli customer-profitability`** - Per-customer net position: total invoiced minus payments received, open balance, and average days-to-pay, ranked across the whole book.


### customers


- **`quickbooks-cli customers top`** - Rank customers by outstanding balance (or lifetime invoiced) to see concentration and collection focus.


### dso


- **`quickbooks-cli dso`** - Rolling DSO plus per-customer average days-to-pay  -  the headline collections-efficiency KPI.


### dupes


- **`quickbooks-cli dupes customers`** - Fuzzy-match customers (or vendors) by display name and email to catch duplicate records before they fragment AR/AP.


### invoices


- **`quickbooks-cli invoices stale`** - Overdue invoices older than N days, ranked by age times balance so the biggest, oldest debts surface first.

- **`quickbooks-cli invoices unsent`** - Open invoices QuickBooks has neither emailed nor printed (EmailStatus not EmailSent, PrintStatus not PrintComplete), ranked by balance: billed but never delivered.


### journal-entries


- **`quickbooks-cli journal-entries check`** - Flag journal entries whose debit and credit lines don't net to zero or that touch suspense accounts.


### payments


- **`quickbooks-cli payments unapplied`** - Payments with money received but not applied to any invoice  -  a reconciliation leak that quietly distorts AR.


### reconcile


- **`quickbooks-cli reconcile`** - One month-end hygiene sweep: unapplied payments, duplicate names, unbalanced journal entries, and inactive records on open transactions in a single findings list.


### report


- **`quickbooks-cli report pnl`** - Profit & Loss for a period (`--month`, or `--start`/`--end`; `--cash` for cash basis).
- **`quickbooks-cli report balance-sheet`** - Balance Sheet as of `--as-of` (default today); `--start` with `--columns` gives a balance per period end; `--highlight` totals any account or group.
- **`quickbooks-cli report cash-flow`** - Statement of Cash Flows for a period (`--month`, or `--start`/`--end`; `--cash` for cash basis).

All three take `--columns total|month|quarter` and `--raw`, and are read-only MCP tools (`report_pnl`, `report_balance_sheet`, `report_cash_flow`).


### vendors


- **`quickbooks-cli vendors spend`** - Total billed per vendor over a date window, ranked  -  see where the money goes.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`quickbooks-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`quickbooks-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`quickbooks-cli learnings list`** - Inspect taught rows
- **`quickbooks-cli learnings forget <query>`** - Undo a teach
- **`quickbooks-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`quickbooks-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`quickbooks-cli teach-pattern`** - Install a query/resource template up front
- **`quickbooks-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `QUICKBOOKS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `quickbooks-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
quickbooks-cli accounts list

# JSON for scripting and agents
quickbooks-cli accounts list --json
# Filter to specific fields
quickbooks-cli accounts list --json --select Id,SyncToken,Name

# Dry run  -  show the request without sending
quickbooks-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
quickbooks-cli accounts list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
quickbooks-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `quickbooks-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/quickbooks-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\quickbooks-cli\config.toml` on Windows; `--home`, `QUICKBOOKS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `QUICKBOOKS_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `QUICKBOOKS_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

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
- **Token expired after about an hour**  -  Set QUICKBOOKS_CLIENT_ID, QUICKBOOKS_CLIENT_SECRET and QUICKBOOKS_REFRESH_TOKEN once; commands then refresh the access token on their own, and quickbooks-cli auth refresh refreshes on demand. An `invalid_grant` from the refresh means the refresh token expired or was revoked: reconnect the app in Intuit to get a new one.
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
