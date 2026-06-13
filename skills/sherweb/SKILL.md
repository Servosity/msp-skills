---
name: sherweb
description: "Every Sherweb Partner API capability, plus a local SQLite store, offline analytics, and margin/drift/orphan joins no other Sherweb tool has. Trigger phrases: `reconcile sherweb billing`, `what's my margin per customer in sherweb`, `find orphaned sherweb subscriptions`, `list sherweb customers`, `preview a sherweb seat change`, `use sherweb`, `run sherweb-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Sherweb"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - sherweb-cli
    install:
      - kind: go
        bins: [sherweb-cli]
        module: github.com/mvanhorn/printing-press-library/library/commerce/sherweb/cmd/sherweb-cli
---

# Sherweb  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `sherweb-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install sherweb --cli-only
   ```
2. Verify: `sherweb-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/commerce/sherweb/cmd/sherweb-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

sherweb-cli wraps the full documented Sherweb Distributor and Service Provider surface  -  customers, subscriptions, payable and receivable charges, platforms, catalog, orders  -  then adds what the portal and every existing wrapper lack: an offline SQLite copy, offline analytics, agent-native --json/--select, --dry-run on every mutation, and cross-entity reporting like margin per customer (receivable minus payable), bill drift across snapshots, and orphaned-subscription detection.

## When to Use This CLI

Reach for sherweb-cli when an agent or operator needs to reconcile Sherweb billing, audit subscription inventory across the whole customer book, preview or apply seat changes safely, or place/track marketplace orders  -  especially when the question spans more than one customer or needs offline, scriptable output. It is the right tool for monthly-close margin reconciliation, zombie-subscription hunts, and seat right-sizing that the Sherweb portal answers one customer at a time. The cross-customer insight commands (margin, drift, orphans, fleet-subs, right-size, amend-preview, margin-trend, sub-changes, usage-leak) read the local SQLite store: run 'sherweb-cli sync' then 'sherweb-cli deep-sync' first to populate it.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`margin`**  -  See net margin per customer (what they owe you minus what you owe Sherweb) for any billing month.

  _Reach for this at monthly close to find margin leaks instead of rebuilding a payable-vs-receivable spreadsheet._

  ```bash
  sherweb-cli margin --month 2026-04 --agent --select customerName,payable,receivable,margin
  ```
- **`drift`**  -  Diff the latest payable-charges snapshot against the prior sync to flag charges that appeared, vanished, or changed.

  _Use it after each sync to catch surprise charges before the invoice lands._

  ```bash
  sherweb-cli drift --since 2026-03-01 --agent
  ```
- **`orphans`**  -  List active subscriptions with zero receivable charges in the latest period  -  zombie subs you're paying for but not billing on.

  _Run it on the weekly inventory sweep to stop paying Sherweb for subscriptions no customer is funding._

  ```bash
  sherweb-cli orphans --agent
  ```
- **`margin-trend`**  -  See each customer's margin across the last N monthly closes so profitability slides surface before an account goes negative.

  _Reach for this when the question is whether an account is getting less profitable over time, not a single month's number._

  ```bash
  sherweb-cli margin-trend --customer 0aa00000-0000-0000-0000-000000000000 --last 6 --agent
  ```
- **`usage-leak`**  -  Flag metered platform usage that has no matching receivable charge - consumption you absorb instead of billing.

  _Reach for this during billing reconciliation to catch metered consumption silently eating margin._

  ```bash
  sherweb-cli usage-leak --customer 0aa00000-0000-0000-0000-000000000000 --agent
  ```

### Fleet visibility
- **`fleet-subs`**  -  One table across ALL customers' subscriptions grouped by product/SKU, with total seats and customer counts.

  _Use it to answer 'how many seats of X across the whole book' without paging customer-by-customer._

  ```bash
  sherweb-cli fleet-subs --product "Microsoft 365" --agent
  ```
- **`right-size`**  -  Flag subscriptions where seats paid differ from metered seats used, per customer.

  _Reach for it before renewals to trim over-provisioned seats and catch under-provisioning._

  ```bash
  sherweb-cli right-size --customer 00000000-0000-0000-0000-000000000000 --agent
  ```
- **`sub-changes`**  -  Flag subscriptions added, cancelled, or quantity-changed between two syncs across the whole customer book.

  _Reach for this to audit what changed across all customers since the last sync instead of paging customer by customer._

  ```bash
  sherweb-cli sub-changes --since 30d --agent
  ```

### Mutation safety
- **`amend-preview`**  -  Preview the monthly cost change of a seat amendment before committing it, computed from stored pricing.

  _Use it on every seat-change ticket so you see the dollar impact before the amendment fires._

  ```bash
  sherweb-cli amend-preview --customer 00000000-0000-0000-0000-000000000000 --sub SUB123 --qty 25 --agent
  ```

## Command Reference

**distributor**  -  Manage distributor

- `sherweb-cli distributor`  -  Returns the charges you are billed by Sherweb for a billing period: products, quantities, list/net pricing, fees, taxes

**service-provider**  -  Manage service provider

- `sherweb-cli service-provider amend-subscriptions`  -  Submit one or more subscription quantity changes. Returns a tracking id.
- `sherweb-cli service-provider cancel-subscriptions`  -  Cancel one or more of a customer's subscriptions
- `sherweb-cli service-provider get-customer-catalog`  -  Get the product catalog available to a specific customer
- `sherweb-cli service-provider get-customer-catalog-items-pricing`  -  Get pricing for specific catalog items for a customer
- `sherweb-cli service-provider get-customer-platform-details`  -  Get details for one provisioned platform of a customer
- `sherweb-cli service-provider get-customer-platform-meter-usages`  -  Get metered usage for a customer's platform
- `sherweb-cli service-provider get-customer-platforms-configurations`  -  Get the platform configurations provisioned for a customer
- `sherweb-cli service-provider get-platform-required-parameters`  -  Get the required provisioning parameters for given SKUs/platform
- `sherweb-cli service-provider get-platforms-for-skus`  -  Resolve which platforms can fulfill given SKUs
- `sherweb-cli service-provider get-receivable-charges`  -  Get receivable charges (what a customer owes you) for a date
- `sherweb-cli service-provider get-subscription-meters`  -  Get metered subscriptions for a customer
- `sherweb-cli service-provider get-subscriptions-pricing`  -  Get pricing information for a customer's subscriptions
- `sherweb-cli service-provider get-tracking-status`  -  Get the status of an order or amendment by tracking id
- `sherweb-cli service-provider list-customers`  -  List the customers under your reseller account
- `sherweb-cli service-provider list-platforms`  -  List the provisioning platforms available to you
- `sherweb-cli service-provider list-subscriptions`  -  List a customer's subscriptions with details
- `sherweb-cli service-provider place-order`  -  Place a marketplace order for a customer
- `sherweb-cli service-provider validate-order`  -  Validate an order cart for a customer before placing it


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
sherweb-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Monthly margin close

```bash
sherweb-cli sync && sherweb-cli deep-sync && sherweb-cli margin --month 2026-04 --agent --select customerName,payable,receivable,margin
```

Sync customers + payable charges, fan out per-customer receivables with deep-sync, then emit per-customer margin as structured rows ready to pipe into a close workbook.

### Catch surprise charges

```bash
sherweb-cli drift --since 2026-03-01 --agent
```

Diff this period's payable charges against the prior snapshot to flag anything new or changed.

### Zombie subscription sweep

```bash
sherweb-cli orphans --agent
```

List active subscriptions with no receivable charges so you stop paying Sherweb for unbilled seats.

### Right-size before renewal

```bash
sherweb-cli right-size --customer 00000000-0000-0000-0000-000000000000 --agent
```

Compare seats paid against metered usage to trim over-provisioned subscriptions.

### Preview a seat change

```bash
sherweb-cli amend-preview --customer 00000000-0000-0000-0000-000000000000 --sub SUB123 --qty 25
```

See the monthly cost delta of an amendment before committing it with the real amend command.

## Auth Setup

Sherweb uses composed auth: an OAuth2 client-credentials bearer token (minted from https://api.sherweb.com/auth/oidc/connect/token with a per-API scope of distributor and/or service-provider) PLUS an Azure APIM gateway key sent as the Ocp-Apim-Subscription-Key header on every call. Set SHERWEB_CLIENT_ID, SHERWEB_CLIENT_SECRET, and SHERWEB_SUBSCRIPTION_KEY (all from cumulus.sherweb.com Security > APIs). The client mints and caches the token; the subscription key rides on every request including the token request.

Run `sherweb-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  sherweb-cli distributor --agent --select id,name,status
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
sherweb-cli feedback "the --since flag is inclusive but docs say exclusive"
sherweb-cli feedback --stdin < notes.txt
sherweb-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/sherweb-cli/feedback.jsonl`. They are never POSTed unless `SHERWEB_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SHERWEB_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
sherweb-cli profile save briefing --json
sherweb-cli --profile briefing distributor
sherweb-cli profile list --json
sherweb-cli profile show briefing
sherweb-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `sherweb-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/commerce/sherweb/cmd/sherweb-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add sherweb-mcp -- sherweb-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which sherweb-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   sherweb-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `sherweb-cli <command> --help`.
