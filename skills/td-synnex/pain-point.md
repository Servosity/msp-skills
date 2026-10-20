# td-synnex skill - the MSP pain it closes

## The pain

Microsoft's New Commerce terms only let a partner cut seats or cancel in the first
seven days of a term or a renewal. Miss the renewal and you carry another year of
seats the client may no longer pay for. The tminus365 blog for MSPs
([How I automated subscription renewal notices for Microsoft NCE](https://tminus365.com/how-i-automated-subscription-renewal-notices-microsoft-nce/),
September 2022) describes building a Power Automate flow on a distributor API just to
open a ticket before each renewal, because "its important that you know when renewals
are coming up" and early NCE renewals through a distributor were not aligned to the
calendar month.

The same book causes two more month-end chores for an MSP that buys through TD SYNNEX
StreamOne ION:

- **Margin lives on each subscription.** Price and cost sit on every subscription, so
  finding the customers and SKUs sold at a loss, or below list price, means exporting
  the book and rebuilding the math in a spreadsheet.
- **Seats live in more than one place.** Many MSPs buy from a second distributor
  (Pax8, Sherweb) and bill from a PSA agreement. Lining those seat counts up per
  customer and SKU to catch unbilled seats is manual unless you pay for a separate
  billing-reconciliation platform.

## What this skill does about it

- `td-synnex-cli subscriptions renewals --within 30d --auto-renew off` - every term
  renewing in the next 30 days with auto-renew off, soonest first, so the seat review
  happens inside Microsoft's window.
- `td-synnex-cli margin --by customer` - customers ranked worst margin first, with
  negative, zero and below-list-price rows flagged and unknown costs reported, not
  counted as zero.
- `td-synnex-cli position trueup --with pax8-positions.csv --billed psa-agreements.csv`
  - TD SYNNEX seats next to Pax8 seats and PSA billed seats per customer and SKU, with
  rows it cannot match listed instead of guessed.
- `td-synnex-cli orders stuck --older-than 24h` - orders still open after a day, and
  orders that failed, with a flag for the ones you can still cancel.
- `td-synnex-cli subscriptions set "10505" "108536" "1608355" --seats 12` - builds the
  exact UPDATE order for a seat change and places it only with `--confirm`.

## Status

Beta. Checked against TD SYNNEX's published v3 API documentation and a mock ION
server; not yet run against a live tenant (see the README's Known gaps). The
closed-loop receipt (a named MSP running it live in their production tenant at a Build
Session) is tracked separately and added here as `video.md` once it exists.
