# DataGate skill - the MSP pain it closes

## The pain

MSPs and telecom resellers who bill customers through DataGate typically only touch
it through its web portal: click into a customer, click into their invoices, click
into their agreement, one customer at a time. There's no built-in way to ask a
cross-customer question ("which invoices are still unpaid this month across every
customer?") without exporting and reassembling the answer by hand, and DataGate's
API enforces a real rate limit (60 requests/minute, 5,000/day, per account) that a
naive script or wrapper can burn through fast when it re-fetches everything on every
question.

Monthly billing reconciliation, in particular, tends to be a recurring manual chore:
open the portal, filter to the billing period, export or eyeball each invoice,
repeat next month.

## What this skill does about it

It puts DataGate's customer, agreement, and invoice data one sentence away instead
of behind portal clicks:

- `invoices` - pull a billing period's invoices with a JSON filter, instead of
  clicking through the portal one invoice at a time.
- `customers` / `agreements` - look up a customer's account and agreement without
  navigating there manually.
- `sync` + `search` - mirror the data locally once, then answer repeated questions
  offline without spending more of DataGate's rate limit.

## Status

Beta. Built against DataGate's public API documentation; awaiting a
live-verification report from an MSP running it against their own tenant (see
[CONTRIBUTING.md](../../CONTRIBUTING.md), rung 1).
