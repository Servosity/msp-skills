---
name: pax8
description: "Every Pax8 Partner API endpoint, plus an offline store that reconciles billing, tracks MRR, and catches usage overages no Pax8 tool surfaces. Trigger phrases: `reconcile pax8 billing`, `what is my pax8 mrr`, `list pax8 companies`, `check pax8 usage overages`, `pax8 customer 360`, `use pax8`, `run pax8-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Pax8"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - pax8-cli
    install:
      - kind: go
        bins: [pax8-cli]
        module: github.com/mvanhorn/printing-press-library/library/commerce/pax8/cmd/pax8-cli
---

# Pax8  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `pax8-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install pax8 --cli-only
   ```
2. Verify: `pax8-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/commerce/pax8/cmd/pax8-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

pax8-cli mirrors the full Pax8 Partner API as typed, agent-native commands, then extends it with a local SQLite store. Reconcile invoices against subscriptions, compute MRR and margin, catch metered overages before they bill, and pull a true customer-360  -  the reports the portal makes you build by hand.

## When to Use This CLI

Use pax8-cli when an MSP needs to do anything with the Pax8 Partner API from the terminal or an agent: list/create/update companies and contacts, pull products and pricing, manage subscriptions and orders, fetch invoices and usage, and especially the cross-entity analytics (reconcile, mrr, overage, spend, customer-360) that the Pax8 portal cannot produce in one place. The six analytics commands (reconcile, mrr, overage, since, company show, spend) read the local synced store  -  run sync first; sync needs PAX8_CLIENT_ID/PAX8_CLIENT_SECRET.

## Anti-triggers

Do not use this CLI for:
- PSA/RMM tasks (tickets, alerts, patching)  -  Pax8 only covers marketplace billing/provisioning
- Vendor-portal administration (Microsoft 365 admin, Azure resources)  -  Pax8 resells products, it cannot administer them
- Payments or invoice disputes with Pax8 itself  -  portal/support workflow, not a Partner API surface
- Quoting/CPQ workflows  -  the Partner API has no quote endpoints

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`reconcile`**  -  Flag invoice lines that no longer match an active subscription, and active subscriptions that never got billed; --draft pre-checks the next unposted invoice.

  _Reach for this when an MSP needs to catch billing leakage the Pax8 portal can't surface in one place._

  ```bash
  pax8-cli reconcile --agent
  ```
- **`mrr`**  -  Compute monthly recurring revenue and margin from subscriptions and product pricing, trended across syncs, with a per-product breakdown.

  _Reach for this when an agent needs the revenue number the portal makes you build in a spreadsheet._

  ```bash
  pax8-cli mrr --agent --select totals.mrr,totals.margin
  ```
- **`overage`**  -  Aggregate usage-lines per subscription and surface overages before they land on the customer invoice.

  _Reach for this to catch metered overages proactively instead of after the invoice posts._

  ```bash
  pax8-cli overage --agent
  ```
- **`since`**  -  Diff subscription snapshots and history over time to show new, cancelled, and quantity-changed subscriptions.

  _Reach for this to answer 'what changed in my book of business' without diffing exports by hand._

  ```bash
  pax8-cli since 7d --agent
  ```
- **`company show`**  -  One view of a company with its subscriptions, contacts, invoices, and usage.

  _Reach for this when an agent needs a full picture of one customer in a single call._

  ```bash
  pax8-cli company show <companyId> --agent --select subscriptions.productName,invoices.total
  ```
- **`spend`**  -  Roll up invoice-items by company to show what each customer is costing across invoices.

  _Reach for this to rank customers by spend without exporting every invoice._

  ```bash
  pax8-cli spend --agent
  ```

## Command Reference

**companies**  -  Companies represent the customers your organization serves through the Pax8 platform. Fetch a list of your company records, or a specific company by its companyId

- `pax8-cli companies create-company`  -  Creates a new company.
- `pax8-cli companies find`  -  Returns a paginated list of all your companies filtered by optional parameters
- `pax8-cli companies get`  -  Returns a single company record matching the ```companyId``` you specify
- `pax8-cli companies update-company`  -  Updates an existing Company. ATTENTION - at least one parameter has to be modified.

**invoices**  -  Manage invoices

- `pax8-cli invoices find-partner`  -  Fetch a paginated list of invoices. Default page is 0 and default size is 10. The maximum page size is 200
- `pax8-cli invoices find-partner-draft-items`  -  Fetch a paginated list of draft invoice items before they are finalized into invoices.
- `pax8-cli invoices get-partner`  -  Fetch a paginated list of invoices. Default page is 0 and default size is 10. The maximum page size is 200

**orders**  -  Orders describe your purchases of Pax8 products. Endpoints let you create new product orders and query past purchases

- `pax8-cli orders create`  -  Create a new order. Currently NOT supported for scheduled orders(orders with a future date).
- `pax8-cli orders find`  -  Returns a paginated list of orders. Currently NOT supported for scheduled orders(orders with a future date).
- `pax8-cli orders find-by-id`  -  Returns the Order record specified by OrderId. Currently NOT supported for scheduled orders

**products**  -  The Products resource lets you fetch all the information required to complete an Order -- product descriptions, pricing, provisioning details, and the dependencies associated with each product

- `pax8-cli products find-all`  -  Returns a paginated list of Pax8 products filtered by optional query parameters
- `pax8-cli products find-by-id`  -  Returns only the product record for the productId you specify

**subscriptions**  -  Subscriptions describe terms of service, including price and the billing start/end dates for a specified product/company combination. Resources let you update, cancel, and fetch details of existing subscriptions

- `pax8-cli subscriptions delete`  -  Cancels the Subscription specified by subscriptionId
- `pax8-cli subscriptions find`  -  Fetch a paginated list of subscriptions. Default page is 0 and default size is 10. The maximum page size is 200
- `pax8-cli subscriptions find-by-id`  -  Returns the Subscription record specified by the subscriptionId
- `pax8-cli subscriptions update`  -  Updates a subscription. Currently NOT supported for subscriptions with a future date.

**usage-summaries**  -  Manage usage summaries

- `pax8-cli usage-summaries <usageSummaryId>`  -  Fetch a paginated list of usage summaries. Default page is 0 and default size is 10. The maximum page size is 200


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
pax8-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Billing reconciliation for the current period

```bash
pax8-cli reconcile --agent
```

Surfaces invoice lines with no active subscription and active subscriptions that were never billed.

### MRR and margin, narrowed for an agent

```bash
pax8-cli mrr --agent --select totals.mrr,totals.margin,byProduct.productName
```

Returns just the revenue numbers an agent needs from a wide aggregate, using dotted select paths.

### Customer 360 in one call

```bash
pax8-cli company show <companyId> --agent --select subscriptions.productName,subscriptions.quantity,invoices.total
```

Joins a company to its subscriptions and invoices locally and narrows the deeply nested result.

### What changed this week

```bash
pax8-cli since 7d --agent
```

Diffs subscription snapshots to show new, cancelled, and quantity-changed subscriptions.

### Find a product by vendor

```bash
pax8-cli search 'microsoft 365' --json
```

Full-text search across the offline store with no API round-trip.

## Auth Setup

Pax8 uses OAuth2 client-credentials. Set PAX8_CLIENT_ID and PAX8_CLIENT_SECRET (create an API key in the Pax8 portal under Integrations). The CLI exchanges them at https://token-manager.pax8.com/oauth/token with the required audience (override with PAX8_AUDIENCE, default api://p8p.client) and caches the bearer token until it expires. Run `pax8-cli doctor` to verify the token exchange; `pax8-cli auth status` shows the active credential source and `pax8-cli auth login` stores credentials interactively.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  pax8-cli companies get <id> --agent --select id,name,status
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
pax8-cli feedback "the --since flag is inclusive but docs say exclusive"
pax8-cli feedback --stdin < notes.txt
pax8-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/pax8-cli/feedback.jsonl`. They are never POSTed unless `PAX8_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PAX8_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
pax8-cli profile save briefing --json
pax8-cli --profile briefing companies get <id>
pax8-cli profile list --json
pax8-cli profile show briefing
pax8-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `pax8-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/commerce/pax8/cmd/pax8-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add pax8-mcp -- pax8-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which pax8-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   pax8-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `pax8-cli <command> --help`.
