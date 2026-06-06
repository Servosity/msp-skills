# syncro skill - the MSP pain it closes

## The pain

MSPs leak revenue they already earned. Industry analyses put billing leakage at
roughly **10% of revenue**, and time-tracking inaccuracy is named the single
largest source: labor gets logged on a ticket and then quietly never makes it
onto an invoice. DeskDay's mid-market revenue-leakage breakdown estimates a
30-person team can have **50-100 unbilled hours a month** - $7,500 to $15,000 in
work done and never charged (DeskDay, "Revenue Leakage in Mid-Market MSPs";
rev.io, "The Hidden Revenue Leak: Where MSPs Are Losing Money on Billing").

Syncro has the time entries and the invoices - but the portal never *ranks*
logged-but-unbilled hours by customer, so nobody sees the leak until a quarter
later, if ever. The same is true of the other questions an owner keeps
re-asking: which tickets are going stale, which assets are missing patches, which clients
generate the most RMM noise. Each one means exporting a report and pivoting a
spreadsheet, so they get asked at QBR time instead of every week.

## What this skill does about it

It syncs your Syncro PSA and RMM data into a local mirror and turns those
recurring questions into one command each:

- **`syncro-cli billing uninvoiced`** - logged-but-unbilled labor ranked by
  customer, so the leak is visible this week, not next quarter.
- **`syncro-cli billing drift`** - tickets closed long ago that had billable
  time and never got invoiced.
- **`syncro-cli billing ar-aging`** - unpaid invoices bucketed 0-30/30-60/60-90/90+
  so collections start with the worst.
- **`syncro-cli tickets aging`** - open tickets with no recent activity, going
  stale before the client notices.
- **`syncro-cli assets patch-gaps`** - assets missing critical patches ranked
  across every customer.

## Status

Beta. Validated against the Syncro API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
