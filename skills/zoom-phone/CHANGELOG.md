# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.1] - 2026-11-06

### Fixed
- Windows: finishing with a saved account's profile lock now closes the lock file before the command carries on, so the accounts folder can be renamed or removed straight afterwards instead of failing with "Access is denied".

## [0.1.0] - 2026-11-06

### Added
- Zoom Phone CLI and MCP server covering the Zoom Phone API (420 MCP tools), with a local SQLite mirror filled by `sync`.
- `e911 audit`: users, common areas and sites with a missing or unverified emergency address, sites with no emergency service location, BYOC numbers needing a carrier update.
- `routing trace` and `routing audit`: where one number rings for business and closed hours, and an account-wide sweep for dead IVR targets, empty call queues, orphaned auto receptionists and missing holiday hours.
- `numbers inventory`, `devices health`, `footprint` and `callbacks`: number and desk-phone hygiene by site, everything one user holds, and external callers still waiting for a call back.
- `offboard`: an ordered removal plan for a departing user; `--apply` sends it after a live re-check of the user and of every group family.
- `accounts connect` and `fleet`: one saved profile per client Zoom account, and any read (or sync) run across all of them with an account column.
- Server-to-Server OAuth (`account_credentials` grant) with the Account ID, Client ID and Client Secret.
- The shared credential kit on both binaries: output, local files, the local store, child processes and outbound requests all pass it, and a credential only goes to the configured API origin and Zoom's token hosts.
