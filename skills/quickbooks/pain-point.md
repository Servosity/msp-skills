# quickbooks skill - the MSP pain it closes

## The pain

Ask any MSP owner what eats the last day of the month and the answer is the books.
On r/msp and r/QuickBooks the same complaints recur: month-end close drags because
you are chasing unapplied payments, deduping "Acme Inc" against "Acme, Inc.", and
trying to prove the ledger actually balances before it goes to the accountant.
Receivables age in the background - by the time you export the A/R Aging Summary,
the 90+ bucket already holds invoices you will fight to collect. And because every
QuickBooks report is point-in-time, you cannot answer "who slipped an aging bucket
since last month?" without keeping your own spreadsheet history. The data is all in
QuickBooks; getting a decision out of it means clicking, exporting, and pivoting.

## What this skill does about it

The skill syncs your company to a local SQLite mirror once, then turns each of those
chores into a single offline question:

- **`ar-aging`** - who owes you, bucketed 0-30 / 31-60 / 61-90 / 90+ and rolled up by
  customer, in one command instead of an export-and-pivot.
- **`invoices stale --days 30`** - a ready-made collections call list: overdue invoices
  ranked by age times balance so the biggest, oldest debts surface first.
- **`reconcile`** - the whole close-hygiene sweep in one findings list: unapplied
  payments, duplicate names, unbalanced journal entries, and inactive records on open
  transactions, with a clean / not-clean verdict.
- **`aging-delta`** - the memory QuickBooks lacks: what changed in AR/AP since your last
  check - who slipped a bucket, whose balance grew, who cleared.
- **`cash-forecast --weeks 4`** - scheduled net cash movement by week so you know whether
  next month's bills clear before you commit to spend.

## Status

Beta. Validated against the QuickBooks Online API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
