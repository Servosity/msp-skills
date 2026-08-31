# Maxio CLI

**Open, local revenue-intelligence CLI for Maxio Advanced Billing  -  MRR waterfalls, retention, and per-client history computed offline from a SQLite mirror, so the trended history survives even though the live API returns only point-in-time figures.**

Every Maxio Advanced Billing resource is queryable offline, but the point is the revenue math no other tool computes: the five-bucket MRR movement waterfall, NRR/GRR/quick-ratio, per-client recurring-revenue history, and a 'what needs attention' triage rollup. It snapshots each sync into a local time series, so historic cohort and retention curves accrue even though the live API has no endpoint to reconstruct them after the fact. Reads are the safe default; mutating commands execute only when you invoke them.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `maxio-cli` and `maxio-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=maxio). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/maxio --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/maxio --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `maxio-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the maxio skill from https://github.com/Servosity/msp-skills/tree/main/skills/maxio. The skill defines how its required CLI (`maxio-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=maxio).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `MAXIO_USERNAME` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. A bundle carries the five platform binaries the builder downloads - macOS (`darwin-arm64`, `darwin-amd64`), Linux (`linux-arm64`, `linux-amd64`) and Windows (`windows-amd64`). Windows on ARM is released as a standalone binary but is not bundled, so use the manual config below there.

> **Interim note:** check any `.mcpb` bundle before you trust it ([#287](https://github.com/Servosity/msp-skills/issues/287)). Its `manifest.json` launches `${__dirname}/bin/maxio-pp-mcp`, while the builder stores the release binaries in `bin/` under their platform-suffixed names - `maxio-mcp-darwin-arm64`, `-darwin-amd64`, `-linux-arm64`, `-linux-amd64`, `-windows-amd64.exe`. Run `unzip -l <file>.mcpb | grep bin/`: if the name the manifest launches is not among them, Claude Desktop has nothing to run - use the installer above and the manual JSON config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/maxio/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/maxio/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "maxio": {
      "command": "maxio-mcp",
      "env": {
        "MAXIO_SITE": "<site>",
        "MAXIO_USERNAME": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Maxio Advanced Billing uses HTTP Basic authentication. Set three environment variables: MAXIO_USERNAME, MAXIO_PASSWORD, and MAXIO_SITE (your subdomain, e.g. acme for acme.chargify.com). For Advanced Billing API-key access, MAXIO_USERNAME is your API key (Config -> Integrations -> API Keys) and MAXIO_PASSWORD is the literal value 'x'. Read commands are safe to run by default; mutating commands execute when you invoke them (some prompt for confirmation interactively, which --agent/--yes skips), so preview with --dry-run and approve writes per your agent policy.

## Quick Start

```bash
# confirm config + auth shape before hitting the API
maxio-cli doctor --dry-run

# mirror the billing surface (customers, subscriptions, invoices, products, components) into local SQLite
maxio-cli sync --full

# snapshot MRR + backfill the movement history the revenue commands compute on
maxio-cli mrr sync

# current MRR + ARR + growth in one line
maxio-cli mrr now

# the movement breakdown driving that number
maxio-cli mrr waterfall --since 2026-01-01 --group-by month

# recurring revenue for one account, current + historic
maxio-cli mrr client --customer 1234567

# NRR / GRR / churn / quick ratio over the window
maxio-cli retention --since 2025-01-01

```

## Unique Features

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

## Usage

Run `maxio-cli --help` for the full command reference and flag list.

## Commands

### api-exports

Manage api exports

- **`maxio-cli api-exports export-invoices`** - This API creates an invoices export and returns a batchjob object.
- **`maxio-cli api-exports export-proforma-invoices`** - This API creates a proforma invoices export and returns a batchjob object.

It is only available for Relationship Invoicing architecture.
- **`maxio-cli api-exports export-subscriptions`** - This API creates a subscriptions export and returns a batchjob object.
- **`maxio-cli api-exports list-exported-invoices`** - This API returns an array of exported invoices for a provided `batch_id`. Pay close attention to pagination in order to control responses from the server.

Example: `GET https://{subdomain}.chargify.com/api_exports/invoices/123/rows?per_page=10000&page=1`.
- **`maxio-cli api-exports list-exported-proforma-invoices`** - This API returns an array of exported proforma invoices for a provided `batch_id`. Pay close attention to pagination in order to control responses from the server.

Example: `GET https://{subdomain}.chargify.com/api_exports/proforma_invoices/123/rows?per_page=10000&page=1`.
- **`maxio-cli api-exports list-exported-subscriptions`** - This API returns an array of exported subscriptions for a provided `batch_id`. Pay close attention to pagination in order to control responses from the server.

Example: `GET https://{subdomain}.chargify.com/api_exports/subscriptions/123/rows?per_page=200&page=1`.
- **`maxio-cli api-exports read-invoices-export`** - This API returns a batchjob object for invoices export.
- **`maxio-cli api-exports read-proforma-invoices-export`** - This API returns a batchjob object for proforma invoices export.
- **`maxio-cli api-exports read-subscriptions-export`** - This API returns a batchjob object for subscriptions export.

### bank-accounts

Manage bank accounts


### chargify-js-keys-json

Manage chargify js keys json

- **`maxio-cli chargify-js-keys-json`** - Returns public keys used for Maxio.js (formerly Chargify.js).

### components

Manage components

- **`maxio-cli components find`** - This request will return information regarding a component having the handle you provide. You can identify your components with a handle so you don't have to save or reference the IDs we generate.
- **`maxio-cli components update`** - This request will update a component.

You may read the component by either the component's id or handle. When using the handle, it must be prefixed with `handle:`.

### components-json

Manage components json

- **`maxio-cli components-json`** - This request will return a list of components for a site.

### components-price-points-json

Manage components price points json

- **`maxio-cli components-price-points-json`** - This method allows to retrieve a list of Components Price Points belonging to a Site.

### coupons

Manage coupons

- **`maxio-cli coupons find`** - You can search for a coupon via the API with the find method. By passing a code parameter, the find will attempt to locate a coupon that matches that code. If no coupon is found, a 404 is returned.

If you have more than one product family and if the coupon you are trying to find does not belong to the default product family in your site, then you will need to specify (either in the url or as a query string param) the product family id.
- **`maxio-cli coupons validate`** - You can verify if a specific coupon code is valid using the `validate` method. This method is useful for validating coupon codes that are entered by a customer. If the coupon is found and is valid, the coupon will be returned with a 200 status code.

If the coupon is invalid, the status code will be 404 and the response will say why it is invalid. If the coupon is valid, the status code will be 200 and the coupon will be returned. The following reasons for invalidity are supported:

+ Coupon not found
+ Coupon is invalid
+ Coupon expired

If you have more than one product family and if the coupon you are validating does not belong to the first product family in your site, then you will need to specify the product family, either in the url or as a query string param. This can be done by supplying the id or the handle in the `handle:my-family` format.

Eg.

```
https://<subdomain>.chargify.com/product_families/handle:<product_family_handle>/coupons/validate.<format>?code=<coupon_code>
```

Or:

```
https://<subdomain>.chargify.com/coupons/validate.<format>?code=<coupon_code>&product_family_id=<id>
```

### coupons-json

Manage coupons json

- **`maxio-cli coupons-json`** - You can retrieve a list of coupons.

### credit-notes

Manage credit notes

- **`maxio-cli credit-notes <uid>`** - Use this endpoint to retrieve the details for a credit note.

### credit-notes-json

Manage credit notes json

- **`maxio-cli credit-notes-json`** - Credit Notes are like inverse invoices. They reduce the amount a customer owes.

By default, the credit notes returned by this endpoint will exclude the arrays of `line_items`, `discounts`, `taxes`, `applications`, or `refunds`. To include these arrays, pass the specific field as a key in the query with a value set to `true`.

### customers

Manage customers

- **`maxio-cli customers delete`** - This method allows you to delete the Customer.
- **`maxio-cli customers read`** - Retrieves the Customer properties by Advanced Billing-generated Customer ID.
- **`maxio-cli customers read-by-reference`** - Use this method to return the customer object if you have the unique **Reference ID (Your App)** value handy. It will return a single match.
- **`maxio-cli customers update`** - This method allows to update the Customer.

### customers-json

Manage customers json

- **`maxio-cli customers-json create-customer`** - You may create a new Customer at any time, or you may create a Customer at the same time you create a Subscription. The only validation restriction is that you may only create one customer for a given reference value.

If provided, the `reference` value must be unique. It represents a unique identifier for the customer from your own app, i.e. the customer’s ID. This allows you to retrieve a given customer via a piece of shared information. Alternatively, you may choose to leave `reference` blank, and store Advanced Billing’s unique ID for the customer, which is in the `id` attribute.

Full documentation on how to locate, create and edit Customers in the Advanced Billing UI can be located [here](https://maxio.zendesk.com/hc/en-us/articles/24252190590093-Customer-Details).

## Required Country Format

Advanced Billing requires that you use the ISO Standard Country codes when formatting country attribute of the customer.

Countries should be formatted as 2 characters. For more information, see the following wikipedia article on [ISO_3166-1.](http://en.wikipedia.org/wiki/ISO_3166-1#Current_codes)

## Required State Format

Advanced Billing requires that you use the ISO Standard State codes when formatting state attribute of the customer.

+ US States (2 characters): [ISO_3166-2](https://en.wikipedia.org/wiki/ISO_3166-2:US)

+ States Outside the US (2-3 characters): To find the correct state codes outside of the US, go to [ISO_3166-1](http://en.wikipedia.org/wiki/ISO_3166-1#Current_codes) and click on the link in the “ISO 3166-2 codes” column next to country you wish to populate.

## Locale

Advanced Billing allows you to attribute a language/region to your customer to deliver invoices in any required language.
For more: [Customer Locale](https://maxio.zendesk.com/hc/en-us/articles/24286672013709-Customer-Locale)
- **`maxio-cli customers-json list-customers`** - This request will by default list all customers associated with your Site.

## Find Customer

Use the search feature with the `q` query parameter to retrieve an array of customers that matches the search query.

Common use cases are:

+ Search by an email
+ Search by an Advanced Billing ID
+ Search by an organization
+ Search by a reference value from your application
+ Search by a first or last name

To retrieve a single, exact match by reference, use the [lookup endpoint](https://developers.chargify.com/docs/api-docs/b710d8fbef104-read-customer-by-reference).

### endpoints

Manage endpoints

- **`maxio-cli endpoints <endpoint_id>`** - Updates an Endpoint. You can change the `url` of your endpoint or the list of `webhook_subscriptions` to which you are subscribed. See the Webhooks Reference page for available events.

Always send a complete list of events to which you want to subscribe. Sending a PUT request for an existing endpoint with an empty list of `webhook_subscriptions` will unsubscribe all events.

If you want to unsubscribe from a specific event, send a list of `webhook_subscriptions` without the specific event key.

### endpoints-json

Manage endpoints json

- **`maxio-cli endpoints-json create-endpoint`** - Creates an endpoint and assigns a list of webhook subscriptions (events) to it.
See the Webhooks Reference page for available events.
- **`maxio-cli endpoints-json list-endpoints`** - Returns created endpoints for a site.

### event-based-billing

Manage event based billing

- **`maxio-cli event-based-billing activate-event-based-component`** - Activates an event-based component for a single subscription.

In order to bill your subscribers on your Events data under the Events-Based Billing feature, the components must be activated for the subscriber.

Learn more about the role of activation in the [Events-Based Billing docs](https://maxio.zendesk.com/hc/en-us/articles/24260323329805-Events-Based-Billing-Overview).

Use this endpoint to activate an event-based component for a single subscription. Activating an event-based component causes Advanced Billing to bill for events when the subscription is renewed.

*Note: it is possible to stream events for a subscription at any time, regardless of component activation status. The activation status only determines if the subscription should be billed for event-based component usage at renewal.*
- **`maxio-cli event-based-billing deactivate-event-based-component`** - Deactivates an event-based component for a single subscription. Deactivating the event-based component causes Advanced Billing to ignore related events at subscription renewal.

### events

Manage events

- **`maxio-cli events read-count`** - Get a count of all the events for a given site by using this method.
- **`maxio-cli events record`** - Records a single event for Events-Based Billing.

## Documentation

Events-Based Billing is an evolved form of metered billing that is based on data-rich events streamed in real-time from your system to Advanced Billing.

These events can then be transformed, enriched, or analyzed to form the computed totals of usage charges billed to your customers.

This API allows you to stream events into the Advanced Billing data ingestion engine.

Learn more about the feature in general in the [Events-Based Billing help docs](https://maxio.zendesk.com/hc/en-us/articles/24260323329805-Events-Based-Billing-Overview).

## Record Event

Use this endpoint to record a single event.

*Note: this endpoint differs from the standard Chargify API endpoints in that the URL subdomain will be `events` and your site subdomain will be included in the URL path. For example:*

```
https://events.chargify.com/my-site-subdomain/events/my-stream-api-handle
```

### events-json

Manage events json

- **`maxio-cli events-json`** - ## Events Intro

Advanced Billing Events include various activity that happens around a Site. This information is **especially** useful to track down issues that arise when subscriptions are not created due to errors.

Within the Advanced Billing UI, "Events" are referred to as "Site Activity".  Full documentation on how to record view Events / Site Activty in the Advanced Billing UI can be located [here](https://maxio.zendesk.com/hc/en-us/articles/24250671733517-Site-Activity).

## List Events for a Site

This method will retrieve a list of events for a site. Use query string filters to narrow down results. You may use the `key` filter as part of your query string to narrow down results.

### Legacy Filters

The following keys are no longer supported.

+ `payment_failure_recreated`
+ `payment_success_recreated`
+ `renewal_failure_recreated`
+ `renewal_success_recreated`
+ `zferral_revenue_post_failure` - (Specific to the deprecated Zferral integration)
+ `zferral_revenue_post_success` - (Specific to the deprecated Zferral integration)

## Event Key
The event type is identified by the key property. You can check supported keys in the Event Key reference.

## Event Specific Data

Different event types may include additional data in `event_specific_data` property.
While some events share the same schema for `event_specific_data`, others may not include it at all.
For precise mappings from key to event_specific_data, refer to the Event reference.

### Example
Here’s an example event for the `subscription_product_change` event:

```
{
    "event": {
        "id": 351,
        "key": "subscription_product_change",
        "message": "Product changed on Marky Mark's subscription from 'Basic' to 'Pro'",
        "subscription_id": 205,
        "event_specific_data": {
            "new_product_id": 3,
            "previous_product_id": 2
        },
        "created_at": "2012-01-30T10:43:31-05:00"
    }
}
```

Here’s an example event for the `subscription_state_change` event:

```
 {
     "event": {
         "id": 353,
         "key": "subscription_state_change",
         "message": "State changed on Marky Mark's subscription to Pro from trialing to active",
         "subscription_id": 205,
         "event_specific_data": {
             "new_subscription_state": "active",
             "previous_subscription_state": "trialing"
         },
         "created_at": "2012-01-30T10:43:33-05:00"
     }
 }
```

### invoices

Manage invoices

- **`maxio-cli invoices list-events`** - This endpoint returns a list of invoice events. Each event contains event "data" (such as an applied payment) as well as a snapshot of the `invoice` at the time of event completion.

Exposed event types are:

+ issue_invoice
+ apply_credit_note
+ apply_payment
+ refund_invoice
+ void_invoice
+ void_remainder
+ backport_invoice
+ change_invoice_status
+ change_invoice_collection_method
+ remove_payment
+ failed_payment
+ apply_debit_note
+ create_debit_note
+ change_chargeback_status

Invoice events are returned in ascending order.

If both a `since_date` and `since_id` are provided in request parameters, the `since_date` will be used.

Note - invoice events that occurred prior to 09/05/2018 __will not__ contain an `invoice` snapshot.
- **`maxio-cli invoices read`** - Use this endpoint to retrieve the details for an invoice.

## PDF Invoice retrieval

Individual PDF Invoices can be retrieved by using the "Accept" header application/pdf or appending .pdf as the format portion of the URL:
```curl -u <api_key>:x -H
Accept:application/pdf -H
https://acme.chargify.com/invoices/inv_8gd8tdhtd3hgr.pdf > output_file.pdf
URL: `https://<subdomain>.chargify.com/invoices/<uid>.<format>`
Method: GET
Required parameters: `uid`
Response: A single Invoice.
```
- **`maxio-cli invoices record-payment-for-multiple`** - This API call should be used when you want to record an external payment against multiple invoices.

 To apply a payment to multiple invoices, at minimum, specify the `amount` and `applications` (i.e., `invoice_uid` and `amount`) details.

```
{
  "payment": {
    "memo": "to pay the bills",
    "details": "check number 8675309",
    "method": "check",
    "amount": "250.00",
    "applications": [
      {
        "invoice_uid": "inv_8gk5bwkct3gqt",
        "amount": "100.00"
      },
      {
        "invoice_uid": "inv_7bc6bwkct3lyt",
        "amount": "150.00"
      }
    ]
  }
}
```

Note that the invoice payment amounts must be greater than 0. Total amount must be greater or equal to invoices payment amount sum.

### invoices-json

Manage invoices json

- **`maxio-cli invoices-json`** - By default, invoices returned on the index will only include totals, not detailed breakdowns for `line_items`, `discounts`, `taxes`, `credits`, `payments`, `custom_fields`, or `refunds`. To include breakdowns, pass the specific field as a key in the query with a value set to `true`.

### mrr-json

Manage mrr json

- **`maxio-cli mrr-json`** - This endpoint returns your site's current MRR, including plan and usage breakouts.

### mrr-movements-json

Manage mrr movements json

- **`maxio-cli mrr-movements-json`** - This endpoint returns your site's MRR movements.

## Understanding MRR movements

This endpoint will aid in accessing your site's [MRR Report](https://maxio.zendesk.com/hc/en-us/articles/24285894587021-MRR-Analytics) data.

Whenever a subscription event occurs that causes your site's MRR to change (such as a signup or upgrade), we record an MRR movement. These records are accessible via the MRR Movements endpoint.

Each MRR Movement belongs to a subscription and contains a timestamp, category, and an amount. `line_items` represent the subscription's product configuration at the time of the movement.

### Plan & Usage Breakouts

In the MRR Report UI, we support a setting to [include or exclude](https://maxio.zendesk.com/hc/en-us/articles/24285894587021-MRR-Analytics#displaying-component-based-metered-usage-in-mrr) usage revenue. In the MRR APIs, responses include `plan` and `usage` breakouts.

Plan includes revenue from:
* Products
* Quantity-Based Components
* On/Off Components

Usage includes revenue from:
* Metered Components
* Prepaid Usage Components

### offers

Manage offers

- **`maxio-cli offers <offer_id>`** - This method allows you to list a specific offer's attributes. This is different than list all offers for a site, as it requires an `offer_id`.

### offers-json

Manage offers json

- **`maxio-cli offers-json create-offer`** - Create an offer within your Advanced Billing site by sending a POST request.

## Documentation

Offers allow you to package complicated combinations of products, components and coupons into a convenient package which can then be subscribed to just like products.

Once an offer is defined it can be used as an alternative to the product when creating subscriptions.

Full documentation on how to use offers in the Advanced Billing UI can be located [here](https://maxio.zendesk.com/hc/en-us/articles/24261295098637-Offers-Overview).

## Using a Product Price Point

You can optionally pass in a `product_price_point_id` that corresponds with the `product_id` and the offer will use that price point. If a `product_price_point_id` is not passed in, the product's default price point will be used.
- **`maxio-cli offers-json list-offers`** - This endpoint will list offers for a site.

### one-time-tokens

Manage one time tokens

- **`maxio-cli one-time-tokens <chargify_token>`** - One Time Tokens aka Advanced Billing Tokens house the credit card or ACH (Authorize.Net or Stripe only) data for a customer.

You can use One Time Tokens while creating a subscription or payment profile instead of passing all bank account or credit card data directly to a given API endpoint.

To obtain a One Time Token you have to use [Chargify.js](https://docs.maxio.com/hc/en-us/articles/38163190843789-Chargify-js-Overview#chargify-js-overview-0-0).

### payment-profiles

Manage payment profiles

- **`maxio-cli payment-profiles delete-unused`** - Deletes an unused payment profile.

If the payment profile is in use by one or more subscriptions or groups, a 422 and error message will be returned.
- **`maxio-cli payment-profiles read`** - Using the GET method you can retrieve a Payment Profile identified by its unique ID.

Note that a different JSON object will be returned if the card method on file is a bank account.

### Response for Bank Account

Example response for Bank Account:

```
{
  "payment_profile": {
    "id": 10089892,
    "first_name": "Chester",
    "last_name": "Tester",
    "created_at": "2025-01-01T00:00:00-05:00",
    "updated_at": "2025-01-01T00:00:00-05:00",
    "customer_id": 14543792,
    "current_vault": "bogus",
    "vault_token": "0011223344",
    "billing_address": "456 Juniper Court",
    "billing_city": "Boulder",
    "billing_state": "CO",
    "billing_zip": "80302",
    "billing_country": "US",
    "customer_vault_token": null,
    "billing_address_2": "",
    "bank_name": "Bank of Kansas City",
    "masked_bank_routing_number": "XXXX6789",
    "masked_bank_account_number": "XXXX3344",
    "bank_account_type": "checking",
    "bank_account_holder_type": "personal",
    "payment_type": "bank_account",
    "site_gateway_setting_id": 1,
    "gateway_handle": null
  }
}
```
- **`maxio-cli payment-profiles update`** - ## Partial Card Updates

In the event that you are using the Authorize.net, Stripe, Cybersource, Forte or Braintree Blue payment gateways, you can update just the billing and contact information for a payment method. Note the lack of credit-card related data contained in the JSON payload.

In this case, the following JSON is acceptable:

```
{
  "payment_profile": {
    "first_name": "Kelly",
    "last_name": "Test",
    "billing_address": "789 Juniper Court",
    "billing_city": "Boulder",
    "billing_state": "CO",
    "billing_zip": "80302",
    "billing_country": "US",
    "billing_address_2": null
  }
}
```

The result will be that you have updated the billing information for the card, yet retained the original card number data.

## Specific notes on updating payment profiles

- Merchants with **Authorize.net**, **Cybersource**, **Forte**, **Braintree Blue** or **Stripe** as their payment gateway can update their Customer’s credit cards without passing in the full credit card number and CVV.

- If you are using **Authorize.net**, **Cybersource**, **Forte**, **Braintree Blue** or **Stripe**, Advanced Billing will ignore the credit card number and CVV when processing an update via the API, and attempt a partial update instead. If you wish to change the card number on a payment profile, you will need to create a new payment profile for the given customer.

- A Payment Profile cannot be updated with the attributes of another type of Payment Profile. For example, if the payment profile you are attempting to update is a credit card, you cannot pass in bank account attributes (like `bank_account_number`), and vice versa.

- Updating a payment profile directly will not trigger an attempt to capture a past-due balance. If this is the intent, update the card details via the Subscription instead.

- If you are using Authorize.net or Stripe, you may elect to manually trigger a retry for a past due subscription after a partial update.

### payment-profiles-json

Manage payment profiles json

- **`maxio-cli payment-profiles-json create-payment-profile`** - Creates a payment profile for a customer.

When you create a new payment profile for a customer via the API, it does not automatically make the profile current for any of the customer’s subscriptions. To use the payment profile as the default, you must set it explicitly for the subscription or subscription group.

Select an option from the **Request Examples** drop-down on the right side of the portal to see examples of common scenarios for creating payment profiles. 

Do not use real card information for testing. See the Sites articles that cover [testing your site setup](https://docs.maxio.com/hc/en-us/articles/24250712113165-Testing-Overview#testing-overview-0-0) for more details on testing in your sandbox.

Note that collecting and sending raw card details in production requires [PCI compliance](https://docs.maxio.com/hc/en-us/articles/24183956938381-PCI-Compliance#pci-compliance-0-0) on your end. If your business is not PCI compliant, use [Maxio.js (formerly Chargify.js)](https://docs.maxio.com/hc/en-us/articles/38163190843789-Chargify-js-Overview#chargify-js-overview-0-0) to collect credit card or bank account information.

See the following articles to learn more about subscriptions and payments:

+ [Subscriber Payment Details](https://maxio.zendesk.com/hc/en-us/articles/24251599929613-Subscription-Summary-Payment-Details-Tab)
+ [Self Service Pages](https://maxio.zendesk.com/hc/en-us/articles/24261425318541-Self-Service-Pages) (Allows credit card updates by Subscriber)
+ [Public Signup Pages payment settings](https://maxio.zendesk.com/hc/en-us/articles/24261368332557-Individual-Page-Settings)
+ [Taxes](https://developers.chargify.com/docs/developer-docs/d2e9e34db740e-signups#taxes)
+ [Maxio.js (formerly Chargify.js)](https://docs.maxio.com/hc/en-us/articles/38163190843789-Chargify-js-Overview)
    + [Maxio.js with GoCardless - minimal example](https://docs.maxio.com/hc/en-us/articles/38206331271693-Examples#h_01K0PJ15QQZKCER8CFK40MR6XJ)
    + [Maxio.js with GoCardless - full example](https://docs.maxio.com/hc/en-us/articles/38206331271693-Examples#h_01K0PJ15QR09JVHWW0MCA7HVJV)
    + [Maxio.js with Stripe Direct Debit - minimal example](https://docs.maxio.com/hc/en-us/articles/38206331271693-Examples#h_01K0PJ15QQFKKN8Z7B7DZ9AJS5)
    + [Maxio.js with Stripe Direct Debit - full example](https://docs.maxio.com/hc/en-us/articles/38206331271693-Examples#h_01K0PJ15QRECQQ4ECS3ZA55GY7)
    + [CMaxio.js with Stripe BECS Direct Debit - minimal example](https://developers.chargify.com/docs/developer-docs/ZG9jOjE0NjAzNDIy-examples#minimal-example-with-sepa-or-becs-direct-debit-stripe-gateway)
    + [Maxio.js with Stripe BECS Direct Debit - full example](https://developers.chargify.com/docs/developer-docs/ZG9jOjE0NjAzNDIy-examples#full-example-with-sepa-direct-debit-stripe-gateway)
+ [Full documentation on GoCardless](https://maxio.zendesk.com/hc/en-us/articles/24176159136909-GoCardless)
+ [Full documentation on Stripe SEPA Direct Debit](https://maxio.zendesk.com/hc/en-us/articles/24176170430093-Stripe-SEPA-and-BECS-Direct-Debit)
+ [Full documentation on Stripe BECS Direct Debit](https://maxio.zendesk.com/hc/en-us/articles/24176170430093-Stripe-SEPA-and-BECS-Direct-Debit)
+ [Full documentation on Stripe BACS Direct Debit](https://maxio.zendesk.com/hc/en-us/articles/24176170430093-Stripe-SEPA-and-BECS-Direct-Debit)

## 3D Secure (3DS) Authentication post-authentication flow

When a payment requires 3DS Authentication to adhere to Strong Customer Authentication (SCA), the request enters a post-authentication flow where a 422 Unprocessable Entity status is returned with an action_link that will direct the customer through 3DS Authentication. 

See the [3D Secure Post-Authentication Flow](https://docs.maxio.com/hc/en-us/articles/44277749524365-3D-Secure-Post-Authentication-Flow) article in the product documentation to learn how to manage the redirect flow.
- **`maxio-cli payment-profiles-json list-payment-profiles`** - This method will return all of the active `payment_profiles` for a Site, or for one Customer within a site.  If no payment profiles are found, this endpoint will return an empty array, not a 404.

### portal

Manage portal

- **`maxio-cli portal enable-billing-for-customer`** - ## Billing Portal Documentation

Full documentation on how the Billing Portal operates within the Advanced Billing UI can be located [here](https://maxio.zendesk.com/hc/en-us/articles/24252412965133-Billing-Portal-Overview).

This documentation is focused on how the to configure the Billing Portal Settings, as well as Subscriber Interaction and Merchant Management of the Billing Portal.

You can use this endpoint to enable Billing Portal access for a Customer, with the option of sending the Customer an Invitation email at the same time.

## Billing Portal Security

If your customer has been invited to the Billing Portal, then they will receive a link to manage their subscription (the “Management URL”) automatically at the bottom of their statements, invoices, and receipts. **This link changes periodically for security and is only valid for 65 days.**

If you need to provide your customer their Management URL through other means, you can retrieve it via the API. Because the URL is cryptographically signed with a timestamp, it is not possible for merchants to generate the URL without requesting it from Advanced Billing.

In order to prevent abuse & overuse, we ask that you request a new URL only when absolutely necessary. Management URLs are good for 65 days, so you should re-use a previously generated one as much as possible. If you use the URL frequently (such as to display on your website), **do not** make an API request to Advanced Billing every time.
- **`maxio-cli portal read-billing-link`** - This method will provide to the API user the exact URL required for a subscriber to access the Billing Portal.

## Rules for Management Link API

+ When retrieving a management URL, multiple requests for the same customer in a short period will return the **same** URL
+ We will not generate a new URL for 15 days
+ You must cache and remember this URL if you are going to need it again within 15 days
+ Only request a new URL after the `new_link_available_at` date
+ You are limited to 15 requests for the same URL. If you make more than 15 requests before `new_link_available_at`, you will be blocked from further Management URL requests (with a response code `429`)
- **`maxio-cli portal resend-billing-invitation`** - You can resend a customer's Billing Portal invitation.

If you attempt to resend an invitation 5 times within 30 minutes, you will receive a `422` response with `error` message in the body.

If you attempt to resend an invitation when the Billing Portal is already disabled for a Customer, you will receive a `422` error response.

If you attempt to resend an invitation when the Billing Portal is already disabled for a Customer, you will receive a `422` error response.

If you attempt to resend an invitation when the Customer does not exist a Customer, you will receive a `404` error response.

## Limitations

This endpoint will only return a JSON response.
- **`maxio-cli portal revoke-billing-access`** - You can revoke a customer's Billing Portal invitation.

If you attempt to revoke an invitation when the Billing Portal is already disabled for a Customer, you will receive a 422 error response.

## Limitations

This endpoint will only return a JSON response.

### price-points

Manage price points


### product-families

Manage product families

- **`maxio-cli product-families <id>`** - Retrieves a Product Family via the `product_family_id`. The response will contain a Product Family object.

The product family can be specified either with the id number, or with the `handle:my-family` format.

### product-families-json

Manage product families json

- **`maxio-cli product-families-json create-product-family`** - Creates a Product Family within your Advanced Billing site. Create a Product Family to act as a container for your products, components and coupons.

Full documentation on how Product Families operate within the Advanced Billing UI can be located [here](https://maxio.zendesk.com/hc/en-us/articles/24261098936205-Product-Families).
- **`maxio-cli product-families-json list-product-families`** - Retrieve a list of Product Families for a site.

### product-price-points

Manage product price points


### products

Manage products

- **`maxio-cli products archive`** - Archives the product. All current subscribers will be unffected; their subscription/purchase will continue to be charged monthly.

This will restrict the option to chose the product for purchase via the Billing Portal, as well as disable Public Signup Pages for the product.
- **`maxio-cli products read`** - Reads the current details of a product.
- **`maxio-cli products read-by-handle`** - Retrieves a Product object by its `api_handle`.
- **`maxio-cli products update`** - Updates aspects of an existing product.

### Input Attributes Update Notes

+ `update_return_params` The parameters we will append to your `update_return_url`. See Return URLs and Parameters

### Product Price Point

Updating a product using this endpoint will create a new price point and set it as the default price point for this product. If you should like to update an existing product price point, that must be done separately.

### products-json

Manage products json

- **`maxio-cli products-json`** - This method allows to retrieve a list of Products belonging to a Site.

### products-price-points-json

Manage products price points json

- **`maxio-cli products-price-points-json`** - This method allows retrieval of a list of Products Price Points belonging to a Site.

### proforma-invoices

Manage proforma invoices

- **`maxio-cli proforma-invoices <proforma_invoice_uid>`** - Use this endpoint to read the details of an existing proforma invoice.

## Restrictions

Proforma invoices are only available on Relationship Invoicing sites.

### reason-codes

Manage reason codes

- **`maxio-cli reason-codes delete`** - This method gives a merchant the option to delete one reason code from the Churn Reason Codes. This code will be immediately removed. This action is not reversible.
- **`maxio-cli reason-codes read`** - This method gives a merchant the option to retrieve a list of a particular code for a given Site by providing the unique numerical ID of the code.
- **`maxio-cli reason-codes update`** - This method gives a merchant the option to update an existing reason code for a given site.

### reason-codes-json

Manage reason codes json

- **`maxio-cli reason-codes-json create-reason-code`** - # Reason Codes Intro

ReasonCodes are a way to gain a high level view of why your customers are cancelling the subscription to your product or service.

Add a set of churn reason codes to be displayed in-app and/or the Maxio Billing Portal. As your subscribers decide to cancel their subscription, learn why they decided to cancel.

## Reason Code Documentation

Full documentation on how Reason Codes operate within Advanced Billing can be located under the following links.

[Churn Reason Codes](https://maxio.zendesk.com/hc/en-us/articles/24286647554701-Churn-Reason-Codes)

## Create Reason Code

This method gives a merchant the option to create a reason codes for a given Site.
- **`maxio-cli reason-codes-json list-reason-codes`** - This method gives a merchant the option to retrieve a list of all of the current churn codes for a given site.

### referral-codes

Manage referral codes

- **`maxio-cli referral-codes`** - Use this method to determine if the referral code is valid and applicable within your Site. This method is useful for validating referral codes that are entered by a customer.

## Referrals Documentation

Full documentation on how to use the referrals feature in the Advanced Billing UI can be located [here](https://maxio.zendesk.com/hc/en-us/sections/24286965611405-Referrals).

## Server Response

If the referral code is valid the status code will be `200` and the referral code will be returned. If the referral code is invalid, a `404` response will be returned.

### sellers

Manage sellers


### site-json

Manage site json

- **`maxio-cli site-json`** - Retrieves site data.

Full documentation on Sites in the Advanced Billing UI can be located [here](https://maxio.zendesk.com/hc/en-us/sections/24250550707085-Sites).

Specifically, the [Clearing Site Data](https://maxio.zendesk.com/hc/en-us/articles/24250617028365-Clearing-Site-Data) section is relevant to this endpoint documentation.

#### Relationship invoicing enabled
If the site has RI enabled then you will see more settings like:

    "customer_hierarchy_enabled": true,
    "whopays_enabled": true,
    "whopays_default_payer": "self"
You can read more about these settings here:
 [Who Pays & Customer Hierarchy](https://maxio.zendesk.com/hc/en-us/articles/24252185211533-Customer-Hierarchies-WhoPays)

### sites

Manage sites

- **`maxio-cli sites`** - Clears all data from a test site asynchronously. This call is asynchronous and there may be a delay before the site data is fully deleted. If you are clearing site data for an automated test, you will need to build in a delay and/or check that there are no products, etc., in the site before proceeding.

**This functionality will only work on sites in TEST mode. Attempts to perform this on sites in “live” mode will result in a response of 403 FORBIDDEN.**

### stats-json

Manage stats json

- **`maxio-cli stats-json`** - The Stats API is a very basic view of some Site-level stats. This API call only answers with JSON responses. An XML version is not provided.

## Stats Documentation

There currently is not a complimentary matching set of documentation that compliments this endpoint. However, each Site's dashboard will reflect the summary of information provided in the Stats response.

```
https://subdomain.chargify.com/dashboard
```

### subscription-groups

Manage subscription groups

- **`maxio-cli subscription-groups delete`** - Deletes a subscription group.
 Only groups without members can be deleted.
- **`maxio-cli subscription-groups find`** - Finds the subscription group associated with a subscription.

If the subscription is not in a group, the endpoint will return a 404 code.
- **`maxio-cli subscription-groups read`** - Returns subscription group details.

#### Current Billing Amount in Cents

Current billing amount for the subscription group is not returned by default. If this information is desired, the `include[]=current_billing_amount_in_cents` parameter must be provided with the request.
- **`maxio-cli subscription-groups signup-with`** - Creates multiple subscriptions at once under the same customer and consolidates them into a subscription group.

You must provide one and only one of the `payer_id`/`payer_reference`/`payer_attributes` for the customer attached to the group.

You must provide one and only one of the `payment_profile_id`/`credit_card_attributes`/`bank_account_attributes` for the payment profile attached to the group.

Only one of the `subscriptions` can have `"primary": true` attribute set.

When passing a product to a subscription you can use either `product_id` or `product_handle` or `offer_id`. You can also use `custom_price` instead.
The subscription request examples below will be split into two sections.
The first section, "Subscription Customization", will focus on passing different information with a subscription, such as components, calendar billing, and custom fields. These examples will presume you are using a secure chargify_token generated by Maxio.js (formerly Chargify.js).
- **`maxio-cli subscription-groups update-members`** - Updates subscription group members.
`"member_ids"` should contain an array of both subscription IDs to set as group members and subscription IDs already present in the groups. Not including them will result in removing them from the subscription group. To clean up members, just leave the array empty.

### subscription-groups-json

Manage subscription groups json

- **`maxio-cli subscription-groups-json create-subscription-group`** - Creates a subscription group with given members.
- **`maxio-cli subscription-groups-json list-subscription-groups`** - Returns an array of subscription groups for the site. The response is paginated and will return a `meta` key with pagination information.

#### Account Balance Information

Account balance information for the subscription groups is not returned by default. If this information is desired, the `include[]=account_balances` parameter must be provided with the request.

### subscriptions

Manage subscriptions

- **`maxio-cli subscriptions cancel`** - Cancels the Subscription. The Delete method sets the Subscription state to `canceled`.
To cancel the subscription immediately, omit any schedule parameters from the request. To use the schedule options, the Schedule Subscription Cancellation feature must be enabled on your site.
- **`maxio-cli subscriptions create-signup-proforma-invoice`** - This endpoint is only available for Relationship Invoicing sites. It cannot be used to create consolidated proforma invoices or preview prepaid subscriptions.

Create a proforma invoice to preview costs before a subscription's signup. Like other proforma invoices, it can be emailed to the customer, voided, and publicly viewed on the chargifypay domain.

Pass a payload that resembles a subscription create or signup preview request. For example, you can specify components, coupons/a referral, offers, custom pricing, and an existing customer or payment profile to populate a shipping or billing address.

A product and customer first name, last name, and email are the minimum requirements. We recommend associating the proforma invoice with a customer_id to easily find their proforma invoices, since the subscription_id will always be blank.
- **`maxio-cli subscriptions find`** - Finds a subscription by its reference.
- **`maxio-cli subscriptions preview`** - Previews a subscription by POSTing the same JSON or XML as for a subscription creation.

The "Next Billing" amount and "Next Billing" date are represented in each Subscriber's Summary.

A subscription will not be created by utilizing this endpoint; it is meant to serve as a prediction.

For more information, see our documentation [here](https://maxio.zendesk.com/hc/en-us/articles/24252493695757-Subscriber-Interface-Overview).

## Taxable Subscriptions

This endpoint will preview taxes applicable to a purchase. In order for taxes to be previewed, the following conditions must be met:

+ Taxes must be configured on the subscription
+ The preview must be for the purchase of a taxable product or component, or combination of the two.
+ The subscription payload must contain a full billing or shipping address in order to calculate tax

For more information about creating taxable previews, see our documentation guide on how to create [taxable subscriptions.](https://maxio.zendesk.com/hc/en-us/sections/24287012349325-Taxes)

You do **not** need to include a card number to generate tax information when you are previewing a subscription. However, when you actually want to create the subscription, you must include the credit card information if you want the billing address to be stored in Advanced Billing. The billing address and the credit card information are stored together within the payment profile object. Also, you may not send a billing address to Advanced Billing without payment profile information, as the address is stored on the card.

You can pass shipping and billing addresses and still decide not to calculate taxes. To do that, pass `skip_billing_manifest_taxes: true` attribute.

## Non-taxable Subscriptions

If you'd like to calculate subscriptions that do not include tax you may leave off the billing information.
- **`maxio-cli subscriptions preview-signup-proforma-invoice`** - This endpoint is only available for Relationship Invoicing sites. It cannot be used to create consolidated proforma invoice previews or preview prepaid subscriptions.

Create a signup preview in the format of a proforma invoice to preview costs before a subscription's signup. You have the option of optionally previewing the first renewal's costs as well. The proforma invoice preview will not be persisted.

Pass a payload that resembles a subscription create or signup preview request. For example, you can specify components, coupons/a referral, offers, custom pricing, and an existing customer or payment profile to populate a shipping or billing address.

A product and customer first name, last name, and email are the minimum requirements.
- **`maxio-cli subscriptions read`** - Retrieves subscription details.

## Self-Service Page token

Self-Service Page token for the subscription is not returned by default. If this information is desired, the include[]=self_service_page_token parameter must be provided with the request.
- **`maxio-cli subscriptions update`** - Updates one or more attributes of a subscription.

## Update Subscription Payment Method

Change the card that your subscriber uses for their subscription. You can also use this method to change the expiration date of the card **if your gateway allows**.

Do not use real card information for testing. See the Sites articles that cover [testing your site setup](https://docs.maxio.com/hc/en-us/articles/24250712113165-Testing-Overview#testing-overview-0-0) for more details on testing in your sandbox.

Note that collecting and sending raw card details in production requires [PCI compliance](https://docs.maxio.com/hc/en-us/articles/24183956938381-PCI-Compliance#pci-compliance-0-0) on your end. If your business is not PCI compliant, use [Chargify.js](https://docs.maxio.com/hc/en-us/articles/38163190843789-Chargify-js-Overview#chargify-js-overview-0-0) to collect credit card or bank account information.

> Note: Partial card updates for **Authorize.Net** are not allowed via this endpoint. The existing Payment Profile must be directly updated instead.

## Update Product

You also use this method to change the subscription to a different product by setting a new value for product_handle. A product change can be done in two different ways, **product change** or **delayed product change**.

### Product Change

You can change a subscription's product. The new payment amount is calculated and charged at the normal start of the next period. If you require complex product changes or prorated upgrades and downgrades instead, please see the documentation on [Migrating Subscription Products](https://docs.maxio.com/hc/en-us/articles/24252069837581-Product-Changes-and-Migrations#product-changes-and-migrations-0-0).

To perform a product change, set either the `product_handle` or `product_id` attribute to that of a different product from the same site as the subscription. You can also change the price point by passing in either `product_price_point_id` or `product_price_point_handle` - otherwise the new product's default price point is used.

### Delayed Product Change

This method also changes the product and/or price point, and the new payment amount is calculated and charged at the normal start of the next period.

This method schedules the product change to happen automatically at the subscription’s next renewal date. To perform a delayed product change, set the `product_handle` attribute as you would in a regular product change, but also set the `product_change_delayed` attribute to `true`. No proration applies in this case.

You can also perform a delayed change to the price point by passing in either `product_price_point_id` or `product_price_point_handle`

> **Note:** To cancel a delayed product change, set `next_product_id` to an empty string.

## Billing Date Changes

You can update dates for a subscription.

### Regular Billing Date Changes

Send the `next_billing_at` to set the next billing date for the subscription. After that date passes and the subscription is processed, the following billing date will be set according to the subscription's product period.

> Note: If you pass an invalid date, the correct date is automatically set to the correct date. For example, if February 30 is passed, the next billing would be set to March 2nd in a non-leap year.

The server response will not return data under the key/value pair of `next_billing_at`. View the key/value pair of `current_period_ends_at` to verify that the `next_billing_at` date has been changed successfully.

### Calendar Billing and Snap Day Changes

For a subscription using Calendar Billing, setting the next billing date is a bit different. Send the `snap_day` attribute to change the calendar billing date for **a subscription using a product eligible for calendar billing**.

> Note: If you change the product associated with a subscription that contains a `snap_day` and immediately `READ/GET` the subscription data, it will still contain original `snap_day`. The `snap_day` will reset to null on the next billing cycle. This is because a product change is instantaneous and only affects the product associated with a subscription.

### subscriptions-components-json

Manage subscriptions components json

- **`maxio-cli subscriptions-components-json`** - Lists components applied to each subscription.

### subscriptions-json

Manage subscriptions json

- **`maxio-cli subscriptions-json create-subscription`** - Creates a Subscription for a customer and product.

Specify the product with `product_id` or `product_handle`. To set a specific product price point, use `product_price_point_handle` or `product_price_point_id`.

Identify an existing customer with `customer_id` or `customer_reference`. Optionally, include an existing payment profile using `payment_profile_id`. To create a new customer, pass customer_attributes. 

Select an option from the **Request Examples** drop-down on the right side of the portal to see examples of common scenarios for creating subscriptions. 

See the Subscription Signups article for more information on working with subscriptions in Advanced Billing.

## Payment information  

Payment information may be required to create a subscription, depending on the options for the Product being subscribed. See [product options](https://docs.maxio.com/hc/en-us/articles/24261076617869-Edit-Products) for more information. See the Payments Profile endpoint for details on payment parameters. 

Do not use real card information for testing. See the Sites articles that cover [testing your site setup](https://docs.maxio.com/hc/en-us/articles/24250712113165-Testing-Overview#testing-overview-0-0) for more details on testing in your sandbox.

Note that collecting and sending raw card details in production requires [PCI compliance](https://docs.maxio.com/hc/en-us/articles/24183956938381-PCI-Compliance#pci-compliance-0-0) on your end. If your business is not PCI compliant, use [Maxio.js (formerly Chargify.js)](https://docs.maxio.com/hc/en-us/articles/38163190843789-Chargify-js-Overview#chargify-js-overview-0-0) to collect credit card or bank account information.

## 3D Secure (3DS) Authentication post-authentication flow

When a payment requires 3DS Authentication to adhere to Strong Customer Authentication (SCA), the request enters a post-authentication flow where a 422 Unprocessable Entity status is returned with an action_link that will direct the customer through 3DS Authentication. 

See the [3D Secure Post-Authentication Flow](https://docs.maxio.com/hc/en-us/articles/44277749524365-3D-Secure-Post-Authentication-Flow) article in the product documentation to learn how to manage the redirect flow.
- **`maxio-cli subscriptions-json list-subscriptions`** - Returns an array of subscriptions from a Site. Pay close attention to query string filters and pagination in order to control responses from the server.

## Search for a subscription

Use the query strings below to search for a subscription using the criteria available. The return value will be an array.

## Self-Service Page token

Self-Service Page token for the subscriptions is not returned by default. If this information is desired, the include[]=self_service_page_token parameter must be provided with the request.

### subscriptions-mrr-json

Manage subscriptions mrr json

- **`maxio-cli subscriptions-mrr-json`** - This endpoint returns your site's current MRR, including plan and usage breakouts split per subscription.

### webhooks

Manage webhooks

- **`maxio-cli webhooks enable`** - Enables webhooks for your site.
- **`maxio-cli webhooks replay`** - Replays webhooks. Posting to this endpoint does not immediately resend the webhooks. They are added to a queue and sent as soon as possible, depending on available system resources. You can submit an array of up to 1000 webhook IDs in the replay request.

### webhooks-json

Manage webhooks json

- **`maxio-cli webhooks-json`** - Retrieves a list of webhooks.  You can pass query parameters if you want to filter webhooks. See the Webhooks documentation for more information.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
maxio-cli api-exports export-invoices

# JSON for scripting and agents
maxio-cli api-exports export-invoices --json

# Filter to specific fields
maxio-cli api-exports export-invoices --json --select id,name,status

# Dry run  -  show the request without sending
maxio-cli api-exports export-invoices --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
maxio-cli api-exports export-invoices --agent
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

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `MAXIO_SITE` resolves `{site}`

Base URL: `https://{site}.chargify.com`

## Health Check

```bash
maxio-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/maxio-advanced-billing-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `MAXIO_SITE` | endpoint | Yes |  |
| `MAXIO_USERNAME` | per_call | Yes |  |
| `MAXIO_PASSWORD` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `maxio-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `maxio-cli doctor` to check credentials
- Verify the environment variable is set: `echo $MAXIO_USERNAME`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **mrr/retention/cohort commands return empty or thin history**  -  Run sync first; historic depth accrues from each sync's snapshot. First sync backfills from the live movement endpoint.
- **401 Unauthorized**  -  Check MAXIO_USERNAME is your Advanced Billing API key, MAXIO_PASSWORD is set (the literal 'x' for API-key access), and MAXIO_SITE is the bare subdomain (no .chargify.com).
- **403 on mrr/mrr_movements/subscriptions_mrr endpoints**  -  The Insights/Analytics add-on is not enabled on the site; the local-compute mrr commands still work off synced data.
- **rate limited (429)**  -  The client backs off and retries; lower sync concurrency or sync fewer --resources at once if it persists.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**ab-golang-sdk**](https://github.com/maxio-com/ab-golang-sdk)  -  Go (7 stars)
- [**go-chargify**](https://github.com/GetWagz/go-chargify)  -  Go (2 stars)
- [**tap-saasoptics**](https://github.com/singer-io/tap-saasoptics)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
