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
