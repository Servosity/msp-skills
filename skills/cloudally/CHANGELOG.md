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
- Hardened before the first release after three review rounds of #400 (Codex
  plus fresh-context agents driving the binary against local stubs).
- Credentials: login and refresh calls refuse every redirect, and the portal
  session refuses one off its origin or to http; `auth logout` deletes the
  partner token sidecar; the portal URL must be https.
- MCP: no tool takes a local file path, `download fetch` and `import` are
  CLI-only, and `download link` is off the read-only surface.
- Local data is kept per CloudAlly user (not per token or API key), so a
  refresh never loses the restore ledger and a restricted partner user never
  reads the administrator's cache; an untrusted auth record stops the restore
  commands instead of opening an empty store.
- Restores: a machine-wide guard (`restore-guard.db`) sends each restore once
  across every login, token, date spelling and concurrent run; `--resume`
  keeps the batch's target account and options; `--new-batch` is the only
  override.
- Downloads: owner-only files and folders, no write through a link or over an
  existing file, https links to public addresses only, size-checked bodies,
  ambiguous creates recorded as UNKNOWN, an identical re-run refused, the
  include flags recorded for `--resume`, and `--timeout` no longer cuts off a
  long archive stream.
- Thanks @sonofskywalker3 for the connector, and for the live CloudAlly behaviour
  it documents, in #400.
