---
name: td-synnex
description: "Use when the user asks about TD SYNNEX StreamOne ION (the ION reseller marketplace for Microsoft 365, Azure and security licences): subscriptions renewing soon or with auto-renew off, margin per customer or SKU, seat true-up against Pax8, Sherweb or PSA exports, stuck or failed orders, recent seat and renewal changes, one customer's full picture, or changing seats, renewal settings or status on an ION subscription. Trigger phrases: `what TD SYNNEX subscriptions renew this month`, `which ION customers am I losing money on`, `change seats on a StreamOne ION subscription`, `true up licences across TD SYNNEX and Pax8`, `which ION orders are stuck on hold`, `use td-synnex`, `run td-synnex`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "TD SYNNEX"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - td-synnex-cli
    install:
      - kind: script
        bins: [td-synnex-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.ps1
---

# TD SYNNEX StreamOne ION - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `td-synnex-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.ps1 | iex
   ```
3. Verify: `td-synnex-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `td-synnex-cli` and `td-synnex-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Manage customers, catalog, orders, subscriptions, carts and billing reports from the terminal or any agent. Sync your book to SQLite and answer book-wide questions no single endpoint returns: what renews this month (subscriptions renewals), where you lose money (margin), what changed (subscriptions changes), and how TD SYNNEX seats line up against Pax8 and Sherweb exports (position trueup). Seat and renewal changes become one previewed command (subscriptions set) instead of a hand-built order body.

## When to Use This CLI

Use this CLI when an agent or operator needs to read or change a TD SYNNEX StreamOne ION reseller account: customers, catalog and pricing, orders, subscription seats and renewals, carts, tenants and billing reports. It is the right tool for book-wide questions (renewals, margin, change history, stuck orders) after a sync, and for preparing licence true-ups against Pax8 or Sherweb exports.

## Anti-triggers

Do not use this CLI for:
- Do not use it for TD SYNNEX invoices; v3 has no invoicing API (use the ION portal or the v1 API)
- Do not use it to place Microsoft Reserved Instance orders; ION only accepts those in the portal
- Do not use it for Pax8 or Sherweb accounts directly; use their own CLIs and feed their exports to position trueup
- Do not use it for TD SYNNEX Digital Bridge or StreamOne Cloud Marketplace (legacy) accounts

## Unique Capabilities

### Licence book you can query
- **`subscriptions renewals`**  -  See every subscription renewing or ending in the next N days across the whole book, soonest first, with auto-renew-off ones flagged.

  _Reach for this before renewal reviews or customer calls instead of paging every customer's subscriptions._

  ```bash
  td-synnex-cli subscriptions renewals --within 30d --agent
  ```
- **`margin`**  -  See monthly cost vs price and margin per customer or per SKU, with negative, zero and below-MSRP rows flagged.

  _Use it to find unprofitable customers or SKUs without exporting a billing report._

  ```bash
  td-synnex-cli margin --by customer --agent
  ```
- **`subscriptions changes`**  -  List seat, renewal, status, tier and friendly-name changes requested through ION orders in a past window, decoded from the order attributes; failed orders are left out and changes made outside ION are not visible.

  _Use it to explain why a bill moved or to feed only the deltas into a PSA sync._

  ```bash
  td-synnex-cli subscriptions changes --since 7d --agent
  ```

### Cross-distributor true-up
- **`position export`**  -  Export your TD SYNNEX licences as normalized, schema-versioned rows (customer, SKU key, seats, monthly cost and price, renewal) for a PSA or a true-up; tenant and domain columns stay blank because ION subscriptions do not carry them.

  _Use it when another system or a Pax8/Sherweb comparison needs TD SYNNEX seats in one predictable shape._

  ```bash
  td-synnex-cli position export --csv
  ```
- **`position trueup`**  -  Compare seats per customer and SKU across TD SYNNEX and Pax8, Sherweb or PSA export files mapped into the licence-position columns, with unmatched rows listed instead of guessed.

  _Use it at month-end to find duplicate purchases and seat mismatches across distributors; input rows need a Microsoft product:SKU sku_key, and customers match on exact normalized names because TD SYNNEX rows carry no tenant or domain._

  ```bash
  td-synnex-cli position trueup --with pax8-positions.csv --map company=customer_name --map sku=sku_key --map quantity=seats --agent
  ```

### Order and subscription operations
- **`orders stuck`**  -  Find orders still NEW, IN_PROGRESS, CONFIRMED or ON_HOLD past an age threshold, plus recent errors, with a flag for the ones you can still cancel.

  _Use it daily after placing orders so nothing sits ON_HOLD unnoticed._

  ```bash
  td-synnex-cli orders stuck --older-than 24h --agent
  ```
- **`customers show`**  -  One view of a customer from the local store: profile, subscriptions with seats and monthly cost and price, open orders and recent changes, plus the cloud-profiles command for live GDAP/MCA status.

  _Use it before a customer conversation instead of clicking through the portal._

  ```bash
  td-synnex-cli customers show 108536 --agent
  ```

## Command Reference

**cart_items**  -  Manage cart items

- `td-synnex-cli cart-items create`  -  Add a plan to a cart.
- `td-synnex-cli cart-items delete`  -  Remove an item from a cart (the docs site's Cart Items intro mentions removing items).
- `td-synnex-cli cart-items get`  -  Get one cart item. Source: https://docs.streamone.cloud/#get-cart-item Official spec: GetCartItem.
- `td-synnex-cli cart-items list`  -  List the items in a cart with plan, quantity, prices, add-ons, term and checkout state.
- `td-synnex-cli cart-items update`  -  Update a cart item's quantity, plan, add-ons or payment. Source: https://docs.streamone.

**carts**  -  Manage carts

- `td-synnex-cli carts checkout`  -  Start the purchase of a cart, which places a real, billable order.
- `td-synnex-cli carts create`  -  Create a cart for a customer. Source: https://docs.streamone.cloud/#create-cart Official spec: CreateCart.
- `td-synnex-cli carts delete`  -  Clear or delete a cart. Source: official spec DeleteCart (not on the docs site).
- `td-synnex-cli carts get`  -  Get one cart (a flat cart object per the official spec; the docs site sample wraps it in a `carts` array).
- `td-synnex-cli carts list`  -  List a customer's carts with status, payment type, currency and item count.
- `td-synnex-cli carts update`  -  Update a cart's display name, payment, status or currency. Source: https://docs.streamone.

**categories**  -  Manage categories

- `td-synnex-cli categories <accountId>`  -  List product categories (Security, Cloud Services and others) with product counts and parent/child links.

**charges**  -  Manage charges

- `td-synnex-cli charges <accountId>`  -  List the account's pricing charge rules (markup/margin plans): charge name, plan type (FIXED_AMOUNT, FIXED_PERCENT

**cloud_profiles**  -  Manage cloud profiles

- `td-synnex-cli cloud-profiles <accountId> <customerId>`  -  Show which cloud providers a customer is linked to (Microsoft CSP, Google Workspace, Azure and others) with tenant ID

**cloud_providers**  -  Manage cloud providers

- `td-synnex-cli cloud-providers get-customer`  -  Get the customer's association with one cloud provider: provisioned domain, status, partner tenant ID
- `td-synnex-cli cloud-providers link-customer`  -  Link a tenant the customer already owns at the cloud provider to the ION customer.
- `td-synnex-cli cloud-providers provision-customer`  -  Create a new tenant for the customer at the cloud provider (for example a Microsoft tenant for provider 30).

**customer_orders**  -  Manage customer orders

- `td-synnex-cli customer-orders cancel-order`  -  Cancel an order. Allowed only while the order status is NEW or ON_HOLD, right after it was placed. No request body.
- `td-synnex-cli customer-orders create-order`  -  Place a real, billable order for a customer, or change an existing subscription.
- `td-synnex-cli customer-orders get-order`  -  Get one order with its status, totals, order items, provisioning info and activity log. Source: https://docs.streamone.
- `td-synnex-cli customer-orders list`  -  List one customer's order history. Source: https://docs.streamone.

**customers**  -  List, get, create and update customers, and show one customer from the local store

- `td-synnex-cli customers create`  -  Create an end customer.
- `td-synnex-cli customers get`  -  Get one end customer by ID. Source: https://docs.streamone.cloud/#get-customer Official spec: GetCustomer.
- `td-synnex-cli customers list`  -  List end customers under the reseller account, oldest first.
- `td-synnex-cli customers update`  -  Replace a customer's organization, address and contact details.

**orders**  -  Manage orders

- `td-synnex-cli orders <accountId>`  -  List the reseller's order history across all customers, with order items, prices, costs and margins.

**pricing**  -  Manage pricing

- `td-synnex-cli pricing <accountId>`  -  Price several products in one call for a price-book customer: price-book price, account cost, MSRP-style master price

**products**  -  Manage products

- `td-synnex-cli products disable`  -  Hide a catalog product for the account (toggles its visibility in the storefront). No request body.
- `td-synnex-cli products enable`  -  Show a catalog product for the account (toggles its visibility in the storefront). No request body.
- `td-synnex-cli products get`  -  Get one catalog product with marketing, definition (SKUs, plans, phases, prices)
- `td-synnex-cli products list`  -  List catalog products available to the reseller account, with SKUs, plans and list pricing in `definition.skus[].

**programs**  -  Manage programs

- `td-synnex-cli programs batch-update-product`  -  Configure which vendor programs apply to a product.
- `td-synnex-cli programs list`  -  List the vendor programs defined for the account and, with --product-name, whether each is attached to that product.

**provision_templates**  -  Manage provision templates

- `td-synnex-cli provision-templates get`  -  Get one provisioning template by ID.
- `td-synnex-cli provision-templates list`  -  Get the JSON schemas that define the order-item attributes a vendor needs to create or update a subscription (Microsoft

**reports**  -  List report definitions, run a report and get its data, save, and delete reports

- `td-synnex-cli reports delete`  -  Delete a saved report if the user has permission (the docs site's Reports intro lists deleting reports).
- `td-synnex-cli reports get`  -  POST a report definition and get back its resolved structure (columns, currency, date ranges) without data
- `td-synnex-cli reports get-data`  -  Run a report: POST the report definition from the metadata endpoint (with the desired date range, currency and columns)
- `td-synnex-cli reports get-data-csv`  -  Same request as the report data endpoint
- `td-synnex-cli reports get-details`  -  Retrieve the full report details with populated specs, returned as priceDetails and costDetails.
- `td-synnex-cli reports get-metadata`  -  Get a report's structure without data: all and selected columns, column filters, date-range options, currencies
- `td-synnex-cli reports list`  -  List the predefined and saved reports available to the account (billing, customer, subscription reports) with their IDs
- `td-synnex-cli reports save`  -  Update a saved report's settings (columns, filters, date range, currency) by POSTing the edited report definition.

**subscriptions**  -  List and get subscriptions, change seats, renewal or status (set), and see renewals and changes across the book

- `td-synnex-cli subscriptions get`  -  Get one subscription for a customer, including billing data. Pass --refresh to refresh the result from the provider.
- `td-synnex-cli subscriptions set <accountId> <customerId> <subscriptionId>`  -  Change seats, renewal setting, status, tier or friendly name; previews the UPDATE order and places it only with --confirm.
- `td-synnex-cli subscriptions list`  -  List subscriptions across the account (or one customer with customerId): licence counts, status, term, billing cycle

**verticals**  -  Manage verticals

- `td-synnex-cli verticals <accountId>`  -  List the industry verticals (Commercial, Corporate, Education, Charity and others) used to segment products and pricing.


**customers**


- `td-synnex-cli customers show`  -  One view of a customer from the local store: profile, subscriptions with seats and monthly cost and price, open orders and recent changes, plus the cloud-profiles command for live GDAP/MCA status.


**margin**


- `td-synnex-cli margin`  -  See monthly cost vs price and margin per customer or per SKU, with negative, zero and below-MSRP rows flagged.


**orders**


- `td-synnex-cli orders stuck`  -  Find orders still NEW, IN_PROGRESS, CONFIRMED or ON_HOLD past an age threshold, plus recent errors, with a flag for the ones you can still cancel.


**position**


- `td-synnex-cli position export`  -  Export your TD SYNNEX licences as normalized, schema-versioned rows (customer, SKU key, seats, monthly cost and price, renewal) for a PSA or a true-up; tenant and domain columns stay blank because ION subscriptions do not carry them.

- `td-synnex-cli position trueup`  -  Compare seats per customer and SKU across TD SYNNEX and Pax8, Sherweb or PSA export files mapped into the licence-position columns, with unmatched rows listed instead of guessed.


**subscriptions**


- `td-synnex-cli subscriptions renewals`  -  See every subscription renewing or ending in the next N days across the whole book, soonest first, with auto-renew-off ones flagged.

- `td-synnex-cli subscriptions changes`  -  List seat, renewal, status, tier and friendly-name changes requested through ION orders in a past window, decoded from the order attributes; failed orders are left out and changes made outside ION are not visible.




## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. The refresh reads only the first page (about 100 rows) of each resource, so run `td-synnex-cli sync` before whole-book answers on a larger book. A command given its own `--db` never refreshes. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. The book commands written for this connector (`customers show`, `margin`, `orders stuck`, `position export`, `position trueup`, `subscriptions changes`, `subscriptions renewals`) are store-only: they accept `--data-source auto` or `local`, and `--data-source live` exits 1 with `no live equivalent for this command`; refresh their data with `td-synnex-cli sync`. Set `TD_SYNNEX_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

- `td-synnex-cli categories`
- `td-synnex-cli charges`
- `td-synnex-cli customers`
- `td-synnex-cli customers get`
- `td-synnex-cli customers list`
- `td-synnex-cli customers show` (store-only)
- `td-synnex-cli margin` (store-only)
- `td-synnex-cli orders`
- `td-synnex-cli orders stuck` (store-only)
- `td-synnex-cli position export` (store-only)
- `td-synnex-cli position trueup` (store-only)
- `td-synnex-cli products`
- `td-synnex-cli products get`
- `td-synnex-cli products list`
- `td-synnex-cli programs`
- `td-synnex-cli programs list`
- `td-synnex-cli reports`
- `td-synnex-cli reports get`
- `td-synnex-cli reports list`
- `td-synnex-cli subscriptions`
- `td-synnex-cli subscriptions changes` (store-only)
- `td-synnex-cli subscriptions get`
- `td-synnex-cli subscriptions list`
- `td-synnex-cli subscriptions renewals` (store-only)
- `td-synnex-cli verticals`

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
td-synnex-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Renewals coming up with auto-renew off

```bash
td-synnex-cli subscriptions renewals --within 45d --auto-renew off --agent
```

Lists subscriptions ending in 45 days that will not renew on their own, soonest first.

### Preview a seat change

```bash
td-synnex-cli subscriptions set 10505 108536 1608355 --seats 12
```

Builds the UPDATE order body from the subscription itself and prints it; add --confirm to place the order. Omit the account ID when TD_SYNNEX_ACCOUNT_ID is set.

### Narrow a large subscription listing

```bash
td-synnex-cli subscriptions list 10505 --pagination-limit 50 --agent --select customerName,subscriptionName,subscriptionTotalLicenses,renewalDate
```

Subscription rows are wide; select only the fields you need.

### Month-end true-up against Pax8

```bash
td-synnex-cli position trueup --with pax8-positions.csv --map company=customer_name --map sku=sku_key --map quantity=seats --agent
```

Maps a Pax8 export's columns into the licence-position format, joins it with TD SYNNEX positions, and lists seat mismatches plus rows it could not match.

### Orders that need attention

```bash
td-synnex-cli orders stuck --older-than 24h --since 7d
```

Shows unfinished orders older than a day and errors from the last week, flagging which can still be cancelled.

### Keep the token fresh from cron

```bash
td-synnex-cli auth mint --if-expiring 15m --json
```

Mints a new access token only when the stored one expires within 15 minutes, saving the rotated refresh token each time.

## Auth Setup

StreamOne ION uses OAuth 2.0 refresh tokens that are single-use and rotate on every exchange (32-day lifetime; access tokens last 2 hours). Get the first refresh token in the ION portal under Settings > Users > Edit > OAuth credentials > Request New Credentials and pipe it to `td-synnex-cli auth mint --refresh-token-stdin`. mint exchanges it and saves the access token and the NEW refresh token to the private credentials file before printing anything; later `auth mint` runs (for example `auth mint --if-expiring 15m` from cron) use the stored token. `auth check` reports how long both tokens stay valid. Do not give the refresh token to `auth set-token`, which stores an access token. Do not export TD_SYNNEX_ACCESS_TOKEN as well: it overrides the minted token and expires after 2 hours, while `auth mint --if-expiring` keeps the stored one fresh.

- `td-synnex-cli auth mint`  -  exchange the refresh token (stdin, stored, or TD_SYNNEX_REFRESH_TOKEN) for an access token and save the rotated refresh token.
- `td-synnex-cli auth check`  -  ask ION how long the access and refresh tokens stay valid.

Keep `TD_SYNNEX_ACCOUNT_ID` set permanently (shell profile, or `setx` on Windows): every ION API path needs it, `doctor` needs it to probe the token, and the local store is keyed on it, so without it each new access token starts an empty store.

Run `td-synnex-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  td-synnex-cli cart-items list 108536 240314 --agent --select cartName,checkedOut,currencyCode
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts; inputs are positional arguments and flags
- **Writes run immediately**  -  Writes run immediately: order, cart checkout, update and delete commands do not prompt, so preview them with `--dry-run`. `subscriptions set` only previews unless you add `--confirm`. Over MCP, the endpoint write tools (order, cart, customer, product, provisioning and report tools) have no dry-run and refuse any argument they do not declare; preview those writes with the CLI.
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

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

- Use `--home <dir>` for one invocation, or set `TD_SYNNEX_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `TD_SYNNEX_CONFIG_DIR`, `TD_SYNNEX_DATA_DIR`, `TD_SYNNEX_STATE_DIR`, `TD_SYNNEX_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `TD_SYNNEX_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `td-synnex-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "td-synnex": {
        "command": "td-synnex-mcp",
        "env": {
          "TD_SYNNEX_HOME": "/srv/td-synnex"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `TD_SYNNEX_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `TD_SYNNEX_HOME`, or `doctor` will not find credentials left under the former root.

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
td-synnex-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "td-synnex-cli learnings confirm 12"] }
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
       materially more, record the divergence via `td-synnex-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `td-synnex-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `td-synnex-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
td-synnex-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
td-synnex-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
td-synnex-cli teach-playbook \
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
td-synnex-cli playbook amend \
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

`td-synnex-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `TD_SYNNEX_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
td-synnex-cli feedback "the --since flag is inclusive but docs say exclusive"
td-synnex-cli feedback --stdin < notes.txt
td-synnex-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `TD_SYNNEX_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `TD_SYNNEX_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
td-synnex-cli profile save briefing --json
td-synnex-cli --profile briefing cart-items list 108536 240314
td-synnex-cli profile list --json
td-synnex-cli profile show briefing
td-synnex-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Other error (for example a sync where every resource failed) |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `td-synnex-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add td-synnex-mcp -- td-synnex-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which td-synnex-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   td-synnex-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `td-synnex-cli <command> --help`.
