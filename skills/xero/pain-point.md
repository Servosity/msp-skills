# xero skill - the MSP pain it closes

## The pain

Cash flow is the thing that kills small service businesses, and the slowest loop
in it is collections. On r/msp and r/bookkeeping the same complaint recurs: the
month-end close and the weekly AR chase are manual rituals. You export the Aged
Receivables report from Xero, paste it into a spreadsheet, pivot by days overdue,
and only then know who to call - a list that is already a day stale by the time
it is built. Reconciliation is worse: matching applied cash to authorised
invoices, and unreconciled bank lines to the invoices they probably settle, is a
click-back-and-forth between two screens, one row at a time.

Trying to script around it runs straight into Xero's API limits - 60 calls per
minute and 5,000 per day per organisation - so naive "just hit the endpoint per
question" integrations crawl, and the answers a business actually wants (aging,
exposure, the cash-application gap, whether the general ledger ties to outstanding
invoices at close) span more than one endpoint and never come back in a single
call.

## What this skill does about it

Sync the organisation once into a local SQLite mirror, then every analytical
question is one offline command - no per-question API call, no rate-limit wall:

- **`xero-cli aging --agent`** - bucket every outstanding invoice by days overdue
  (current / 1-30 / 31-60 / 61-90 / 90+) so this week's chase list builds itself;
  `--payable` does the same for what you owe.
- **`xero-cli exposure --agent`** - rank contacts by total amount due with an
  overdue split, so you see receivable concentration before a collections push.
- **`xero-cli reconcile --agent`** - surface authorised invoices still owed with
  no (or partial) applied payment: the cash-application gap, not a raw list.
- **`xero-cli bank-recon --agent`** - list unreconciled bank transactions and the
  invoices/payments they likely match by contact and exact amount.
- **`xero-cli tie-out --agent`** - prove the books tie at close by comparing the
  GL receivable/payable control accounts against outstanding invoices; a variance
  of zero is the closure signal.

## Status

Beta. Validated against the Xero API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
