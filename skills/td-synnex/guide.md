# TD SYNNEX StreamOne ION CLI

**The StreamOne ION v3 reseller API (customers, catalog, orders, subscriptions, carts, reports), plus a local licence book for renewals, margin and cross-distributor true-up.**

Manage customers, catalog, orders, subscriptions, carts and billing reports from the terminal or any agent. Sync your book to SQLite and answer book-wide questions no single endpoint returns: what renews this month (subscriptions renewals), where you lose money (margin), what changed (subscriptions changes), and how TD SYNNEX seats line up against Pax8 and Sherweb exports (position trueup). Seat and renewal changes become one previewed command (subscriptions set) instead of a hand-built order body.

Learn more at [TD SYNNEX StreamOne ION](https://docs.streamone.cloud/).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `td-synnex-cli` and `td-synnex-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=td-synnex). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/td-synnex --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/td-synnex --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `td-synnex-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the td-synnex skill from https://github.com/Servosity/msp-skills/tree/main/skills/td-synnex. The skill defines how its required CLI (`td-synnex-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=td-synnex).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. When Claude Desktop prompts, set `TD_SYNNEX_ACCOUNT_ID` to your reseller account ID and leave every other field empty. Before the first start, run `td-synnex-cli auth mint --refresh-token-stdin` once and `td-synnex-cli auth mint --if-expiring 15m` from cron, and the server reads the stored, refreshed token. An access token set here expires after 2 hours and always overrides the stored one.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "td-synnex": {
      "command": "td-synnex-mcp",
      "env": {
        "TD_SYNNEX_ACCOUNT_ID": "<your reseller account ID>"
      }
    }
  }
}
```

Leave `TD_SYNNEX_ACCESS_TOKEN` out: run `td-synnex-cli auth mint --refresh-token-stdin` once and `td-synnex-cli auth mint --if-expiring 15m` from cron, and the server reads the stored, refreshed token. An access token set here expires after 2 hours and always overrides the stored one.

</details>

## Authentication

**API calls need a StreamOne ION access token.** The local-store commands (subscriptions renewals and changes, margin, orders stuck, customers show, position export and trueup) read synced data and need no token once you have synced. Set `TD_SYNNEX_ACCOUNT_ID` to your reseller account ID (portal Quick Access > Account Information) so account paths resolve.

StreamOne ION uses OAuth 2.0 refresh tokens that are single-use and rotate on every exchange (32-day lifetime; access tokens last 2 hours). Get the first refresh token in the ION portal under Settings > Users > Edit > OAuth credentials > Request New Credentials and pipe it to `td-synnex-cli auth mint --refresh-token-stdin`. mint exchanges it and saves the access token and the NEW refresh token to the private credentials file before printing anything; later `auth mint` runs (for example `auth mint --if-expiring 15m` from cron) use the stored token. `auth check` reports how long both tokens stay valid. Do not give the refresh token to `auth set-token`, which stores an access token. Do not export TD_SYNNEX_ACCESS_TOKEN as well: it overrides the minted token and expires after 2 hours, while `auth mint --if-expiring` keeps the stored one fresh.

## Quick Start

```bash
# First set TD_SYNNEX_ACCOUNT_ID=<your reseller account ID> (portal Quick Access > Account Information) and keep it set (shell profile, or setx on Windows): the local store is keyed on it; doctor then probes the token against your account
td-synnex-cli doctor

# Exchange the portal refresh token (on stdin, here from a file; pbpaste | ... works too) for an access token and store the rotated refresh token
td-synnex-cli auth mint --refresh-token-stdin < refresh-token.txt
# Windows (PowerShell): Get-Content refresh-token.txt | td-synnex-cli auth mint --refresh-token-stdin

# Pull your licence book into the local store (needs TD_SYNNEX_ACCOUNT_ID)
td-synnex-cli sync --resources customers,subscriptions,orders

# See what renews in the next 30 days
td-synnex-cli subscriptions renewals --within 30d

# Find the customers you are losing money on
td-synnex-cli margin --by customer

```

## Known Gaps

- **Not yet run against a live StreamOne ION tenant.** Every command was checked against TD SYNNEX's published v3 OpenAPI spec, a docs-shaped mock server and unauthenticated route probes, not real account data. Start with `td-synnex-cli doctor` and a small `td-synnex-cli sync --resources customers`, and check the first results against the ION portal.
- **Money columns in `margin`, `subscriptions renewals`, `position export` and `customers show` are unconfirmed.** They use the price and cost ION reports for each subscription (for monthly billing, its newest billed month), not multiplied by seats. Every documented example has one licence, so whether ION reports those per seat or per subscription has not been checked; if they are per seat, multi-seat totals are understated, and so is `position trueup`'s `unit_cost_by_source` (cost divided by seats), by the same factor. Compare one multi-seat subscription with the ION portal before relying on the totals.

## Unique Features

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

## Usage

Run `td-synnex-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `TD_SYNNEX_CONFIG_DIR`, `TD_SYNNEX_DATA_DIR`, `TD_SYNNEX_STATE_DIR`, or `TD_SYNNEX_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `TD_SYNNEX_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export TD_SYNNEX_HOME=/srv/td-synnex
td-synnex-cli doctor
```

On Windows (PowerShell):

```powershell
$env:TD_SYNNEX_HOME = "C:\ProgramData\td-synnex"
td-synnex-cli doctor
```

Under `TD_SYNNEX_HOME=/srv/td-synnex`, the four dirs resolve to `/srv/td-synnex/config`, `/srv/td-synnex/data`, `/srv/td-synnex/state`, and `/srv/td-synnex/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `TD_SYNNEX_DATA_DIR` overrides an explicit `--home` for that kind. Use `TD_SYNNEX_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `TD_SYNNEX_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `td-synnex-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### cart_items

Manage cart items

- **`td-synnex-cli cart-items create`** - Add a plan to a cart. `planName` is the full plan resource name, for example
accounts/{accountId}/products/IBMCognos/versions/17/plans/Analytics-on-Cloud-1-Month-Upfro.
Source: https://docs.streamone.cloud/#create-cart-item
Official spec: CreateCartItem (which confirms this path).
- **`td-synnex-cli cart-items delete`** - Remove an item from a cart (the docs site's Cart Items intro mentions removing items).
Source: official spec DeleteCartItem. Route checked 2026-10-14: unauthenticated request returns 401 (route exists).
- **`td-synnex-cli cart-items get`** - Get one cart item.
Source: https://docs.streamone.cloud/#get-cart-item
Official spec: GetCartItem.
- **`td-synnex-cli cart-items list`** - List the items in a cart with plan, quantity, prices, add-ons, term and checkout
state. The response key is `cartitems` (lowercase i).
Source: https://docs.streamone.cloud/#list-cart-items
Official spec: ListCartItems.
- **`td-synnex-cli cart-items update`** - Update a cart item's quantity, plan, add-ons or payment.
Source: https://docs.streamone.cloud/#update-cart-item
Official spec: UpdateCartItem (which confirms this path).

### carts

Manage carts

- **`td-synnex-cli carts checkout`** - Start the purchase of a cart, which places a real, billable order. Pass the final cart value (totalCalculatedValue) and
the per-item provisioning attributes (for Microsoft: domainName, agreementDateAgreed,
agreementUserId, agreementEmail, agreementFirstName, agreementLastName,
agreementPhoneNumber, companyRegistrationId, customTermEndDate). Returns the
resulting order.
Source: https://docs.streamone.cloud/#checkout-cart
Official spec: CheckoutCart.
The docs site marks totalCalculatedValue and provisionData required; the official spec marks no body field required.
- **`td-synnex-cli carts create`** - Create a cart for a customer.
Source: https://docs.streamone.cloud/#create-cart
Official spec: CreateCart.
- **`td-synnex-cli carts delete`** - Clear or delete a cart.
Source: official spec DeleteCart (not on the docs site). Route checked 2026-10-14: unauthenticated request returns 401 (route exists).
- **`td-synnex-cli carts get`** - Get one cart (a flat cart object per the official spec; the docs site sample wraps it in a
`carts` array).
Source: https://docs.streamone.cloud/#get-cart
Official spec: GetCart.
- **`td-synnex-cli carts list`** - List a customer's carts with status, payment type, currency and item count. Carts
are customer-scoped: this path has no account ID. Paginate with pagination.limit
and pagination.offset.
Source: https://docs.streamone.cloud/#list-cart
Official spec: ListCarts (status is required there and on the docs site).
- **`td-synnex-cli carts update`** - Update a cart's display name, payment, status or currency.
Source: https://docs.streamone.cloud/#update-cart
Official spec: UpdateCart.

### categories

Manage categories

- **`td-synnex-cli categories <accountId>`** - List product categories (Security, Cloud Services and others) with product counts
and parent/child links.
Source: https://docs.streamone.cloud/#list-categories
Official spec: ListCategories.

### charges

Manage charges

- **`td-synnex-cli charges <accountId>`** - List the account's pricing charge rules (markup/margin plans): charge name, plan type (FIXED_AMOUNT, FIXED_PERCENT, TIERED), whether the rate applies to customer or seller cost, tiers and currency.
Source: official spec ListCharges (not on the docs site). Route checked 2026-10-14: unauthenticated request returns 401 (route exists).

### cloud_profiles

Manage cloud profiles

- **`td-synnex-cli cloud-profiles <accountId> <customerId>`** - Show which cloud providers a customer is linked to (Microsoft CSP, Google
Workspace, Azure and others) with tenant ID, domain, GDAP status, GDAP roles and
Microsoft Customer Agreement status for Azure/Microsoft providers.
Source: https://docs.streamone.cloud/#get-customer-cloud-profiles
Official spec: GetCustomerCloudProfiles (providerId optional there; the docs site marks it required).

### cloud_providers

Manage cloud providers

- **`td-synnex-cli cloud-providers get-customer`** - Get the customer's association with one cloud provider: provisioned domain, status,
partner tenant ID, MCA email status and options (key/value pairs such as autoRenew and termSettings, shown in the docs sample).
For Microsoft, `azureCustomerInfo` is a JSON string from Partner Center that the docs
sample shows carrying tenant admin UserCredentials (including a Password). This CLI
withholds the whole string (`[redacted]`).
Source: https://docs.streamone.cloud/#get-customer-cloud-provider
Official spec: GetCustomerCloudProvider.
- **`td-synnex-cli cloud-providers link-customer`** - Link a tenant the customer already owns at the cloud provider to the ION customer.
Body `name` is accounts/{accountId}/customers/{customerId}/cloudProviders/{providerId}
and `id` is the tenant ID. The docs header shows GET, but the endpoint table lists
POST and the live host answers GET with 415 and POST with 401 (route-probe-2026-10-14.txt), so POST is used.
Source: https://docs.streamone.cloud/#link-customer-cloud-provider
Official spec: LinkCustomerCloudProvider.
- **`td-synnex-cli cloud-providers provision-customer`** - Create a new tenant for the customer at the cloud provider (for example a Microsoft
tenant for provider 30). The response returns the tenant admin username, a generated
password and the login URL where the customer signs the Microsoft Customer Agreement.
This CLI shows the password as `[redacted]`. The docs header shows GET, but the endpoint table
lists POST and the live host answers GET with 415 and POST with 401 (route-probe-2026-10-14.txt), so POST is used.
Source: https://docs.streamone.cloud/#create-customer-cloud-provider
Official spec: CustomerCloudProviderProvision.

### customer_orders

Manage customer orders

- **`td-synnex-cli customer-orders cancel-order`** - Cancel an order. Allowed only while the order status is NEW or ON_HOLD, right after
it was placed. No request body.
Source: https://docs.streamone.cloud/#cancel-order
Official spec: CancelOrder.
- **`td-synnex-cli customer-orders create-order`** - Place a real, billable order for a customer, or change an existing subscription. Every lifecycle
change in ION is an order on this same endpoint: set each order item's `action`
(CREATE or UPDATE) and pass `resourceId` (the vendor subscription ID) for updates,
with name/value `attributes` from the product's provisioning template. Documented
update attributes: operations=updateSubscription with a new `quantity` (licence
count, also during renewal); operations=updateSubscription + renewalSetting
(auto_on, cancel_at_renewal, renew_to_est); operations=updateSubscription +
statusUpdate (suspended, active, deleted); operations=changeSubscription with a new
skuId/planId (upgrade); operations=updateSubscription + friendlyName. Microsoft
creates also take domainName and the agreement* attributes. Reserved Instance orders
cannot be placed through the API.
Sources: https://docs.streamone.cloud/#create-order ,
https://docs.streamone.cloud/#update-order ,
https://docs.streamone.cloud/#update-order-license-count ,
https://docs.streamone.cloud/#update-order-renewal-settings ,
https://docs.streamone.cloud/#update-order-delete-suspend-reactivate-subscription ,
https://docs.streamone.cloud/#update-order-upgrade-subscription-from-lower-tier-to-higher-tier ,
https://docs.streamone.cloud/#update-order-adding-entitlement-and-friendly-name
Official spec: CreateOrder.
- **`td-synnex-cli customer-orders get-order`** - Get one order with its status, totals, order items, provisioning info and activity
log.
Source: https://docs.streamone.cloud/#get-order
Official spec: GetOrder.
- **`td-synnex-cli customer-orders list`** - List one customer's order history.
Source: https://docs.streamone.cloud/#list-customer-orders
Official spec: ListCustomerOrders.

### customers

List, get, create and update customers, and show one customer from the local store

- **`td-synnex-cli customers create`** - Create an end customer. The official spec requires customerOrganization, customerAddress
(street, city, state, zip, country), customerName, customerEmail and languageCode. The response includes a default customer user whose `password`
field carries the generated initial password. This CLI shows it as `[redacted]`.
Source: https://docs.streamone.cloud/#create-customer
Official spec: CreateCustomer.
- **`td-synnex-cli customers get`** - Get one end customer by ID.
Source: https://docs.streamone.cloud/#get-customer
Official spec: GetCustomer.
- **`td-synnex-cli customers list`** - List end customers under the reseller account, oldest first. The API rejects the
call with 400 "At least one of page_size and page_token must be present" when both
are missing, so pageSize is required. Each customer's `name` is its resource path
(accounts/{accountId}/customers/{customerId}); the trailing segment is the customer ID.
Source: https://docs.streamone.cloud/#list-customers
Official spec: ListCustomers.
- **`td-synnex-cli customers update`** - Replace a customer's organization, address and contact details. Same body as
create; the live API validates customerEmail, customerName and customerOrganization
as required.
Source: https://docs.streamone.cloud/#update-customer
Official spec: UpdateCustomer.

### orders

Manage orders

- **`td-synnex-cli orders <accountId>`** - List the reseller's order history across all customers, with order items, prices,
costs and margins. Filter by status, customers, providers or sub-accounts.
Source: https://docs.streamone.cloud/#list-account-orders
Official spec: ListAccountOrders.

### pricing

Manage pricing

- **`td-synnex-cli pricing <accountId>`** - Price several products in one call for a price-book customer: price-book price,
account cost, MSRP-style master price, currency and availability per product. This
is a read despite the POST. Product identifiers are the comma-joined external IDs,
for example "GB,CFQ7TTC0LFLS,0002,One-Month commitment for monthly billing", grouped
by provider resource name (accounts/{accountId}/cloudProviders/{providerId}).
Source: https://docs.streamone.cloud/#get-product-pricing
Official spec: GetProductPricing.

### products

Manage products

- **`td-synnex-cli products disable`** - Hide a catalog product for the account (toggles its visibility in the storefront). No request body.
Source: official spec DisableProduct (not on the docs site). Route checked 2026-10-14: unauthenticated request returns 401 (route exists).
- **`td-synnex-cli products enable`** - Show a catalog product for the account (toggles its visibility in the storefront). No request body.
Source: official spec EnableProduct (not on the docs site). Route checked 2026-10-14: unauthenticated request returns 401 (route exists).
- **`td-synnex-cli products get`** - Get one catalog product with marketing, definition (SKUs, plans, phases, prices),
version history and deployment details. Pass priceBookCustomerId to price it for a
customer's price book, and the exclude flags to trim the response.
Source: https://docs.streamone.cloud/#get-product
Official spec: GetProductDetails.
- **`td-synnex-cli products list`** - List catalog products available to the reseller account, with SKUs, plans and
list pricing in `definition.skus[].plans[]`. Responses are large; filter by name,
SKU, add-on, cloud provider or manufacturer part number to narrow them.
Source: https://docs.streamone.cloud/#list-products
Official spec: ListcatalogProducts.

### programs

Manage programs

- **`td-synnex-cli programs batch-update-product`** - Configure which vendor programs apply to a product. Body product_programs carries programName (accounts/{accountId}/programs/{id}) and partnerName (accounts/{accountId}).
Source: official spec BatchUpdateProductsPrograms (not on the docs site). Route checked 2026-10-14: unauthenticated request returns 401 (route exists).
- **`td-synnex-cli programs list`** - List the vendor programs defined for the account and, with --product-name, whether each is attached to that product.
Source: official spec ListPrograms (not on the docs site). Route checked 2026-10-14: unauthenticated request returns 401 (route exists).

### provision_templates

Manage provision templates

- **`td-synnex-cli provision-templates get`** - Get one provisioning template by ID. Documented IDs include provisiontemplate30
(Microsoft create), provisiontemplate33 (Microsoft modify and manage subscription),
provisiontemplate34 (Microsoft change plan), provisiontemplate55 (Microsoft renewal
settings), provisiontemplate56 (Microsoft cancel), provisiontemplate01 (Google
Workspace create) and provisiontemplate40 (Sophos create).
Source: https://docs.streamone.cloud/#get-provisioning-info-by-template-id
Official spec: GetProvisioningInfoByTemplateId.
- **`td-synnex-cli provision-templates list`** - Get the JSON schemas that define the order-item attributes a vendor needs to create
or update a subscription (Microsoft, Google, IBM, Sophos). Read these before
building an order body.
Source: https://docs.streamone.cloud/#get-provisioning-info-by-vendor-provider-or-action
Official spec: GetProvisionTemplates.

### reports

List report definitions, run a report and get its data, save, and delete reports

- **`td-synnex-cli reports delete`** - Delete a saved report if the user has permission (the docs site's Reports intro lists deleting reports).
Source: official spec DeleteReport. Route checked 2026-10-14: unauthenticated request returns 401 (route exists).
- **`td-synnex-cli reports get`** - POST a report definition and get back its resolved structure (columns, currency,
date ranges) without data, to preview a layout before running it. This is a read
despite the POST.
Source: https://docs.streamone.cloud/#get-report
Official spec: GetReport.
- **`td-synnex-cli reports get-data`** - Run a report: POST the report definition from the metadata endpoint (with the
desired date range, currency and columns) and get back the definition plus
`results.rows[].values[]`, one value per selected column. Values are typed
(stringValue, moneyValue, floatValue). This is a read despite the POST.
Source: https://docs.streamone.cloud/#get-report-data
Official spec: GetReportData.
- **`td-synnex-cli reports get-data-csv`** - Same request as the report data endpoint, but the data comes back as CSV wrapped in
a JSON response (not a downloadable attachment). This is a read despite the POST.
Source: https://docs.streamone.cloud/#get-report-data-csv
Official spec: GetReportDataCsv (response fields results = CSV text, resultCount).
- **`td-synnex-cli reports get-details`** - Retrieve the full report details with populated specs, returned as priceDetails and costDetails. Uses POST because the request can be very large; this is a read.
Source: official spec GetReportDetails (not on the docs site). Route checked 2026-10-14: unauthenticated request returns 401 (route exists).
- **`td-synnex-cli reports get-metadata`** - Get a report's structure without data: all and selected columns, column filters,
date-range options, currencies, conversion rates and data-import status. Edit the
returned object (for example specs.dateRangeOption.selectedRange) and POST it to the
report's data endpoint to run it.
Source: https://docs.streamone.cloud/#get-report-metadata
Official spec: GetReportMetadata (query includeMetadata; the docs site shows include_metadata).
- **`td-synnex-cli reports list`** - List the predefined and saved reports available to the account (billing, customer,
subscription reports) with their IDs, category, template and providers.
Source: https://docs.streamone.cloud/#list-reports
Official spec: ListReports.
- **`td-synnex-cli reports save`** - Update a saved report's settings (columns, filters, date range, currency) by POSTing the edited report definition. Returns the report ID.
Source: official spec SaveReport (not on the docs site). Route checked 2026-10-14: unauthenticated request returns 401 (route exists).

### subscriptions

List and get subscriptions, change seats, renewal or status (set), and see renewals and changes across the book

- **`td-synnex-cli subscriptions get`** - Get one subscription for a customer, including billing data. Pass --refresh to
refresh the result from the provider.
Source: https://docs.streamone.cloud/#get-customer-subscription-details
Official spec: GetCustomerSubscriptionDetails.
- **`td-synnex-cli subscriptions list`** - List subscriptions across the account (or one customer with customerId): licence
counts, status, term, billing cycle, start/end/renewal dates, auto-renew, price,
cost, margin and CCP product/SKU/plan IDs. Paginate with --pagination-limit and --pagination-offset (pagination.limit and
pagination.offset on the wire; ION documents a limit as required); the response's paginationResponse.totalSize is the filtered
total. Status values include ACCEPTED, ACTIVE, CANCELLED, DELETED, EXPIRED,
SUSPENDED and others.
Source: https://docs.streamone.cloud/#list-customer-subscriptions
Official spec: ListCustomerSubscriptions (query names customerId and subscriptionId; the docs site spells them customerID and subscriptionID).

### verticals

Manage verticals

- **`td-synnex-cli verticals <accountId>`** - List the industry verticals (Commercial, Corporate, Education, Charity and others)
used to segment products and pricing.
Source: https://docs.streamone.cloud/#list-verticals
Official spec: ListVerticals.


### customers


- **`td-synnex-cli customers show`** - One view of a customer from the local store: profile, subscriptions with seats and monthly cost and price, open orders and recent changes, plus the cloud-profiles command for live GDAP/MCA status.


### margin


- **`td-synnex-cli margin`** - See monthly cost vs price and margin per customer or per SKU, with negative, zero and below-MSRP rows flagged.


### orders


- **`td-synnex-cli orders stuck`** - Find orders still NEW, IN_PROGRESS, CONFIRMED or ON_HOLD past an age threshold, plus recent errors, with a flag for the ones you can still cancel.


### position


- **`td-synnex-cli position export`** - Export your TD SYNNEX licences as normalized, schema-versioned rows (customer, SKU key, seats, monthly cost and price, renewal) for a PSA or a true-up; tenant and domain columns stay blank because ION subscriptions do not carry them.

- **`td-synnex-cli position trueup`** - Compare seats per customer and SKU across TD SYNNEX and Pax8, Sherweb or PSA export files mapped into the licence-position columns, with unmatched rows listed instead of guessed.


### subscriptions


- **`td-synnex-cli subscriptions renewals`** - See every subscription renewing or ending in the next N days across the whole book, soonest first, with auto-renew-off ones flagged.

- **`td-synnex-cli subscriptions changes`** - List seat, renewal, status, tier and friendly-name changes requested through ION orders in a past window, decoded from the order attributes; failed orders are left out and changes made outside ION are not visible.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`td-synnex-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`td-synnex-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`td-synnex-cli learnings list`** - Inspect taught rows
- **`td-synnex-cli learnings forget <query>`** - Undo a teach
- **`td-synnex-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`td-synnex-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`td-synnex-cli teach-pattern`** - Install a query/resource template up front
- **`td-synnex-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `TD_SYNNEX_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `td-synnex-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
td-synnex-cli cart-items list 108536 240314

# JSON for scripting and agents
td-synnex-cli cart-items list 108536 240314 --json
# Filter to specific fields
td-synnex-cli cart-items list 108536 240314 --json --select cartName,checkedOut,currencyCode

# Dry run  -  show the request without sending
td-synnex-cli cart-items list 108536 240314 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
td-synnex-cli cart-items list 108536 240314 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts; inputs are positional arguments and flags
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Writes run immediately** - Writes run immediately: order, cart checkout, update and delete commands do not prompt, so preview them with `--dry-run`. `subscriptions set` only previews unless you add `--confirm`.
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` other error, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh that reads only the first page (about 100 rows) of each resource, so run `td-synnex-cli sync` before whole-book answers on a larger book; refresh failures fall back to the existing local data with a warning, and a command given its own `--db` never refreshes. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store. The book commands written for this connector (`customers show`, `margin`, `orders stuck`, `position export`, `position trueup`, `subscriptions changes`, `subscriptions renewals`) are store-only: they accept `--data-source auto` or `local`, and `--data-source live` exits 1 with `no live equivalent for this command`; refresh their data with `td-synnex-cli sync`.

Set `TD_SYNNEX_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
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

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `TD_SYNNEX_ACCOUNT_ID` resolves `{accountId}`

Base URL: `https://ion.tdsynnex.com/api/v3`

## Health Check

```bash
td-synnex-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `td-synnex-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/td-synnex-cli/config.toml` (`%USERPROFILE%\.config\td-synnex-cli\config.toml` on Windows); `--home`, `TD_SYNNEX_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `TD_SYNNEX_ACCOUNT_ID` | endpoint | Yes | Reseller account ID; fills {accountId} in every account path. |
| `TD_SYNNEX_ACCESS_TOKEN` | per_call | No | A current access token. Overrides the token stored by `auth mint`, so leave it unset when you mint. |
| `TD_SYNNEX_REFRESH_TOKEN` | auth | No | First refresh token for `auth mint` before one is stored; the stored token wins after the first mint. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `td-synnex-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `td-synnex-cli doctor` to check credentials
- Run `td-synnex-cli auth check` to see whether the token is valid and how long it lasts; an exported `TD_SYNNEX_ACCESS_TOKEN` overrides the minted one, so unset a stale export
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 {"code":16,"message":"Unable to authenticate..."}**  -  Run td-synnex-cli auth mint (access tokens expire after 2 hours). An exported TD_SYNNEX_ACCESS_TOKEN always wins over the minted token, so unset a stale one
- **auth mint says "ION rejected the refresh token (it may be used already or older than 32 days)"**  -  The refresh token was already used or is older than 32 days; request new OAuth credentials in the ION portal and pipe the new refresh token to auth mint --refresh-token-stdin
- **missing required argument (Usage shows <accountId>)**  -  set TD_SYNNEX_ACCOUNT_ID=<reseller account ID from portal Quick Access > Account Information> permanently (shell profile, or setx on Windows), or pass the account ID as the first argument
- **400 "At least one of page_size and page_token must be present"**  -  Pass --page-size (customers, products and orders lists require it)
- **403 PERMISSION_DENIED**  -  The ION user behind the token lacks the role for that resource; ask the account admin, or use a dedicated API user

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**NinjaOneAutomation (TDSynnex_NinjaOne_Sync)**](https://github.com/lwhitelock/NinjaOneAutomation)  -  PowerShell (56 stars)
- [**StreamOneIonSDKPython**](https://github.com/techdata-cloudautomation/StreamOneIonSDKPython)  -  Python (5 stars)
- [**td-synnex-stream-one-api-php**](https://github.com/inserveit/td-synnex-stream-one-api-php)  -  PHP (2 stars)
- [**cloudmindsab td-synnex**](https://github.com/cloudmindsab/td-synnex)  -  PowerShell (2 stars)
- [**IONModule**](https://github.com/BNWEIN/IONModule)  -  PowerShell
- [**n8n-nodes-tdsynnex**](https://github.com/affeldt28/n8n-nodes-tdsynnex)  -  TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
