# cloudally skill - the MSP pain it closes

## The pain

CloudAlly backs up a customer's Microsoft 365 or Google Workspace every day, and the
backups are good. Getting data back out at scale is where an MSP's afternoon goes.

The request usually sounds simple: "restore every mailbox to how it was on the 1st",
or, from a lawyer or an auditor, "give us every mailbox as it stood on 31 December,
for each of the last few years". In the CloudAlly portal that is one mailbox, one
date, one wizard at a time. A customer with a hundred mailboxes and four year-ends is
four hundred trips through the same screens, and nothing records which ones were done.

The documented CloudAlly API does not shorten the trip. It reports each mailbox's
**latest** backup only, so it cannot answer "which backup was taken on or before
31 December 2022". The documented export call answers 200 and creates nothing. Past
backup dates and working downloads exist only in the customer portal.

Then the details bite. An MS365 mailbox is not one download but five (mail, calendar,
contacts, tasks, notes), each with its own timestamp. Backup times are stored in UTC,
so the backup CloudAlly stamps 31 December holds the 30th for a US customer. Download
links expire after 60 seconds, so pasting them into a ticket does not work, and a
multi-gigabyte archive cannot be fetched by hand fast enough to matter.

Meanwhile the everyday question - "is anyone's backup failing?" - means opening every
customer account in the partner portal and looking.

## What this skill does about it

- `cloudally-cli backup snapshots` lists every backup a mailbox has in a month, from
  the same calendar the portal's restore wizard reads, reached through the partner
  handoff the partner portal itself uses.
- `cloudally-cli restore plan` resolves every mailbox on a task to its newest
  successful backup at or before a date and shows the plan before anything is sent;
  `restore submit` sends one restore per mailbox with a local ledger, `restore watch`
  follows them, and `restore history` answers "what did we restore for this customer
  last quarter" without calling the API.
- `cloudally-cli download submit` creates one download job per mailbox and sub-source
  for every mailbox backed up on one date (in the customer's own timezone with
  `--utc-offset`), reconciles what the portal actually created, and resumes cleanly;
  `download fetch` mints each 60-second link and streams the archive to disk in one
  step, one folder per year and mailbox.
- `cloudally-cli backup health` finds every stale or failing mailbox and site across
  every customer from one synced feed.

`restore submit`, `download submit` and `download start` print what they would do and
stop until `--yes` is passed. The generated API commands, including `recovery restore`
and every delete, send immediately, so governance.md marks them human-only.

## Status

Beta, awaiting live verification. Servosity built this connector for its own
CloudAlly partner tenant and has run it there in production: backup calendar reads
across customers and a multi-terabyte year-end mailbox export. That is a first-party
run; the badge flips once a maintainer records it, and an outside MSP's receipt is
still the most valuable one. The command surface is CloudAlly's published Management
API v2 plus the customer-portal calls above, covered by unit and integration tests
against simulated CloudAlly and portal responses.
