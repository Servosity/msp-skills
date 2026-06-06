# Pax8 CLI

**Every Pax8 Partner API endpoint, plus an offline store that reconciles billing, tracks MRR, and catches usage overages no Pax8 tool surfaces.**

pax8-cli mirrors the full Pax8 Partner API as typed, agent-native commands, then extends it with a local SQLite store. Reconcile invoices against subscriptions, compute MRR and margin, catch metered overages before they bill, and pull a true customer-360  -  the reports the portal makes you build by hand.

For the short install path see [README.md](./README.md). This file is the command reference.

## Authentication

Pax8 uses OAuth2 client-credentials. Set PAX8_CLIENT_ID and PAX8_CLIENT_SECRET (create an API key in the Pax8 portal under Integrations). The CLI exchanges them at https://token-manager.pax8.com/oauth/token with the required audience (override with PAX8_AUDIENCE, default api://p8p.client) and caches the bearer token until it expires. Run `pax8-cli doctor` to verify the token exchange; `pax8-cli auth status` shows the active credential source and `pax8-cli auth login` stores credentials interactively.

## Quick Start

```bash
# confirm OAuth client-credentials exchange works before anything else
pax8-cli doctor

# pull companies, subscriptions, invoices, and usage into the local store
pax8-cli sync

# list the customers you serve through Pax8
pax8-cli companies find --json

# flag billing mismatches between invoices and subscriptions
pax8-cli reconcile

# see monthly recurring revenue and margin from the synced store
pax8-cli mrr

```

## Unique Features

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

## Usage

Run `pax8-cli --help` for the full command reference and flag list.

## Commands

### companies

Companies represent the customers your organization serves through the Pax8 platform. Fetch a list of your company records, or a specific company by its companyId

- **`pax8-cli companies create-company`** - Creates a new company. You can optionally include contacts in the request body to create the company with contacts in a single request. If no contacts are provided, the company will be placed in an "inactive" status until the [company has primary contacts added](#tag/Contacts). Once contacts are added, the company will move to "Active".
- **`pax8-cli companies find`** - Returns a paginated list of all your companies filtered by optional parameters
- **`pax8-cli companies get`** - Returns a single company record matching the ```companyId``` you specify
- **`pax8-cli companies update-company`** - Updates an existing Company. ATTENTION - at least one parameter has to be modified.

### invoices

Manage invoices

- **`pax8-cli invoices find-partner`** - Fetch a paginated list of invoices. Default page is 0 and default size is 10. The maximum page size is 200
- **`pax8-cli invoices find-partner-draft-items`** - Fetch a paginated list of draft invoice items before they are finalized into invoices. Default page is 0 and default size is 10. The maximum page size is 200.

DISCLAIMER: For future-dated items, the data represents a snapshot of charges that will be applied to a future invoice as of the request date. This is not an actual invoice. Clients should be aware that the final invoice generated on the 5th of the month may differ. This endpoint does not include charges for usage-based products billed in arrears. Tax calculations are based on current account taxability status and current month tax rates, both of which are subject to change for future invoices.
- **`pax8-cli invoices get-partner`** - Fetch a paginated list of invoices. Default page is 0 and default size is 10. The maximum page size is 200

### orders

Orders describe your purchases of Pax8 products. Endpoints let you create new product orders and query past purchases

- **`pax8-cli orders create`** - Create a new order. Currently NOT supported for scheduled orders(orders with a future date).
- **`pax8-cli orders find`** - Returns a paginated list of orders. Currently NOT supported for scheduled orders(orders with a future date).
- **`pax8-cli orders find-by-id`** - Returns the Order record specified by OrderId. Currently NOT supported for scheduled orders

### products

The Products resource lets you fetch all the information required to complete an Order -- product descriptions, pricing, provisioning details, and the dependencies associated with each product

- **`pax8-cli products find-all`** - Returns a paginated list of Pax8 products filtered by optional query parameters
- **`pax8-cli products find-by-id`** - Returns only the product record for the productId you specify

### subscriptions

Subscriptions describe terms of service, including price and the billing start/end dates for a specified product/company combination. Resources let you update, cancel, and fetch details of existing subscriptions

- **`pax8-cli subscriptions delete`** - Cancels the Subscription specified by subscriptionId
- **`pax8-cli subscriptions find`** - Fetch a paginated list of subscriptions. Default page is 0 and default size is 10. The maximum page size is 200
- **`pax8-cli subscriptions find-by-id`** - Returns the Subscription record specified by the subscriptionId
- **`pax8-cli subscriptions update`** - Updates a subscription. Currently NOT supported for subscriptions with a future date. At least one of the following fields are required: Price, BillingTerm, Quantity, StartDate

### usage-summaries

Manage usage summaries

- **`pax8-cli usage-summaries <usageSummaryId>`** - Fetch a paginated list of usage summaries. Default page is 0 and default size is 10. The maximum page size is 200


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
pax8-cli companies get <companyId>

# JSON for scripting and agents
pax8-cli companies get <companyId> --json

# Filter to specific fields
pax8-cli companies get <companyId> --json --select id,name,status

# Dry run  -  show the request without sending
pax8-cli companies get <companyId> --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
pax8-cli companies get <companyId> --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
pax8-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/pax8-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `PAX8_CLIENT_ID` | per_call | Yes | Set to your API credential. |
| `PAX8_CLIENT_SECRET` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `pax8-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `pax8-cli doctor` to check credentials
- Verify the environment variable is set: `echo $PAX8_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Confirm PAX8_CLIENT_ID and PAX8_CLIENT_SECRET are set and run `pax8-cli doctor`; the token exchange needs the correct audience (PAX8_AUDIENCE).
- **doctor reports token exchange failed**  -  Re-create the API credential in the Pax8 portal (Integrations) and ensure the secret was copied exactly; secrets are shown only once.
- **reconcile or mrr returns empty**  -  Run `pax8-cli sync` first; analytics commands read the local store, not the live API.
- **list commands paginate slowly**  -  Use `pax8-cli sync` once, then query the offline store with `search`, `analytics`, and `--csv`.
