# aws-billing skill - the MSP pain it closes

## The pain

On r/msp, the recurring AWS thread is some version of "the client's bill jumped
and I can't tell them why" - the console shows a 200-line wall of usage-type
codes like `EUC1-DataTransfer-Out-Bytes`, and Cost Explorer answers the question
you didn't ask (buy Reserved Instances) instead of the one you did (what moved,
and where is money leaking). Corey Quinn's *Last Week in AWS* has built an entire
audience on the premise that AWS billing is opaque by design; the FinOps
community treats "make the bill legible to non-specialists" as an unsolved
problem. For an MSP owner reselling or managing AWS for clients, that opacity is
a monthly tax: the cost review gets skipped because nobody can read the bill,
and the waste - idle instances, orphaned volumes, cross-AZ data-transfer bleed -
compounds until the bill is already a surprise.

## What this skill does about it

- **`compare`** - answers "why did the bill change?" by ranking the services that
  moved month-over-month, so the cost review takes a sentence, not a spreadsheet.
- **`consolidated`** - resolves every linked account by name with an inline
  delta, so you can name the account driving org spend in one view.
- **`waste rank`** - one dollar-ranked table of idle EC2, unattached EBS,
  orphaned snapshots, and unassociated Elastic IPs, with a grand total you could
  save - the table to paste into Slack before the bill lands.
- **`waste transfer`** - names exactly where cross-AZ, cross-region, and
  NAT-gateway data-transfer cost is leaking, the line item nobody can find by hand.
- **`explain`** - decodes an opaque usage-type string into plain English, so the
  bill stops being a wall of codes.

## Status

Beta. Validated against the Amazon Web Services API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
