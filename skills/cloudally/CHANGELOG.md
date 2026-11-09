# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.1] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.2.0] - 2026-11-05

### Added
- Past backup dates: `backup snapshots` lists one mailbox's backups for a month from
  the customer portal's backup calendar, reached through the partner handoff
  (`partner-access-token`), because the documented API reports only the latest
  backup per mailbox.
- Restores to a date: `restore plan` resolves every mailbox on a task to its newest
  successful backup at or before each date, `restore submit` sends one restore per
  mailbox per date with a local ledger and a `--resume` that never resends an
  accepted or unknown restore, `restore watch` follows a batch to completion and
  `restore history` reads the ledger offline.
- Mailbox downloads to a date: `download submit` creates one EXPORT job per mailbox
  and sub-source for every mailbox backed up on one date (`--utc-offset` selects the
  account's own calendar day, `--latest` each mailbox's newest backup) and reconciles
  what the portal created; `download fetch` mints each 60-second link and streams the
  archive to disk; `download start`, `download list`, `download watch` and
  `download link` cover one job at a time.
- `backup health` finds stale or failing mailboxes and sites across every customer
  from the synced partner status feed.
- `CLOUDALLY_PORTAL_URL` names the customer portal origin (default
  `https://app.cloudally.com`).

### Changed
- Download jobs are matched by label: every create sends a unique `jobLabel`
  (`cloudally-cli <id>`, with `download start --label` text after it), and a row is
  confirmed only by the portal job carrying it (or, where the portal drops labels, by
  the one job of that snapshot created after the row was requested).
- Calendar timestamps: an MS365 mailbox is downloaded one sub-source at a time, each
  asked for with that sub-source's own timestamp from the backup calendar, never the
  run's headline time.
- Restore guard: `restore-guard.db` in the state directory sends each restore (task,
  mailbox, resolved backup) once across every login, token, date spelling and
  concurrent run that uses it, and two resolved backups of one mailbox within 30
  minutes of each other count as the same backup. `--new-batch` is the only override.
- Download guard: `download-guard.db` in the state directory claims each archive
  (account, task, mailbox, sub-source, snapshot, format and include flags) before its
  create, so no two batches or runs create the same archive.
- Ledgers written by pre-release builds of these commands are not imported into the
  guards. While a restore ledger or download batch ledger the guard has no record of
  is on disk, `restore submit`, `download submit` and `download start` refuse before
  any request, exit 2 and name the file; `restore history`, `restore watch` and
  `download fetch --batch` still read it. A v0.1.0 install has no such file.
- The credential kit covers the portal and download paths: the portal handoff token,
  session cookie and CSRF token are held and go only to `https://app.cloudally.com`
  or the API origin, so a portal on any other origin is refused before anything is
  sent. A presigned download link is not held (`download link` exists to show it),
  and archive bytes stream to `--dest` outside the kit.
- MCP: `CLOUDALLY_MCP_READ_ONLY=1` also removes `restore_submit`, `download_submit`,
  `download_start` and `download_link`; over MCP the submit and start tools only
  preview, because `--yes` is never passed; `download fetch`, `import` and
  `local-data purge` are CLI-only.
- Thanks @sonofskywalker3 for the connector's restore and download workflows, and
  for the live CloudAlly behaviour they document, in #400.

## [0.1.0] - 2026-10-19

### Added
- Initial msp-skills release: CloudAlly CLI (`cloudally-cli`) and MCP server
  (`cloudally-mcp`) over CloudAlly's Management API v2: distributor, reseller and
  customer accounts, backup tasks and their entities, backup status feeds, users,
  billing and recovery jobs.
- `auth login` signs in as one company account; `auth login --partner` signs in to
  the partner portal and reaches every customer. Both send the `client-id` and
  `client-secret` headers on every request and refresh the token on their own.
- `auth use-account <accountId>` mints an account-scoped token for one customer
  (`--reseller` for reseller partners) and `auth use-account --partner` switches back.
- `CLOUDALLY_MCP_READ_ONLY=1` lists only read-only MCP tools; `cloudally_read` keeps
  live reads available and refuses every state-changing endpoint.
- `local-data purge` previews and erases the local store, response cache and learn
  journal; `CLOUDALLY_NO_LOCAL_DATA=1` keeps API responses off the disk.
- `doctor` verifies the stored token with one authenticated read (`GET /v2/user`)
  and turns the API row red when `base_url` is not the CloudAlly API.
