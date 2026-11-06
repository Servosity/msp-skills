---
name: maxio
description: "Open, local revenue-intelligence CLI for Maxio Advanced Billing  -  MRR waterfalls, retention, and per-client history computed offline from a SQLite mirror, so the trended history survives even though the live API returns only point-in-time figures. Trigger phrases: `what's our MRR`, `show the MRR waterfall`, `recurring revenue for this customer`, `net revenue retention this year`, `what revenue needs attention`, `use maxio`, `run maxio`."
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
      - kind: script
        bins: [maxio-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/maxio/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/maxio/install.ps1
---

# Maxio - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `maxio-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/maxio/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/maxio/install.ps1 | iex
   ```
3. Verify: `maxio-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `maxio-cli` and `maxio-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Every Maxio Advanced Billing resource is queryable offline, but the point is the revenue math no other tool computes: the five-bucket MRR movement waterfall, NRR/GRR/quick-ratio, per-client recurring-revenue history, and a 'what needs attention' triage rollup. It snapshots each sync into a local time series, so historic cohort and retention curves accrue even though the live API has no endpoint to reconstruct them after the fact. Reads are the safe default; mutating commands execute only when you invoke them.

## When to Use This CLI

Use this CLI when an agent or operator needs Maxio recurring-revenue intelligence from the terminal: current and historic MRR/ARR, the movement waterfall, retention and churn, per-client revenue, and a revenue triage list. It is the right tool when the answer requires joining or trending data across subscriptions, customers, components, and invoices, or when the Maxio dashboard is too slow/manual to query.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for GAAP recognized revenue, ASC 606, or deferred-revenue schedules  -  those live in SaaS Optics, a separate Maxio API not covered by this build.
- Do not point an autonomous agent at create, cancel, refund, or modify operations against production without a human in the loop  -  these are real writes that execute when invoked; preview them with --dry-run and require approval (see governance.md).
- Do not use it for payment processing, collections, or dunning actions.

## Unique Capabilities

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
- **`mrr now`**  -  Current MRR + ARR + growth vs the prior snapshot.

  _Use this for the one-line answer to 'what is our MRR right now and is it growing'._

  ```bash
  maxio-cli mrr now --json
  ```
- **`mrr sync`**  -  Snapshot current MRR and backfill movement history into the local store.

  _Run it alongside sync so the revenue commands have snapshots and movements to compute on._

  ```bash
  maxio-cli mrr sync
  ```

### Agent-native revenue ops
- **`triage`**  -  A ranked list of accounts that need attention: past-due, large upcoming renewals, and high-value concentration.

  _Reach for this to answer 'what revenue needs my attention this week' in one call instead of fanning out across endpoints._

  ```bash
  maxio-cli triage --limit 20 --agent
  ```
- **`reconcile`**  -  Per-customer gaps between normalized MRR and the amounts actually invoiced, with the deltas flagged.

  _Use this to catch where normalized recurring revenue and invoiced amounts diverge (discounts, usage spikes, proration)._

  ```bash
  maxio-cli reconcile --since 2026-01-01 --agent
  ```
- **`usage-drivers`**  -  Which metered/usage components drove expansion versus contraction MRR, ranked.

  _Pick this when expansion or contraction needs to be attributed to specific usage-billed components._

  ```bash
  maxio-cli usage-drivers --since 2026-01-01 --limit 20 --agent
  ```
- **`arpa`**  -  ARPA/ARPU, MRR percentiles, and price-vs-volume decomposition of MRR change.

  _Pick this when the question is whether revenue moved because of price or because of customer count._

  ```bash
  maxio-cli arpa --json
  ```
- **`churn`**  -  Logo and revenue churn per period, voluntary vs involuntary, by reason code.

  _Use this to see who churned, why, and how much recurring revenue left with them._

  ```bash
  maxio-cli churn --since 2026-01-01 --json
  ```
- **`concentration`**  -  Customer concentration: share of ARR held by the top 1 / 5 / 10 customers.

  _Reach for this when the question is how dependent revenue is on a handful of accounts._

  ```bash
  maxio-cli concentration --limit 20 --json
  ```
- **`new-customers`**  -  New logos acquired per period (first-activation month) plus the new MRR they brought.

  _Use this to answer how many new accounts were won per month and what they added._

  ```bash
  maxio-cli new-customers --since 2026-01-01 --json
  ```
- **`renewals`**  -  Active subscriptions renewing within the next N days (default 90), with MRR at risk.

  _Pick this to see which renewals are coming up and how much recurring revenue rides on them._

  ```bash
  maxio-cli renewals --within 30 --json
  ```
- **`revenue-mix`**  -  Recurring vs usage vs services split of the latest month's billed line items.

  _Use this when the question is how much of billed revenue is recurring versus usage or services._

  ```bash
  maxio-cli revenue-mix --month 2026-05 --json
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

- `maxio-cli portal enable-billing-for-customer`  -  Full documentation on how the Billing Portal operates within the Advanced Billing UI can be located in the Maxio docs.
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


**arpa**


- `maxio-cli arpa`  -  ARPA/ARPU, MRR percentiles, and price-vs-volume decomposition of MRR change.


**churn**


- `maxio-cli churn`  -  Logo and revenue churn per period, voluntary vs involuntary, by reason code.


**cohort**


- `maxio-cli cohort`  -  Revenue and logo retention by signup-month cohort across N periods.


**concentration**


- `maxio-cli concentration`  -  Customer concentration: share of ARR held by the top 1 / 5 / 10 customers.


**mrr**


- `maxio-cli mrr waterfall`  -  See MRR move month over month, broken into New, Expansion, Contraction, Churn, and Reactivation  -  plus ARR and growth rate.

- `maxio-cli mrr client`  -  Normalized MRR for one customer or the whole book, with the history the live API has no endpoint for.

- `maxio-cli mrr now`  -  Current MRR + ARR + growth vs the prior snapshot.

- `maxio-cli mrr sync`  -  Snapshot current MRR and backfill movement history into the local store.


**new-customers**


- `maxio-cli new-customers`  -  New logos acquired per period (first-activation month) plus the new MRR they brought.


**reconcile**


- `maxio-cli reconcile`  -  Per-customer gaps between normalized MRR and the amounts actually invoiced, with the deltas flagged.


**renewals**


- `maxio-cli renewals`  -  Active subscriptions renewing within the next N days (default 90), with MRR at risk.


**retention**


- `maxio-cli retention`  -  Net and gross revenue retention, logo churn, revenue churn, and quick ratio over any window.


**revenue-mix**


- `maxio-cli revenue-mix`  -  Recurring vs usage vs services split of the latest month's billed line items.


**triage**


- `maxio-cli triage`  -  A ranked list of accounts that need attention: past-due, large upcoming renewals, and high-value concentration.


**usage-drivers**


- `maxio-cli usage-drivers`  -  Which metered/usage components drove expansion versus contraction MRR, ranked.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
maxio-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

Ranked accounts that need attention: past-due, large upcoming renewals, and high-value concentration.

### Reconcile one client's normalized vs billed

```bash
maxio-cli reconcile --customer 1234567 --since 2026-01-01 --agent
```

Per-customer gaps between normalized MRR and what was actually invoiced for that client.

## Auth Setup

Maxio Advanced Billing uses HTTP Basic authentication: your API key (Config -> Integrations -> API Keys) is the username and the password is the literal x, which the CLI supplies. Set MAXIO_API_KEY to your API key and MAXIO_SITE to your subdomain (acme for acme.chargify.com). The MAXIO_USERNAME (the key) and MAXIO_PASSWORD (x) pair from earlier versions still works; MAXIO_API_KEY wins when both are set. Read commands are safe to run by default; mutating commands execute when you invoke them (some prompt for confirmation interactively, which --agent/--yes skips), so preview with --dry-run and approve writes per your agent policy.

Run `maxio-cli doctor` to verify setup.

## Credential safety

- The API key travels only as HTTP Basic (`<key>:x`) and only to the scheme, host and port of the configured base URL (`https://<site>.chargify.com` built from `MAXIO_SITE`, or `MAXIO_BASE_URL`). The one other host is Maxio's Events-Based Billing ingestion endpoint, `https://events.chargify.com/<the same site>/events/...`, used by `events record` and `bulk-record-events`. A redirect to any other host, a plain http address and a site that is not a single host-name label are all refused, so the key never reaches them. `MAXIO_SITE` has no default: with it unset, a command names the variable and sends nothing.
- A response's card, bank and token fields (a vault token, a self-service page token, a webhook signature, a Maxio.js `chargify_token`) are replaced with `[redacted]` as the response arrives, so ordinary lists still work and none of those values reach the output or the local store.
- Any other output, file or MCP result that would repeat `MAXIO_API_KEY`, the stored key, or a credential you typed (a card number or CVV, a bank account number or IBAN, a `chargify_token`, a payment nonce) is withheld as a whole and prints `[credkit: withheld from <where>: it carried a credential this program holds]` in its place, including in the common encodings.
- Responses are never cached on disk: every command reads the API's current answer.
- A saved key lives only in `credentials.toml`, or in a `config.toml` you put it in, and both are kept at mode 0600. `auth set-token` reads the key from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, `maxio_execute` (every Maxio endpoint that changes something) and `import` return a preview and send only with `confirm: true`, and a tool argument can never name a file to read or write.

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
  maxio-cli api-exports list-exported-invoices mock-value --agent --select avatax_details,billing_address,collection_method
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
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

- Use `--home <dir>` for one invocation, or set `MAXIO_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `MAXIO_CONFIG_DIR`, `MAXIO_DATA_DIR`, `MAXIO_STATE_DIR`, `MAXIO_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `MAXIO_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `maxio-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "maxio": {
        "command": "maxio-mcp",
        "env": {
          "MAXIO_HOME": "/srv/maxio"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `MAXIO_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `MAXIO_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `MAXIO_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `MAXIO_DATA_SOURCE` sets the `--data-source` default.

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
maxio-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "maxio-cli learnings confirm 12"] }
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
       materially more, record the divergence via `maxio-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `maxio-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `maxio-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
maxio-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
maxio-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
maxio-cli teach-playbook \
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
maxio-cli playbook amend \
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

`maxio-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `MAXIO_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
maxio-cli feedback "the --since flag is inclusive but docs say exclusive"
maxio-cli feedback --stdin < notes.txt
maxio-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `MAXIO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `MAXIO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
maxio-cli profile save briefing --json
maxio-cli --profile briefing api-exports list-exported-invoices mock-value
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
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `maxio-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add maxio-mcp -- maxio-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `maxio-mcp --transport http` requires `MAXIO_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `MAXIO_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `MAXIO_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which maxio-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   maxio-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `maxio-cli <command> --help`.
