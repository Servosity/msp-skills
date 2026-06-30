---
name: maxio
description: "The first open, local revenue-intelligence CLI for Maxio Advanced Billing  -  MRR waterfalls, retention, and per-client history computed offline from a SQLite mirror that outlives Maxio's deprecated Insights API. Trigger phrases: `what's our MRR`, `show the MRR waterfall`, `recurring revenue for this customer`, `net revenue retention this year`, `what revenue needs attention`, `use maxio`, `run maxio`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Maxio"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - maxio-cli
    install:
      - kind: go
        bins: [maxio-cli]
        module: github.com/mvanhorn/printing-press-library/library/payments/maxio/cmd/maxio-cli
---

# Maxio  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `maxio-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install maxio --cli-only
   ```
2. Verify: `maxio-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/payments/maxio/cmd/maxio-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Every Maxio Advanced Billing resource is queryable offline, but the point is the revenue math no other tool computes: the five-bucket MRR movement waterfall, NRR/GRR/quick-ratio, per-client recurring-revenue history, and a 'what needs attention' triage rollup. It snapshots each sync into a local time series, so historic cohort and retention curves survive even as Maxio sunsets its Insights endpoints. Read-only by default.

## When to Use This CLI

Use this CLI when an agent or operator needs Maxio recurring-revenue intelligence from the terminal: current and historic MRR/ARR, the movement waterfall, retention and churn, per-client revenue, and a revenue triage list. It is the right tool when the answer requires joining or trending data across subscriptions, customers, components, and invoices, or when the Maxio dashboard is too slow/manual to query.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for GAAP recognized revenue, ASC 606, or deferred-revenue schedules  -  those live in SaaS Optics, a separate Maxio API not covered by this build.
- Do not use it to create, cancel, refund, or modify subscriptions or invoices against production  -  it is read-only by default and writes are intentionally gated.
- Do not use it for payment processing, collections, or dunning actions.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### History the live API can't reconstruct
- **`mrr waterfall`**  -  See MRR move month over month, broken into New, Expansion, Contraction, Churn, and Reactivation  -  plus ARR and growth rate.

  _Reach for this when an agent needs the recurring-revenue movement breakdown for a period instead of a single point-in-time number._

  ```bash
  maxio-cli mrr waterfall --since 2026-01-01 --group-by month --agent
  ```
- **`mrr client`**  -  Normalized MRR for one customer or the whole book, with the history the live API has no endpoint for.

  _Use this to answer 'what is recurring revenue for this account, and how has it trended' without scraping the dashboard._

  ```bash
  maxio-cli mrr client --customer 1234567 --since 2025-01-01 --agent
  ```
- **`retention`**  -  Net and gross revenue retention, logo churn, revenue churn, and quick ratio over any window.

  _Pick this when the question is about retention ratios over time rather than the raw movement dollars._

  ```bash
  maxio-cli retention --since 2025-01-01 --group-by month --agent
  ```
- **`cohort`**  -  Revenue and logo retention by signup-month cohort across N periods.

  _Use for cohort analysis; depth grows as the local snapshot history accumulates across syncs._

  ```bash
  maxio-cli cohort --by signup-month --periods 12 --agent
  ```

### Agent-native revenue ops
- **`triage`**  -  A ranked list of accounts that need attention: trending contraction, stalled expansion, recent churn, large auto-renewals approaching.

  _Reach for this to answer 'what revenue needs my attention this week' in one call instead of fanning out across endpoints._

  ```bash
  maxio-cli triage --limit 20 --agent
  ```
- **`reconcile`**  -  Per-subscription gaps between normalized MRR and the amounts actually billed, with the deltas flagged.

  _Use this to catch where normalized recurring revenue and invoiced amounts diverge (discounts, usage spikes, proration)._

  ```bash
  maxio-cli reconcile --since 2026-01-01 --agent
  ```
- **`usage-drivers`**  -  Which metered/usage components drove expansion versus contraction MRR, ranked.

  _Pick this when expansion or contraction needs to be attributed to specific usage-billed components._

  ```bash
  maxio-cli usage-drivers --since 2026-01-01 --limit 20 --agent
  ```

## Command Reference

**api-exports**  -  Manage api exports

- `maxio-cli api-exports export-invoices`  -  This API creates an invoices export and returns a batchjob object.
- `maxio-cli api-exports export-proforma-invoices`  -  This API creates a proforma invoices export and returns a batchjob object.
- `maxio-cli api-exports export-subscriptions`  -  This API creates a subscriptions export and returns a batchjob object.
- `maxio-cli api-exports list-exported-invoices`  -  This API returns an array of exported invoices for a provided `batch_id`.
- `maxio-cli api-exports list-exported-proforma-invoices`  -  This API returns an array of exported proforma invoices for a provided `batch_id`.
- `maxio-cli api-exports list-exported-subscriptions`  -  This API returns an array of exported subscriptions for a provided `batch_id`.
- `maxio-cli api-exports read-invoices-export`  -  This API returns a batchjob object for invoices export.
- `maxio-cli api-exports read-proforma-invoices-export`  -  This API returns a batchjob object for proforma invoices export.
- `maxio-cli api-exports read-subscriptions-export`  -  This API returns a batchjob object for subscriptions export.

**bank-accounts**  -  Manage bank accounts


**chargify-js-keys-json**  -  Manage chargify js keys json

- `maxio-cli chargify-js-keys-json`  -  Returns public keys used for Maxio.js (formerly Chargify.js).

**components**  -  Manage components

- `maxio-cli components find`  -  This request will return information regarding a component having the handle you provide.
- `maxio-cli components update`  -  This request will update a component. You may read the component by either the component's id or handle.

**components-json**  -  Manage components json

- `maxio-cli components-json`  -  This request will return a list of components for a site.

**components-price-points-json**  -  Manage components price points json

- `maxio-cli components-price-points-json`  -  This method allows to retrieve a list of Components Price Points belonging to a Site.

**coupons**  -  Manage coupons

- `maxio-cli coupons find`  -  You can search for a coupon via the API with the find method.
- `maxio-cli coupons validate`  -  You can verify if a specific coupon code is valid using the `validate` method.

**coupons-json**  -  Manage coupons json

- `maxio-cli coupons-json`  -  You can retrieve a list of coupons.

**credit-notes**  -  Manage credit notes

- `maxio-cli credit-notes <uid>`  -  Use this endpoint to retrieve the details for a credit note.

**credit-notes-json**  -  Manage credit notes json

- `maxio-cli credit-notes-json`  -  Credit Notes are like inverse invoices. They reduce the amount a customer owes.

**customers**  -  Manage customers

- `maxio-cli customers delete`  -  This method allows you to delete the Customer.
- `maxio-cli customers read`  -  Retrieves the Customer properties by Advanced Billing-generated Customer ID.
- `maxio-cli customers read-by-reference`  -  Use this method to return the customer object if you have the unique **Reference ID (Your App)** value handy.
- `maxio-cli customers update`  -  This method allows to update the Customer.

**customers-json**  -  Manage customers json

- `maxio-cli customers-json create-customer`  -  You may create a new Customer at any time, or you may create a Customer at the same time you create a Subscription.
- `maxio-cli customers-json list-customers`  -  This request will by default list all customers associated with your Site.

**endpoints**  -  Manage endpoints

- `maxio-cli endpoints <endpoint_id>`  -  Updates an Endpoint.

**endpoints-json**  -  Manage endpoints json

- `maxio-cli endpoints-json create-endpoint`  -  Creates an endpoint and assigns a list of webhook subscriptions (events) to it.
- `maxio-cli endpoints-json list-endpoints`  -  Returns created endpoints for a site.

**event-based-billing**  -  Manage event based billing

- `maxio-cli event-based-billing activate-event-based-component`  -  Activates an event-based component for a single subscription.
- `maxio-cli event-based-billing deactivate-event-based-component`  -  Deactivates an event-based component for a single subscription.

**events**  -  Manage events

- `maxio-cli events read-count`  -  Get a count of all the events for a given site by using this method.
- `maxio-cli events record`  -  Records a single event for Events-Based Billing.

**events-json**  -  Manage events json

- `maxio-cli events-json`  -  Advanced Billing Events include various activity that happens around a Site.

**invoices**  -  Manage invoices

- `maxio-cli invoices list-events`  -  This endpoint returns a list of invoice events.
- `maxio-cli invoices read`  -  Use this endpoint to retrieve the details for an invoice.
- `maxio-cli invoices record-payment-for-multiple`  -  This API call should be used when you want to record an external payment against multiple invoices.

**invoices-json**  -  Manage invoices json

- `maxio-cli invoices-json`  -  By default, invoices returned on the index will only include totals, not detailed breakdowns for `line_items`

**mrr-json**  -  Manage mrr json

- `maxio-cli mrr-json`  -  This endpoint returns your site's current MRR, including plan and usage breakouts.

**mrr-movements-json**  -  Manage mrr movements json

- `maxio-cli mrr-movements-json`  -  This endpoint returns your site's MRR movements.

**offers**  -  Manage offers

- `maxio-cli offers <offer_id>`  -  This method allows you to list a specific offer's attributes.

**offers-json**  -  Manage offers json

- `maxio-cli offers-json create-offer`  -  Create an offer within your Advanced Billing site by sending a POST request.
- `maxio-cli offers-json list-offers`  -  This endpoint will list offers for a site.

**one-time-tokens**  -  Manage one time tokens

- `maxio-cli one-time-tokens <chargify_token>`  -  One Time Tokens aka Advanced Billing Tokens house the credit card or ACH (Authorize.

**payment-profiles**  -  Manage payment profiles

- `maxio-cli payment-profiles delete-unused`  -  Deletes an unused payment profile.
- `maxio-cli payment-profiles read`  -  Using the GET method you can retrieve a Payment Profile identified by its unique ID.
- `maxio-cli payment-profiles update`  -  In the event that you are using the Authorize.

**payment-profiles-json**  -  Manage payment profiles json

- `maxio-cli payment-profiles-json create-payment-profile`  -  Creates a payment profile for a customer.
- `maxio-cli payment-profiles-json list-payment-profiles`  -  This method will return all of the active `payment_profiles` for a Site, or for one Customer within a site.

**portal**  -  Manage portal

- `maxio-cli portal enable-billing-for-customer`  -  Full documentation on how the Billing Portal operates within the Advanced Billing UI can be located [here](https
- `maxio-cli portal read-billing-link`  -  This method will provide to the API user the exact URL required for a subscriber to access the Billing Portal.
- `maxio-cli portal resend-billing-invitation`  -  You can resend a customer's Billing Portal invitation.
- `maxio-cli portal revoke-billing-access`  -  You can revoke a customer's Billing Portal invitation.

**price-points**  -  Manage price points


**product-families**  -  Manage product families

- `maxio-cli product-families <id>`  -  Retrieves a Product Family via the `product_family_id`. The response will contain a Product Family object.

**product-families-json**  -  Manage product families json

- `maxio-cli product-families-json create-product-family`  -  Creates a Product Family within your Advanced Billing site.
- `maxio-cli product-families-json list-product-families`  -  Retrieve a list of Product Families for a site.

**product-price-points**  -  Manage product price points


**products**  -  Manage products

- `maxio-cli products archive`  -  Archives the product.
- `maxio-cli products read`  -  Reads the current details of a product.
- `maxio-cli products read-by-handle`  -  Retrieves a Product object by its `api_handle`.
- `maxio-cli products update`  -  Updates aspects of an existing product.

**products-json**  -  Manage products json

- `maxio-cli products-json`  -  This method allows to retrieve a list of Products belonging to a Site.

**products-price-points-json**  -  Manage products price points json

- `maxio-cli products-price-points-json`  -  This method allows retrieval of a list of Products Price Points belonging to a Site.

**proforma-invoices**  -  Manage proforma invoices

- `maxio-cli proforma-invoices <proforma_invoice_uid>`  -  Use this endpoint to read the details of an existing proforma invoice.

**reason-codes**  -  Manage reason codes

- `maxio-cli reason-codes delete`  -  This method gives a merchant the option to delete one reason code from the Churn Reason Codes.
- `maxio-cli reason-codes read`  -  This method gives a merchant the option to retrieve a list of a particular code for a given Site by providing the
- `maxio-cli reason-codes update`  -  This method gives a merchant the option to update an existing reason code for a given site.

**reason-codes-json**  -  Manage reason codes json

- `maxio-cli reason-codes-json create-reason-code`  -  ReasonCodes are a way to gain a high level view of why your customers are cancelling the subscription to your product
- `maxio-cli reason-codes-json list-reason-codes`  -  This method gives a merchant the option to retrieve a list of all of the current churn codes for a given site.

**referral-codes**  -  Manage referral codes

- `maxio-cli referral-codes`  -  Use this method to determine if the referral code is valid and applicable within your Site.

**sellers**  -  Manage sellers


**site-json**  -  Manage site json

- `maxio-cli site-json`  -  Retrieves site data. Full documentation on Sites in the Advanced Billing UI can be located [here](https://maxio.zendesk.

**sites**  -  Manage sites

- `maxio-cli sites`  -  Clears all data from a test site asynchronously.

**stats-json**  -  Manage stats json

- `maxio-cli stats-json`  -  The Stats API is a very basic view of some Site-level stats. This API call only answers with JSON responses.

**subscription-groups**  -  Manage subscription groups

- `maxio-cli subscription-groups delete`  -  Deletes a subscription group. Only groups without members can be deleted.
- `maxio-cli subscription-groups find`  -  Finds the subscription group associated with a subscription.
- `maxio-cli subscription-groups read`  -  Returns subscription group details.
- `maxio-cli subscription-groups signup-with`  -  Creates multiple subscriptions at once under the same customer and consolidates them into a subscription group.
- `maxio-cli subscription-groups update-members`  -  Updates subscription group members.

**subscription-groups-json**  -  Manage subscription groups json

- `maxio-cli subscription-groups-json create-subscription-group`  -  Creates a subscription group with given members.
- `maxio-cli subscription-groups-json list-subscription-groups`  -  Returns an array of subscription groups for the site.

**subscriptions**  -  Manage subscriptions

- `maxio-cli subscriptions cancel`  -  Cancels the Subscription. The Delete method sets the Subscription state to `canceled`.
- `maxio-cli subscriptions create-signup-proforma-invoice`  -  This endpoint is only available for Relationship Invoicing sites.
- `maxio-cli subscriptions find`  -  Finds a subscription by its reference.
- `maxio-cli subscriptions preview`  -  Previews a subscription by POSTing the same JSON or XML as for a subscription creation.
- `maxio-cli subscriptions preview-signup-proforma-invoice`  -  This endpoint is only available for Relationship Invoicing sites.
- `maxio-cli subscriptions read`  -  Retrieves subscription details.
- `maxio-cli subscriptions update`  -  Updates one or more attributes of a subscription.

**subscriptions-components-json**  -  Manage subscriptions components json

- `maxio-cli subscriptions-components-json`  -  Lists components applied to each subscription.

**subscriptions-json**  -  Manage subscriptions json

- `maxio-cli subscriptions-json create-subscription`  -  Creates a Subscription for a customer and product. Specify the product with `product_id` or `product_handle`.
- `maxio-cli subscriptions-json list-subscriptions`  -  Returns an array of subscriptions from a Site.

**subscriptions-mrr-json**  -  Manage subscriptions mrr json

- `maxio-cli subscriptions-mrr-json`  -  This endpoint returns your site's current MRR, including plan and usage breakouts split per subscription.

**webhooks**  -  Manage webhooks

- `maxio-cli webhooks enable`  -  Enables webhooks for your site.
- `maxio-cli webhooks replay`  -  Replays webhooks. Posting to this endpoint does not immediately resend the webhooks.

**webhooks-json**  -  Manage webhooks json

- `maxio-cli webhooks-json`  -  Retrieves a list of webhooks. You can pass query parameters if you want to filter webhooks.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
maxio-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Board-prep MRR movement

```bash
maxio-cli mrr waterfall --since 2026-01-01 --group-by month --agent
```

The month-by-month New/Expansion/Contraction/Churn/Reactivation breakdown, agent-formatted for a deck.

### Narrow a deeply-nested subscription pull

```bash
maxio-cli subscriptions-json list-subscriptions --agent --select subscription.product.name,subscription.state,subscription.current_period_ends_at
```

Use --agent with --select dotted paths so the agent reads only the fields it needs instead of the full nested subscription payload.

### Find accounts that need attention

```bash
maxio-cli triage --limit 20 --agent
```

Ranked accounts with trending contraction, stalled expansion, recent churn, or large approaching renewals.

### Reconcile one client's normalized vs billed

```bash
maxio-cli reconcile --customer 1234567 --since 2026-01-01 --agent
```

Per-subscription gaps between normalized MRR and what was actually invoiced for that client.

## Auth Setup

Maxio Advanced Billing uses HTTP Basic auth where the username is your API key and the password is the literal 'x'. Set one secret, MAXIO_API_KEY (your Advanced Billing API key from Config -> Integrations -> API Keys), plus MAXIO_SITE (your subdomain, e.g. acme for acme.chargify.com). The CLI supplies the constant 'x' password for you. It never issues writes by default.

Run `maxio-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  maxio-cli api-exports export-invoices --agent --select id,name,status
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
maxio-cli feedback "the --since flag is inclusive but docs say exclusive"
maxio-cli feedback --stdin < notes.txt
maxio-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/maxio-cli/feedback.jsonl`. They are never POSTed unless `MAXIO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `MAXIO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
maxio-cli profile save briefing --json
maxio-cli --profile briefing api-exports export-invoices
maxio-cli profile list --json
maxio-cli profile show briefing
maxio-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `maxio-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/payments/maxio/cmd/maxio-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add maxio-mcp -- maxio-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which maxio-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   maxio-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `maxio-cli <command> --help`.
