---
name: quickbooks
description: "Every QuickBooks Online entity, plus an offline SQLite mirror, AR/AP aging and close checks. Trigger phrases: `who owes us money`, `ar aging`, `what do we owe`, `overdue invoices in quickbooks`, `sync quickbooks`, `quickbooks cash position`, `use quickbooks`, `run quickbooks`."
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
      - kind: script
        bins: [quickbooks-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.ps1
---

# QuickBooks Online - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `quickbooks-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

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

The installer downloads the `quickbooks-cli` and `quickbooks-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

QuickBooks tools are thin endpoint mirrors that need a live API call for every question and can't answer cross-entity questions at all. This CLI syncs your books to local SQLite once, then answers who's overdue (ar-aging), what you owe (ap-aging), where the cash is (balances), and what's duplicated (dupes) instantly, offline, with agent-native --json/--select output.

## When to Use This CLI

Use this CLI when an agent or finance owner needs answers from a QuickBooks Online company: receivables and payables aging, cash and balance snapshots, collection lists, reconciliation leaks, duplicate cleanup, or any read across the books. It is the right choice when the question spans more than one entity or should be answered offline without burning API calls. For one-off writes, the create/update/delete commands and the raw query passthrough cover the full Accounting surface.

## Unique Capabilities

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


**aging-delta**


- `quickbooks-cli aging-delta`  -  See what changed in receivables and payables since the previous sync  -  who slipped an aging bucket and whose balance grew.


**ap-aging**


- `quickbooks-cli ap-aging`  -  The payables mirror of AR aging: open bills bucketed by age and rolled up by vendor, so you know what's due and when.


**ar-aging**


- `quickbooks-cli ar-aging`  -  See exactly who owes you and how overdue, bucketed 0-30 / 31-60 / 61-90 / 90+ and rolled up by customer.


**balances**


- `quickbooks-cli balances`  -  One view of bank/AR/AP account balances plus total customer receivables and total vendor payables.


**bills**


- `quickbooks-cli bills due`  -  Open bills due in the next N days (and any already overdue), sorted by due date with a per-vendor total, so the weekly payment run is one list.


**cash-forecast**


- `quickbooks-cli cash-forecast`  -  Projected net cash movement by week: expected invoice inflows minus bill outflows bucketed by due date over the next N weeks.


**customer-profitability**


- `quickbooks-cli customer-profitability`  -  Per-customer net position: total invoiced minus payments received, open balance, and average days-to-pay, ranked across the whole book.


**customers**


- `quickbooks-cli customers top`  -  Rank customers by outstanding balance (or lifetime invoiced) to see concentration and collection focus.


**dso**


- `quickbooks-cli dso`  -  Rolling DSO plus per-customer average days-to-pay  -  the headline collections-efficiency KPI.


**dupes**


- `quickbooks-cli dupes customers`  -  Fuzzy-match customers (or vendors) by display name and email to catch duplicate records before they fragment AR/AP.


**invoices**


- `quickbooks-cli invoices stale`  -  Overdue invoices older than N days, ranked by age times balance so the biggest, oldest debts surface first.

- `quickbooks-cli invoices unsent`  -  Open invoices QuickBooks has neither emailed nor printed (EmailStatus not EmailSent, PrintStatus not PrintComplete), ranked by balance: billed but never delivered.


**journal-entries**


- `quickbooks-cli journal-entries check`  -  Flag journal entries whose debit and credit lines don't net to zero or that touch suspense accounts.


**payments**


- `quickbooks-cli payments unapplied`  -  Payments with money received but not applied to any invoice  -  a reconciliation leak that quietly distorts AR.


**reconcile**


- `quickbooks-cli reconcile`  -  One month-end hygiene sweep: unapplied payments, duplicate names, unbalanced journal entries, and inactive records on open transactions in a single findings list.


**report**


- `quickbooks-cli report pnl`  -  Profit & Loss from the Reports API: revenue by account, COGS, gross and operating margin, net income.

- `quickbooks-cli report balance-sheet`  -  Balance Sheet as of a date (or one column per period end with `--start` and `--columns`), with `--highlight` totals.

- `quickbooks-cli report cash-flow`  -  Statement of Cash Flows: net cash from operating, investing and financing activities, net cash change, beginning and ending cash.

Every report takes `--columns total|month|quarter` (one amount per period in `columns[]` and each row's `amounts`), returns `rows[]` (every header, line and total in QuickBooks' order with `kind` header|account|total, `depth` and `parent`), and `--raw` prints QuickBooks' own Header/Columns/Rows JSON. All three are read-only MCP tools.


**vendors**


- `quickbooks-cli vendors spend`  -  Total billed per vendor over a date window, ranked  -  see where the money goes.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
quickbooks-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

### A quarter's statements, by month

```bash
quickbooks-cli report pnl --start 2026-01-01 --end 2026-03-31 --columns month --agent
quickbooks-cli report balance-sheet --start 2026-01-01 --as-of 2026-03-31 --columns month --agent
quickbooks-cli report cash-flow --start 2026-01-01 --end 2026-03-31 --columns month --agent
```

Each row carries `kind`, `depth`, `parent` and one amount per month, so the statements can be laid out the way QuickBooks shows them. `report cash-flow` returns `metrics` (operating, investing and financing activities, net cash change, beginning and ending cash; null when the report has no such line) and `metrics_by_column`. Add `--raw` for QuickBooks' own JSON.

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

QuickBooks Online uses OAuth 2.0 and scopes every call to a company 'realm'. Set QUICKBOOKS_ACCESS_TOKEN to a current bearer access token (from the Intuit OAuth 2.0 Playground or minted from a refresh token), and set the company via QUICKBOOKS_REALM_ID + QUICKBOOKS_ENVIRONMENT (production|sandbox) or the full QUICKBOOKS_BASE_URL. Access tokens last about an hour. With QUICKBOOKS_CLIENT_ID, QUICKBOOKS_CLIENT_SECRET and a refresh token (QUICKBOOKS_REFRESH_TOKEN, or the one a refresh saved), every command and the MCP server refresh the access token on their own: before a request when it expires within 60 seconds, and once after a 401. QuickBooks rotates the refresh token on refresh; the new one is saved to the credential store before the request is retried, and the stored one is used before QUICKBOOKS_REFRESH_TOKEN. A CLI run and the MCP server refreshing at the same moment make one token call between them. `auth refresh` refreshes on demand. Required scope: com.intuit.quickbooks.accounting.

Run `quickbooks-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  quickbooks-cli accounts list --agent --select Id,SyncToken,Name
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
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

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `QUICKBOOKS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `QUICKBOOKS_CONFIG_DIR`, `QUICKBOOKS_DATA_DIR`, `QUICKBOOKS_STATE_DIR`, `QUICKBOOKS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `QUICKBOOKS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `quickbooks-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `QUICKBOOKS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `QUICKBOOKS_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `QUICKBOOKS_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `QUICKBOOKS_DATA_SOURCE` sets the `--data-source` default.

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
quickbooks-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "quickbooks-cli learnings confirm 12"] }
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
       materially more, record the divergence via `quickbooks-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `quickbooks-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `quickbooks-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
quickbooks-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
quickbooks-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
quickbooks-cli teach-playbook \
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
quickbooks-cli playbook amend \
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

`quickbooks-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `QUICKBOOKS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
quickbooks-cli feedback "the --since flag is inclusive but docs say exclusive"
quickbooks-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `QUICKBOOKS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `QUICKBOOKS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

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
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `quickbooks-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add quickbooks-mcp -- quickbooks-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `quickbooks-mcp --transport http` requires `QUICKBOOKS_MCP_HTTP_TOKEN` (callers send `Authorization: Bearer <token>`), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. `QUICKBOOKS_MCP_READ_ONLY=1` (exactly `1`) lists only read-only tools: it removes `quickbooks_execute`, `sync`, `import`, `workflow_archive`, `aging_delta`, `report` and the learn-store writers (`teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`, `playbook_amend`, `learnings_confirm`, `learnings_reject`, `learnings_forget`), 14 of the 45 tools, and `quickbooks_read` still refuses every state-changing endpoint.

## Direct Use

1. Check if installed: `which quickbooks-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   quickbooks-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `quickbooks-cli <command> --help`.
