---
name: quickbooks
description: "Every QuickBooks Online Accounting entity, plus an offline SQLite mirror, cross-entity search, and AR/AP aging no SDK or read-only MCP ships. Trigger phrases: `who owes us money`, `ar aging`, `what do we owe`, `overdue invoices in quickbooks`, `sync quickbooks`, `quickbooks cash position`, `use quickbooks`, `run quickbooks`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "QuickBooks Online"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - quickbooks-cli
    install:
      - kind: go
        bins: [quickbooks-cli]
        module: github.com/mvanhorn/printing-press-library/library/other/quickbooks/cmd/quickbooks-cli
---

# QuickBooks Online  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `quickbooks-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install quickbooks --cli-only
   ```
2. Verify: `quickbooks-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/other/quickbooks/cmd/quickbooks-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

QuickBooks tools are thin endpoint mirrors that need a live API call for every question and can't answer cross-entity questions at all. This CLI syncs your books to local SQLite once, then answers who's overdue (ar-aging), what you owe (ap-aging), where the cash is (balances), and what's duplicated (dupes) instantly, offline, with agent-native --json/--select output.

## When to Use This CLI

Use this CLI when an agent or finance owner needs answers from a QuickBooks Online company: receivables and payables aging, cash and balance snapshots, collection lists, reconciliation leaks, duplicate cleanup, or any read across the books. It is the right choice when the question spans more than one entity or should be answered offline without burning API calls. For one-off writes, the create/update/delete commands and the raw query passthrough cover the full Accounting surface.

## Unique Capabilities

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

## Command Reference

**accounts**  -  Accounts  -  the chart of accounts

- `quickbooks-cli accounts create`  -  Create an account in the chart of accounts.
- `quickbooks-cli accounts get`  -  Get a single account by Id
- `quickbooks-cli accounts list`  -  List/query accounts
- `quickbooks-cli accounts update`  -  Sparse-update an account (requires Id + SyncToken).

**bills**  -  Bills  -  accounts-payable transactions

- `quickbooks-cli bills create`  -  Create a bill. Requires VendorRef + Line  -  use --stdin / --body-json for the full payload.
- `quickbooks-cli bills delete`  -  Delete a bill (hard delete; requires Id + SyncToken)
- `quickbooks-cli bills get`  -  Get a single bill by Id
- `quickbooks-cli bills list`  -  List/query bills
- `quickbooks-cli bills update`  -  Sparse-update a bill (requires Id + SyncToken).

**changes**  -  Change Data Capture  -  entities changed since a timestamp (powers incremental sync)

- `quickbooks-cli changes`  -  Fetch entities changed since a timestamp (RFC3339), e.g. for incremental sync

**customers**  -  Customers  -  the people and companies you invoice

- `quickbooks-cli customers create`  -  Create a customer. Use --stdin / --body-json for nested fields (email, addresses).
- `quickbooks-cli customers get`  -  Get a single customer by Id
- `quickbooks-cli customers list`  -  List/query customers (SQL-like SELECT against the QBO query endpoint)
- `quickbooks-cli customers update`  -  Sparse-update a customer (requires Id + SyncToken). Use --stdin for full payloads.

**invoices**  -  Invoices  -  accounts-receivable transactions

- `quickbooks-cli invoices create`  -  Create an invoice. Requires Line items + CustomerRef  -  use --stdin / --body-json for the full payload.
- `quickbooks-cli invoices delete`  -  Delete an invoice (hard delete; requires Id + SyncToken)
- `quickbooks-cli invoices get`  -  Get a single invoice by Id
- `quickbooks-cli invoices list`  -  List/query invoices
- `quickbooks-cli invoices update`  -  Sparse-update an invoice (requires Id + SyncToken). Use --stdin for line edits.

**items**  -  Items  -  products and services you sell or buy

- `quickbooks-cli items create`  -  Create an item. Inventory items need account refs  -  use --stdin for those.
- `quickbooks-cli items get`  -  Get a single item by Id
- `quickbooks-cli items list`  -  List/query items
- `quickbooks-cli items update`  -  Sparse-update an item (requires Id + SyncToken).

**journal-entries**  -  Journal entries  -  manual debits and credits

- `quickbooks-cli journal-entries create`  -  Create a journal entry. Requires balanced Line debits/credits  -  use --stdin / --body-json.
- `quickbooks-cli journal-entries delete`  -  Delete a journal entry (hard delete; requires Id + SyncToken)
- `quickbooks-cli journal-entries get`  -  Get a single journal entry by Id
- `quickbooks-cli journal-entries list`  -  List/query journal entries
- `quickbooks-cli journal-entries update`  -  Sparse-update a journal entry (requires Id + SyncToken). Use --stdin for line edits.

**payments**  -  Payments  -  customer payments received against invoices

- `quickbooks-cli payments create`  -  Record a payment. Requires CustomerRef + TotalAmt  -  use --stdin / --body-json for the full payload.
- `quickbooks-cli payments delete`  -  Delete a payment (hard delete; requires Id + SyncToken)
- `quickbooks-cli payments get`  -  Get a single payment by Id
- `quickbooks-cli payments list`  -  List/query payments
- `quickbooks-cli payments update`  -  Sparse-update a payment (requires Id + SyncToken).

**query**  -  Raw QBO query passthrough  -  run any SQL-like SELECT against any entity

- `quickbooks-cli query`  -  Run a raw QBO query, e.g. 'select * from Invoice where Balance > '0' orderby TxnDate'

**vendors**  -  Vendors  -  the people and companies you pay

- `quickbooks-cli vendors create`  -  Create a vendor. Use --stdin / --body-json for nested fields.
- `quickbooks-cli vendors get`  -  Get a single vendor by Id
- `quickbooks-cli vendors list`  -  List/query vendors
- `quickbooks-cli vendors update`  -  Sparse-update a vendor (requires Id + SyncToken).


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
quickbooks-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

QuickBooks Online uses OAuth 2.0 and scopes every call to a company 'realm'. Set QUICKBOOKS_ACCESS_TOKEN to a current bearer access token (from the Intuit OAuth 2.0 Playground or minted from a refresh token), and set the company via QUICKBOOKS_REALM_ID + QUICKBOOKS_ENVIRONMENT (production|sandbox) or the full QUICKBOOKS_BASE_URL. Access tokens last about an hour; auth refresh mints a fresh one from QUICKBOOKS_CLIENT_ID/CLIENT_SECRET/REFRESH_TOKEN so you are not pasting tokens hourly. Required scope: com.intuit.quickbooks.accounting.

Run `quickbooks-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  quickbooks-cli accounts list --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success

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
quickbooks-cli feedback "the --since flag is inclusive but docs say exclusive"
quickbooks-cli feedback --stdin < notes.txt
quickbooks-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/quickbooks-cli/feedback.jsonl`. They are never POSTed unless `QUICKBOOKS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `QUICKBOOKS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
quickbooks-cli profile save briefing --json
quickbooks-cli --profile briefing accounts list
quickbooks-cli profile list --json
quickbooks-cli profile show briefing
quickbooks-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `quickbooks-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/other/quickbooks/cmd/quickbooks-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add quickbooks-mcp -- quickbooks-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which quickbooks-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   quickbooks-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `quickbooks-cli <command> --help`.
