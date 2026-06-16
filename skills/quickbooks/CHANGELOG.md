# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - unreleased

### Fixed
- `sync` now pages through the entire book instead of stopping at 1000 rows per
  entity. QuickBooks Online serves every entity through `/query` and caps a page
  at 1000 (`MAXRESULTS`), and the generic sync loop broke after the first page,
  so large books were silently truncated (e.g. 1,000 of 17,940 invoices). Because
  QBO returns rows oldest-first, the missing rows were the *recent* ones, making
  `ar-aging`, `ap-aging`, `dso`, and `balances` wrong, not merely partial. `sync`
  now advances QBO's in-query `STARTPOSITION` until a short page. Verified live
  against a production tenant: all eight resources sync to their exact `count(*)`
  (44,211 records). Recorded as hand-fix `qbo-query-paging`.
- `aging-delta` no longer errors `no such table: aging_snapshots`. The command
  reads and writes an `aging_snapshots` table that no store migration created, so
  it failed on first use. It now creates the table on demand. Recorded as hand-fix
  `qbo-aging-snapshots-table`.

### Added
- `QUICKBOOKS_DB_PATH` environment override for the local SQLite mirror, so a
  single machine can keep separate sandbox and production mirrors. QBO entity IDs
  are per-company and collide across companies, so one shared `data.db` would
  corrupt aggregates after switching environments. Recorded as hand-fix
  `qbo-db-path-env`.

## [0.1.2] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.1] - unreleased

### Fixed
- `sync` now stores rows. It was sending a bare `/query` with no SQL, which
  QuickBooks Online answers with an HTTP 200 `SystemFault` envelope, so every
  resource failed with `missing id for <resource>` and the offline mirror stayed
  empty (`ar-aging`, `ap-aging`, `dso`, `balances`, `reconcile` all returned
  zeros). `sync` now injects `select * from <Entity>` plus `minorversion` per
  resource and unwraps QuickBooks' `QueryResponse.<Entity>` envelope. Verified
  live against a sandbox tenant: 227 records across 8 resources. Recorded as
  hand-fixes `qbo-query-injection` and `qbo-queryresponse-envelope` so a
  cli-printing-press reprint cannot silently revert it.

### Note
- This release caps `sync` at 1000 rows per entity (no STARTPOSITION paging).
  Large production books need press-side pagination; tracked in the hand-fix
  ledger's `spec_encode_followup`.

## [0.1.0]

### Added
- Initial msp-skills release: QuickBooks Online CLI + MCP server.
- Full Accounting entity coverage: accounts, items, customers, vendors, invoices,
  bills, payments, and journal entries (list / get / create / update / delete).
- Offline SQLite mirror via `sync`, with incremental and full resync, plus FTS5
  `search` across every synced entity.
- Receivables and payables intelligence: `ar-aging`, `ap-aging`, `invoices stale`,
  `payments unapplied`, `customers top`, and `vendors spend`.
- Cash and collections KPIs: `balances`, `cash-forecast`, `dso`, and
  `customer-profitability`.
- Month-end hygiene: `reconcile`, `dupes`, `journal-entries check`, and `aging-delta`
  cross-run snapshot diffing.
- Raw `query` passthrough to the QBO query endpoint, agent-native `--json` / `--select`
  / `--agent` output, and `--dry-run` previews on every write.
