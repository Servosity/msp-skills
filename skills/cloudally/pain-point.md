# cloudally skill - the MSP pain it closes

## The pain

CloudAlly backs up a customer's Microsoft 365 or Google Workspace every day, and the
backups are good. Keeping an eye on them across every customer is where an MSP's
morning goes.

The everyday question - "is anyone's backup failing?" - means opening every customer
account in the partner portal and looking. "What is running for this customer right
now?", "which tasks are paused?" and "what did this month's bill come to?" are the same
tour, one customer at a time.

The CloudAlly Management API v2 can answer these, but it is not friendly to script
against: every request needs a client-id and client-secret header plus a bearer token,
partner and company scope are separate logins, acting inside one customer needs a
token minted for that account, and tokens expire. Some documented calls also do less
than the reference suggests: the export endpoint answers 200 with an empty body and
creates no job.

## What this skill does about it

- `cloudally-cli distributor-accounts backup-statuses` (or `reseller-accounts
  backup-statuses`, `distributor-resellers all-backup-statuses`) returns the latest
  backup status of every customer in one call.
- `cloudally-cli auth use-account <accountId>` switches a partner login into one
  customer, so `tasks list`, `tasks get` and `user status` answer for that customer
  without a portal visit; `auth use-account --partner` switches back.
- `cloudally-cli tasks` runs, pauses, activates and cancels backup tasks and their
  entities, and `recovery restore` starts a restore of one entity from a backup date.
- `cloudally-cli distributor-resellers all-bills`, `reseller bills` and `user bills`
  read billing.
- `cloudally-cli sync` mirrors the read endpoints into a local SQLite store for
  offline `search`.

Every command takes `--dry-run` to show the request before anything changes.

## Status

Beta, awaiting live verification. Servosity built this connector for its own
CloudAlly partner tenant and has run it there against live data. That is a first-party
run; the badge flips once a maintainer records it, and an outside MSP's receipt is
still the most valuable one. The command surface is CloudAlly's published Management
API v2, covered by unit and integration tests against simulated CloudAlly responses.
