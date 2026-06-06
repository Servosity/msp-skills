# pax8 skill - the MSP pain it closes

## The pain

Pax8 is where a lot of MSPs buy and resell Microsoft, security, and backup. It
is excellent at provisioning and ordering. It is not good at telling you, in one
place, where the money is leaking.

Every month an MSP faces three moving targets at once: vendor invoices (Pax8,
Microsoft 365, security tools), client usage that changes mid-cycle, and PSA
contracts that have to match what the client actually consumed. The marketplace
gives you a monthly invoice file; it does not join those invoice lines back to
your active subscriptions, so finding a line that bills for a cancelled product,
or a live subscription that never got invoiced, is manual work.

How manual? Automation vendor **Bumblebee** profiled an MSP that spent **five
days every month** reconciling Pax8 invoices against their PSA before they
automated it ([hirebumblebee.com](https://www.hirebumblebee.com/blogs/pax8-integration-guide-partnership)).
That an entire market of paid reconciliation add-ons exists at all is the
clearest signal the portal does not surface this on its own. Pax8's own
developer documentation is candid that mapping invoice line items back to
subscriptions, representing credited invoices, and choosing between invoice
events and scheduled reconciliation is real integration work
([devx.pax8.com](https://devx.pax8.com/docs/invoice-billing-integrations)).

The same blind spot hides margin and overages. Recurring revenue and margin
(subscription price times quantity, minus partner cost) is a spreadsheet you
rebuild by hand. Metered-usage overages - Azure, backup, per-GB security - only
become visible once they have already posted to the customer invoice, so the
surprise is yours to absorb.

## What this skill does about it

It syncs your Pax8 Partner API data into a local SQLite mirror, then answers the
cross-entity questions the portal cannot compose - offline, in one query:

- **`pax8-cli reconcile`** - flags invoice lines with no active subscription, and
  active subscriptions that were never billed. `--draft` runs the same join
  against the next unposted invoice so you catch leakage before it finalizes.
- **`pax8-cli mrr`** - monthly recurring revenue and margin, broken down by
  product, trended across syncs - the number you used to build in a spreadsheet.
- **`pax8-cli overage`** - flags usage summaries running well above their product
  average before they land on the customer invoice.
- **`pax8-cli spend`** - ranks customers by total spend across every invoice.
- **`pax8-cli company show <companyId>`** - one customer's subscriptions,
  contacts, invoices, and usage in a single view.

## Status

Beta. Validated against the Pax8 API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
