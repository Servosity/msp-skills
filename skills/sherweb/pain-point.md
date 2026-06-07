# sherweb skill - the MSP pain it closes

## The pain

MSPs who resell Microsoft 365, Azure, and security through Sherweb hit the same
wall every month-end: the Sherweb Partner API splits billing across two
different surfaces. The **Distributor API** returns the *payable* charges - what
you owe Sherweb. The **Service Provider API** returns the *receivable* charges
and the per-customer subscriptions - what you bill your clients. No single portal
screen joins them, so the one number an owner actually runs the business on -
net margin per customer - gets rebuilt by hand in a spreadsheet every close.

The recurring r/msp version of this pain is "license sprawl": a client offboards
or downsizes, the subscription stays active on the Sherweb side, and the MSP
keeps paying for seats it never bills back. Combined with metered Azure/usage
add-ons that only become visible after they post to a charge, margin bleeds
quietly until someone audits the entire book - usually after the quarter is
already lost.

## What this skill does about it

After one `sherweb-cli sync` + `sherweb-cli deep-sync` into a local SQLite
mirror, the cross-entity questions become single offline joins:

- `sherweb-cli margin` - net margin per customer (receivable minus payable),
  worst-margin-first, for any billing month.
- `sherweb-cli orphans` - active subscriptions with zero receivable charges:
  seats you pay for but never bill back.
- `sherweb-cli usage-leak` - metered platform usage with no matching receivable
  charge: consumption you absorb instead of billing.
- `sherweb-cli right-size` - subscriptions where seats paid differ from seats
  metered as used, per customer.
- `sherweb-cli margin-trend` - each customer's margin across the last N closes,
  steepest-decline-first, so a sliding account surfaces before it goes negative.

## Status

Beta. Validated against the Sherweb Partner API surface; the closed-loop receipt
(a named MSP running it live in their production partner account at a Build
Session) is tracked separately and added here as `video.md` once it exists.
