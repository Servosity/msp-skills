---
name: xero
description: "Every Xero Accounting read plus a local SQLite ledger  -  aging, reconciliation, and GL tie-out that no other Xero tool computes offline. Trigger phrases: `xero aging report`, `who owes us money in xero`, `reconcile xero payments`, `tie out the general ledger`, `what changed in xero since`, `use xero`, `run xero-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Xero"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - xero-cli
    install:
      - kind: go
        bins: [xero-cli]
        module: github.com/mvanhorn/printing-press-library/library/payments/xero/cmd/xero-cli
---

# Xero  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `xero-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install xero --cli-only
   ```
2. Verify: `xero-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/payments/xero/cmd/xero-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A Go CLI for the Xero Accounting API across invoices, contacts, accounts, payments, bank transactions, items, and the immutable journals feed. It syncs the org into a queryable local store, then answers the questions the web UI, SDKs, and endpoint-mirroring MCP servers cannot in a single call: AR/AP aging (`aging`), payment and bank reconciliation gaps (`reconcile`, `bank-recon`), general-ledger-to-invoice tie-out (`tie-out`), and a running-balance ledger walk (`ledger`).

## When to Use This CLI

Use this CLI when an agent or operator needs to read and analyze a Xero organisation's accounting state from the terminal: pulling invoices/contacts/accounts/payments/bank-transactions/items/journals, or answering portfolio questions like AR aging, payment reconciliation gaps, and general-ledger tie-out. It is the right tool when the question spans more than one entity or should run offline against a synced store rather than re-hitting the rate-limited API.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Receivables & exposure
- **`aging`**  -  Bucket outstanding invoices into current / 1-30 / 31-60 / 61-90 / 90+ days overdue with totals per bucket, computed locally from your synced ledger.

  _Reach for this to answer 'who owes us and how overdue' in one offline call instead of paging the web UI per org._

  ```bash
  xero-cli aging --json
  ```
- **`exposure`**  -  Rank contacts by total outstanding amount due across their authorised invoices, with an overdue split and invoice count; drill into one contact for a running-balance statement.

  _Use to find concentration of receivable risk  -  who owes the most and how overdue  -  before a collections push._

  ```bash
  xero-cli exposure --json
  ```

### Reconciliation
- **`reconcile`**  -  Cross-join invoices and payments locally to surface AUTHORISED invoices with an amount still due and no (or partial) applied payment, plus payments not fully applied.

  _Pick this when you need the gap between the ledger and applied cash, not a raw list of either._

  ```bash
  xero-cli reconcile --json
  ```
- **`bank-recon`**  -  List bank transactions still flagged unreconciled and match them to invoices and payments by contact and exact amount to suggest likely reconciliation matches.

  _Use before a reconciliation session to see unreconciled spend and its probable invoice/payment matches up front._

  ```bash
  xero-cli bank-recon --json
  ```

### Ledger audit & tie-out
- **`tie-out`**  -  Sum the immutable journal lines by account code and compare the receivable/payable control-account totals against the sum of outstanding invoice amounts, reporting the variance.

  _Reach for this at period close to prove the books tie before signing off  -  variance of zero is the closure signal._

  ```bash
  xero-cli tie-out --json
  ```
- **`ledger`**  -  Replay the immutable journals feed for one account code as an ordered running-balance statement (date, journal number, debit, credit, running balance), entirely offline.

  _Use to audit exactly what posted to a single account without paging the read-only Journals view in the web UI._

  ```bash
  xero-cli ledger 200 --json
  ```

### Agent-native plumbing
- **`since`**  -  List records across every synced entity whose last-modified timestamp is newer than a given time, read entirely from the local store's sync cursor with zero API calls.

  _Pick this when an agent needs the org delta cheaply instead of re-pulling everything against the rate limit._

  ```bash
  xero-cli since 2026-05-01 --json
  ```
- **`snapshot`**  -  One offline call returning total receivable and payable outstanding, overdue count, unreconciled bank-transaction count, and per-table sync staleness as a single structured object.

  _Reach for this as an agent's single state-of-the-org call instead of fanning out across multiple list endpoints._

  ```bash
  xero-cli snapshot --json
  ```

## Command Reference

**accounts**  -  Manage accounts

- `xero-cli accounts create`  -  Creates a new chart of accounts
- `xero-cli accounts delete`  -  Deletes a chart of accounts
- `xero-cli accounts get`  -  Retrieves the full chart of accounts
- `xero-cli accounts get-accountid`  -  Retrieves a single chart of accounts by using a unique account Id
- `xero-cli accounts update`  -  Updates a chart of accounts

**bank-transactions**  -  Manage bank transactions

- `xero-cli bank-transactions create`  -  Creates one or more spent or received money transaction
- `xero-cli bank-transactions get`  -  Retrieves any spent or received money transactions
- `xero-cli bank-transactions get-banktransactions`  -  Retrieves a single spent or received money transaction by using a unique bank transaction Id
- `xero-cli bank-transactions update`  -  Updates a single spent or received money transaction
- `xero-cli bank-transactions update-or-create`  -  Updates or creates one or more spent or received money transaction

**contacts**  -  Manage contacts

- `xero-cli contacts create`  -  Creates multiple contacts (bulk) in a Xero organisation
- `xero-cli contacts get`  -  Retrieves all contacts in a Xero organisation
- `xero-cli contacts get-by-number`  -  Retrieves a specific contact by contact number in a Xero organisation
- `xero-cli contacts get-contactid`  -  Retrieves a specific contacts in a Xero organisation using a unique contact Id
- `xero-cli contacts update`  -  Updates a specific contact in a Xero organisation
- `xero-cli contacts update-or-create`  -  Updates or creates one or more contacts in a Xero organisation

**invoices**  -  Manage invoices

- `xero-cli invoices create`  -  Creates one or more sales invoices or purchase bills
- `xero-cli invoices get`  -  Retrieves sales invoices or purchase bills
- `xero-cli invoices get-invoiceid`  -  Retrieves a specific sales invoice or purchase bill using a unique invoice Id
- `xero-cli invoices update`  -  Updates a specific sales invoices or purchase bills
- `xero-cli invoices update-or-create`  -  Updates or creates one or more sales invoices or purchase bills

**items**  -  Manage items

- `xero-cli items create`  -  Creates one or more items
- `xero-cli items delete`  -  Deletes a specific item
- `xero-cli items get`  -  Retrieves items
- `xero-cli items get-itemid`  -  Retrieves a specific item using a unique item Id
- `xero-cli items update`  -  Updates a specific item
- `xero-cli items update-or-create`  -  Updates or creates one or more items

**journals**  -  Manage journals

- `xero-cli journals get`  -  Retrieves journals
- `xero-cli journals get-by-number`  -  Retrieves a specific journal using a unique journal number.
- `xero-cli journals get-journalid`  -  Retrieves a specific journal using a unique journal Id.

**payments**  -  Manage payments

- `xero-cli payments create`  -  Creates a single payment for invoice or credit notes
- `xero-cli payments create-endpoint`  -  Creates multiple payments for invoices or credit notes
- `xero-cli payments delete`  -  Updates a specific payment for invoices and credit notes
- `xero-cli payments get`  -  Retrieves payments for invoices and credit notes
- `xero-cli payments get-paymentid`  -  Retrieves a specific payment for invoices and credit notes using a unique payment Id


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
xero-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Monday AR sweep

```bash
xero-cli aging --json
```

Buckets every outstanding invoice by days overdue so you can build a chase list without exporting and pivoting in a spreadsheet.

### Narrow a verbose invoice list for an agent

```bash
xero-cli invoices get --agent --select Invoices.InvoiceNumber,Invoices.Contact.Name,Invoices.AmountDue,Invoices.Status
```

Xero invoice payloads are large and deeply nested; dotted-path --select returns only the fields an agent needs, keeping context small.

### Find the cash-application gap

```bash
xero-cli reconcile --json
```

Cross-joins invoices and payments locally to show authorised invoices with an amount still due and no applied payment.

### Prove the books tie at close

```bash
xero-cli tie-out --json
```

Compares the immutable general-ledger control accounts against the outstanding invoice totals and reports the variance.

### Audit one account's postings

```bash
xero-cli ledger 200 --json
```

Replays the journal feed for account code 200 as a running-balance statement from the local store.

## Auth Setup

Xero requires two credentials on every call: an OAuth2 Bearer access token and a required Xero-Tenant-Id header. Set XERO_ACCESS_TOKEN (obtained via the OAuth2 authorization-code flow or a Xero Custom Connection client-credentials grant) and XERO_TENANT_ID (the target organisation's tenant id from GET /connections). Run `doctor` to confirm both are present before syncing.

Run `xero-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  xero-cli accounts get --agent --select id,name,status
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
xero-cli feedback "the --since flag is inclusive but docs say exclusive"
xero-cli feedback --stdin < notes.txt
xero-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/xero-cli/feedback.jsonl`. They are never POSTed unless `XERO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `XERO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
xero-cli profile save briefing --json
xero-cli --profile briefing accounts get
xero-cli profile list --json
xero-cli profile show briefing
xero-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `xero-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/payments/xero/cmd/xero-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add xero-mcp -- xero-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which xero-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   xero-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `xero-cli <command> --help`.
