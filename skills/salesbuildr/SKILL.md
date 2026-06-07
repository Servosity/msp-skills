---
name: salesbuildr
description: "Every Salesbuildr resource as a scriptable command, plus offline margin and pipeline analytics. Trigger phrases: `salesbuildr stale quotes`, `check quote margins`, `salesbuildr pipeline velocity`, `sync salesbuildr catalog`, `company whitespace salesbuildr`, `use salesbuildr`, `run salesbuildr`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Salesbuildr"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - salesbuildr-cli
    install:
      - kind: go
        bins: [salesbuildr-cli]
        module: github.com/mvanhorn/printing-press-library/library/sales-and-crm/salesbuildr/cmd/salesbuildr-cli
---

# Salesbuildr  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `salesbuildr-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install salesbuildr --cli-only
   ```
2. Verify: `salesbuildr-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/salesbuildr/cmd/salesbuildr-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

salesbuildr-cli mirrors the full Salesbuildr Public API  -  companies, contacts, products, opportunities, quotes, pricing books, templates and more  -  with agent-native JSON output and a local SQLite store. The local store is what unlocks the commands no other Salesbuildr tool has: stale-quote radar, low-margin line detection, pricing drift, pipeline velocity, win-rate, MRR forecast, and company whitespace.

## When to Use This CLI

Choose salesbuildr-cli when an agent or script needs to read or mutate Salesbuildr data non-interactively, or when you need margin, quote-lifecycle, or pipeline analytics that the web UI and the Public API do not expose. It is the right tool for batch catalog updates by external identifier, PSA-id sync workflows, and weekly pipeline/margin reviews.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local analytics that compound
- **`quote stale`**  -  Surface quotes that have been sent or approved but are aging past a cutoff, with the dollar value still at risk.

  _Reach for this when you need the one list the web UI never shows: which open quotes are slipping and how much revenue is on the line._

  ```bash
  salesbuildr-cli quote stale --days 14 --agent
  ```
- **`product velocity`**  -  Rank catalog products by how often they appear across quote line items and the total value quoted  -  your de-facto bestsellers.

  _Reach for this for most-quoted products by frequency and value; to find products a company has never been quoted, use 'company whitespace' instead._

  ```bash
  salesbuildr-cli product velocity --limit 20 --agent
  ```
- **`reconcile-psa`**  -  Find rows across companies, contacts, products, and opportunities whose external identifier is missing  -  the records that silently miss a PSA sync.

  _Reach for this before an Autotask/ConnectWise/HaloPSA sync to catch records that will be skipped for lack of an external id._

  ```bash
  salesbuildr-cli reconcile-psa --agent
  ```
- **`company whitespace`**  -  Show catalog products a company has never been quoted  -  the cross-sell gaps.

  _Use this before an account review to find concrete cross-sell opportunities grounded in what the company has not bought._

  ```bash
  salesbuildr-cli company whitespace 1234 --agent
  ```

### Margin intelligence
- **`quote thin`**  -  Find every quote line item across all open quotes whose markup falls below a floor.

  _Use this before quotes go out to catch under-priced lines that silently erode margin when distributor costs move._

  ```bash
  salesbuildr-cli quote thin --floor 20 --agent
  ```
- **`pricing drift`**  -  Flag per-company pricing-book prices that have diverged from the master catalog price or cost.

  _Run this when a distributor price-list lands to see exactly which pricing books drifted away from catalog._

  ```bash
  salesbuildr-cli pricing drift --agent
  ```

### Pipeline analytics
- **`quote funnel`**  -  See stage-by-stage conversion of the quote lifecycle  -  count and dollar value at draft, sent, approved, declined, and expired.

  _Reach for this when you need quote-lifecycle conversion with dollars at each stage; for opportunity-pipeline conversion use 'opportunity winrate' instead._

  ```bash
  salesbuildr-cli quote funnel --agent
  ```
- **`opportunity velocity`**  -  Aggregate sales-cycle duration and time-in-stage across the opportunity pipeline.

  _Pick this when you need to know where deals stall and how long the pipeline actually takes to close._

  ```bash
  salesbuildr-cli opportunity velocity --agent
  ```
- **`opportunity winrate`**  -  Compute won/lost ratios grouped by owner, stage, or category.

  _Use this to report conversion by rep or stage without rebuilding it from memory each week._

  ```bash
  salesbuildr-cli opportunity winrate --by owner --agent
  ```
- **`opportunity mrr-forecast`**  -  Weight open-pipeline monthly revenue and profit by probability, churn-adjusted.

  _Reach for this for a fast weighted MRR forecast across the open pipeline._

  ```bash
  salesbuildr-cli opportunity mrr-forecast --agent
  ```

## Command Reference

**category**  -  Manage category

- `salesbuildr-cli category public-get-all-categories`  -  Get all categories
- `salesbuildr-cli category public-get-all-categories-as-tree`  -  Get all categories as tree

**company**  -  Manage company

- `salesbuildr-cli company public-create`  -  Create
- `salesbuildr-cli company public-delete`  -  Delete
- `salesbuildr-cli company public-delete-by-external-identifier`  -  Delete by ext ID
- `salesbuildr-cli company public-get`  -  Get
- `salesbuildr-cli company public-get-by-external-identifier`  -  Get by ext ID
- `salesbuildr-cli company public-get-list`  -  Search
- `salesbuildr-cli company public-update`  -  Update
- `salesbuildr-cli company public-upsert`  -  Create or update a company using the ext ID as key

**contact**  -  Manage contact

- `salesbuildr-cli contact company-public-create`  -  Create
- `salesbuildr-cli contact company-public-delete`  -  Delete by ext ID
- `salesbuildr-cli contact company-public-delete-by-id`  -  Delete by ID
- `salesbuildr-cli contact company-public-get`  -  Get
- `salesbuildr-cli contact company-public-get-by-external-identifier`  -  Get by ext ID
- `salesbuildr-cli contact company-public-get-list`  -  Search contacts
- `salesbuildr-cli contact company-public-undelete-by-ext-id`  -  Undelete by ext ID
- `salesbuildr-cli contact company-public-update`  -  Update
- `salesbuildr-cli contact company-public-upsert`  -  Upsert by ext ID
- `salesbuildr-cli contact company-public-upsert-by-email`  -  Upsert by email address

**contract**  -  Manage contract

- `salesbuildr-cli contract`  -  ⚠️ **Beta API** - This endpoint is in beta and may change without notice. Use with caution in production environments.

**field**  -  Manage field

- `salesbuildr-cli field public-get-values`  -  Get the list of values from a field
- `salesbuildr-cli field public-update-values`  -  Replace existing field values with new list

**opportunity**  -  Manage opportunity

- `salesbuildr-cli opportunity public-get-by-id`  -  Get by ID
- `salesbuildr-cli opportunity public-get-opportunities`  -  Search
- `salesbuildr-cli opportunity public-lose`  -  Lose by ext ID
- `salesbuildr-cli opportunity public-upsert`  -  Create or update a opportunity using the ext ID as key
- `salesbuildr-cli opportunity public-win`  -  Win by ext ID

**pricing-book**  -  Manage pricing book

- `salesbuildr-cli pricing-book public-create`  -  Create a new pricing book
- `salesbuildr-cli pricing-book public-get`  -  Get pricing book by ID
- `salesbuildr-cli pricing-book public-get-all`  -  Get pricing books
- `salesbuildr-cli pricing-book public-update`  -  Update an existing pricing book

**product**  -  Manage product

- `salesbuildr-cli product public-create`  -  Create product
- `salesbuildr-cli product public-create-batch`  -  Create products in batch. Maximum 100 products
- `salesbuildr-cli product public-delete`  -  Delete product
- `salesbuildr-cli product public-delete-by-external-identifier`  -  Delete product by external identifier
- `salesbuildr-cli product public-get`  -  Search
- `salesbuildr-cli product public-get-by-external-identifier`  -  Get product by external identifier
- `salesbuildr-cli product public-get-id`  -  Get product by ID
- `salesbuildr-cli product public-update`  -  Update product
- `salesbuildr-cli product public-upsert-by-external-identifier`  -  Upsert product by external identifier

**quote**  -  Manage quote

- `salesbuildr-cli quote public-create-approved`  -  Create approved quote
- `salesbuildr-cli quote public-create-draft`  -  Create draft quote
- `salesbuildr-cli quote public-get`  -  Search
- `salesbuildr-cli quote public-get-approved`  -  Search approved
- `salesbuildr-cli quote public-get-id`  -  Get

**quote-discount-group**  -  Manage quote discount group

- `salesbuildr-cli quote-discount-group discount-group-public-create`  -  Create new
- `salesbuildr-cli quote-discount-group discount-group-public-get-all`  -  Get all
- `salesbuildr-cli quote-discount-group discount-group-public-get-by-id`  -  Get by id
- `salesbuildr-cli quote-discount-group discount-group-public-update`  -  Update by id

**quote-template**  -  Manage quote template

- `salesbuildr-cli quote-template public-create`  -  Creates a new quote template. All product references should use internal Sfindr product IDs.
- `salesbuildr-cli quote-template public-get-by-id`  -  Returns a single quote template with all its contents. The response can be used directly in PUT requests.
- `salesbuildr-cli quote-template public-list`  -  Returns a list of all quote templates ordered by their display order.
- `salesbuildr-cli quote-template public-update`  -  Updates an existing quote template. All product references should use internal Sfindr product IDs.
- `salesbuildr-cli quote-template public-validate-template`  -  Validates template data including product references and attachments.

**quote-widget-template**  -  Manage quote widget template

- `salesbuildr-cli quote-widget-template public-create`  -  Creates a new widget template. The widget configuration should match the structure used in copy/paste operations.
- `salesbuildr-cli quote-widget-template public-get-by-id`  -  Returns a single widget template with all its contents. The response can be used directly in PUT requests.
- `salesbuildr-cli quote-widget-template public-list`  -  Returns a list of all widget templates ordered by their display order.
- `salesbuildr-cli quote-widget-template public-update`  -  Updates an existing widget template. Only provided fields will be updated.
- `salesbuildr-cli quote-widget-template public-validate-widget`  -  Validates widget template data including product references.

**sync-state**  -  Manage sync state

- `salesbuildr-cli sync-state <id>`  -  This endpoint is exempt from rate limiting


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
salesbuildr-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Weekly pipeline review

```bash
salesbuildr-cli sync && salesbuildr-cli quote stale --days 14 && salesbuildr-cli opportunity velocity
```

Refresh the store, then list aging quotes and pipeline cycle-time in one pass.

### Margin guard before sending

```bash
salesbuildr-cli quote thin --floor 20 --agent
```

List only the under-priced lines with the fields that matter, narrowed for an agent.

### Distributor price-list landed

```bash
salesbuildr-cli sync && salesbuildr-cli pricing drift
```

After re-syncing the catalog, flag pricing books whose prices drifted from catalog.

### Account-review cross-sell

```bash
salesbuildr-cli company whitespace "Acme Managed IT" --json
```

Catalog products this company has never been quoted, ready to pipe into prep notes.

### Catalog margin audit in SQL

```bash
salesbuildr-cli sql "SELECT name, mpn, markup, price FROM product WHERE markup < 25 ORDER BY markup"
```

Query the synced catalog offline for products priced below a 25% markup floor.

## Auth Setup

Set SALESBUILDR_API_KEY to an API key generated under Admin > Integrations > API Key, and SALESBUILDR_TENANT to your tenant subdomain (acme for https://acme.salesbuildr.com). Advanced: SALESBUILDR_BASE_URL overrides the full base URL and must include the /public-api suffix (https://acme.salesbuildr.com/public-api). The CLI sends the key in the api-key header on every request.

Run `salesbuildr-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  salesbuildr-cli category public-get-all-categories --agent --select id,name,status
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
salesbuildr-cli feedback "the --since flag is inclusive but docs say exclusive"
salesbuildr-cli feedback --stdin < notes.txt
salesbuildr-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/salesbuildr-cli/feedback.jsonl`. They are never POSTed unless `SALESBUILDR_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SALESBUILDR_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
salesbuildr-cli profile save briefing --json
salesbuildr-cli --profile briefing category public-get-all-categories
salesbuildr-cli profile list --json
salesbuildr-cli profile show briefing
salesbuildr-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `salesbuildr-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/salesbuildr/cmd/salesbuildr-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add salesbuildr-mcp -- salesbuildr-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which salesbuildr-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   salesbuildr-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `salesbuildr-cli <command> --help`.
