# appdirect skill - the MSP pain it closes

## The pain

On r/msp, billing reconciliation for resold SaaS is a recurring complaint: threads
about "billing reconciliation nightmare," monthly true-ups, and **margin leak from
subscriptions that are active in the marketplace but never made it onto an invoice**.
The pattern MSP owners describe is always the same - the marketplace console shows
one company on one screen, so catching an un-invoiced subscription, a failed payment,
or a stalled deal means clicking through hundreds of company billing pages by hand.
Nobody has time, so the reconciliation slips, and the leak compounds every
month-close.

AppDirect powers a large slice of the distributor and telco marketplaces MSPs resell
through. Its REST API can answer these questions - but only one record at a time,
behind an OAuth token that expires hourly, with no cross-company rollup. So the data
exists and is still effectively unusable for the question that actually matters:
*"across every company, what's wrong with my billing right now?"*

## What this skill does about it

- **`reconcile --since 30d --agent`** - flags active subscriptions with no matching
  invoice, overdue invoices, and failed payments across every company in one call,
  before month-close.
- **`payments unpaid --since 7d --json`** - the weekly failed-payment chase as one
  sorted list instead of a tour through company billing screens.
- **`subs changed --since 7d --json`** - new, ended, and suspended subscriptions
  across the whole marketplace for churn and change review.
- **`company show <companyId>`** - one customer's users, subscriptions, invoices, and
  open opportunities in a single view, for renewal and support prep.
- **`pipeline stale --days 14 --json`** - surfaces open assisted-sales opportunities
  that have gone quiet before they die.

Everything answers from a local SQLite mirror (`sync`), so the questions are
instant, offline, and don't burn API quota.

## Status

Beta. Validated against the AppDirect API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
