# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-10-19

### Added
- Initial msp-skills release: CloudAlly CLI (`cloudally-cli`) and MCP server
  (`cloudally-mcp`) over CloudAlly's Management API v2, for one company account
  (`auth login`) or every partner customer (`auth login --partner`,
  `auth use-account`).
- Past backup dates: `backup snapshots` lists a mailbox's backups for a month from
  the customer portal's backup calendar, reached through the partner handoff
  (`partner-access-token`), because the documented API reports only the latest
  backup.
- Bulk restores to a date: `restore plan` resolves every mailbox on a task to its
  newest successful backup at or before the date, `restore submit` sends one
  restore per mailbox with a local ledger and safe `--resume`, `restore watch`
  follows a batch and `restore history` reads the ledger offline.
- Mailbox downloads to a date: `download submit` creates one EXPORT job per
  mailbox and sub-source for every mailbox backed up on one date (`--utc-offset`
  selects the account's own calendar day) and reconciles what the portal created;
  `download fetch` mints each 60-second link and streams the archive to disk;
  `download start`, `list`, `watch` and `link` cover one job at a time.
- `backup health` finds stale or failing mailboxes and sites across every customer
  from the synced partner status feed.
- `doctor` verifies the stored token with one authenticated read (`GET /v2/user`)
  and turns the API row red when `base_url` is not the CloudAlly API.

### Security
- Hardened before the first release after a Codex and two fresh-context reviews
  of #400: login and refresh calls refuse every redirect, and the portal session
  refuses one off its origin; no MCP tool takes a local file path, `download fetch`
  is CLI-only and `download link` is off the read-only surface; `auth logout`
  deletes the partner token sidecar; the local store follows the login, not the
  rotating token, so the restore ledger survives a refresh.
- Restores: `--resume` never resends a restore CloudAlly accepted, keeps the
  batch's target account and options, and an identical re-run is refused unless
  `--new-batch` is passed.
- Downloads: owner-only files and folders, no write through a link or over an
  existing file, https links to public hosts only, size-checked bodies, and
  ambiguous creates recorded as UNKNOWN instead of being resent.
- Thanks @sonofskywalker3 for the connector, and for the live CloudAlly behaviour
  it documents, in #400.
