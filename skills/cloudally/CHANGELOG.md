# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.5] - 2026-11-14

### Fixed
- **The `cloudally_search` MCP tool's description now names this API's resources.** It used to be one sentence shared by every connector, so an MCP host that picks tools by matching words ranked other tools above it for a request like `<vendor> <resource>`. The description now ends with the resource families from the endpoint list, most endpoints first, at most 300 characters ([#629](https://github.com/Servosity/msp-skills/issues/629)).

## [0.2.4] - 2026-11-11

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.2.3] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.2.2] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

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
