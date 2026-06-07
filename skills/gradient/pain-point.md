# gradient skill - the MSP pain it closes

## The pain

Billing reconciliation is the recurring tax on MSP margin: every cycle, someone
pulls a CSV from each vendor, hand-maps the fields to the PSA, and hopes they
caught every seat that drifted between the vendor's count and the contract. It is
a standing topic on r/msp, and Gradient MSP's own research frames the manual
version as costing up to 90% more time than automating it - which is exactly why
1,000+ MSPs run Synthesize to reconcile usage against billing.

But Synthesize's only programmatic surface for feeding that data is a PowerShell
SDK that expects a script project per integration. So the usage pushes that drive
reconciliation end up living in brittle one-off scripts: no record of what was
sent, no answer to "what changed since last night?", and no confirmation that a
dispatched alert ever became a PSA ticket. Push one count per account in a loop
and you can even trigger a billing rebuild on every single call.

## What this skill does about it

- **`usage push`** - push a whole CSV or JSON file of unit counts in one shot and
  trigger exactly one billing rebuild, not one per row.
- **`usage drift`** - the pre-invoice pre-flight: show exactly which accounts'
  counts changed between your last two pushes, old-to-new, from a local ledger.
- **`alert send --wait`** - dispatch an alert and block until the PSA ticket
  actually exists, instead of fire-and-forget; **`alert trace --stuck`** lists the
  ones that never landed.
- **`hygiene unmapped`** - one work-queue rollup of every unmapped account and
  every service missing a vendor SKU, so nothing reconciles to nothing.
- **`status ready`** - a go/no-go check on whether the integration is ready to
  flip to active before you turn it on.

## Status

Beta. Validated against the Gradient MSP Synthesize vendor API surface; the
closed-loop receipt (a named MSP running it live in their production tenant at a
Build Session) is tracked separately and added here as `video.md` once it exists.
