# autotask skill - the MSP pain it closes

## The pain

Ask any MSP owner where Autotask hurts and you get the same two answers: **time
that never gets billed**, and **reporting that means an export**.

Techs close tickets without entering time, or approved hours never make it onto
an invoice - and nobody catches it until the billing run, because Autotask has no
single screen that joins approved time entries to the invoices they belong on. The
hours are real work already delivered; they just quietly fall off the invoice.

And when an owner wants the cross-object picture - how burned is this contract,
what is this client's full footprint, which work is unbilled this month - Autotask's
native **LiveReports** is a copy-and-edit report designer that outputs to Excel, PDF,
RTF, or CSV (per Kaseya's own Autotask PSA reporting help). Anything that joins
tickets to time to contracts to invoices gets pushed into Power BI or another BI
tool; an entire third-party connector market exists for exactly that. The data is
all in the PSA - it just takes a report build and a spreadsheet to see it together.

Underneath, the REST API adds friction of its own: it authenticates with three
static headers (UserName, Secret, and an ApiIntegrationCode) against a per-tenant
zone URL you must discover first, and categorical fields like status and priority
are integer picklist IDs that vary per instance (per Autotask's REST API developer
docs) - so even a plain filter starts with resolving a label to its number.

## What this skill does about it

It syncs Autotask into a local SQLite mirror and answers the cross-object questions
as one offline query - no LiveReport, no export, no API round-trips per question:

- **`autotask-cli unbilled`** - approved time not yet attached to an invoice, the
  revenue-leak answer in one command.
- **`autotask-cli reconcile`** - the month-end billing picture: unbilled time,
  contract burn, and the money left on the table, in one table.
- **`autotask-cli retainer`** - block-hour contracts ranked by percent consumed with
  projected run-out dates, so a retainer never silently goes negative.
- **`autotask-cli company-360 "1234"`** - one company's tickets, contacts, contracts,
  config items, and opportunities, assembled for a client review.
- **`autotask-cli picklist "Tickets" "status"`** - the label-to-ID decoder ring for
  any picklist field, so filters and reports stop being a guessing game.

## Status

Beta. Validated against the Autotask API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
