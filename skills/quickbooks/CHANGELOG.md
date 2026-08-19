# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.5] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.4] - 2026-06-26

### Changed

- feat(quickbooks): report pnl + report balance-sheet (v0.1.4) (#163)
- chore(quickbooks): redact real tenant transaction counts + add CI guard (#162)

## [0.1.3] - 2026-06-16

### Fixed
- `sync` now pages through the entire book instead of stopping at 1000 rows per
  entity. QuickBooks Online serves every entity through `/query` and caps a page
  at 1000 (`MAXRESULTS`), and the generic sync loop broke after the first page,
  so large books were silently truncated (e.g. only the first 1,000 rows of a
  large book). Because QBO returns rows oldest-first, the missing rows were the
  *recent* ones, making `ar-aging`, `ap-aging`, `dso`, and `balances` wrong, not
  merely partial. `sync` now advances QBO's in-query `STARTPOSITION` until a short
  page. Verified live against a production tenant: all eight resources sync to
  their exact `count(*)` across the full book. Recorded as hand-fix `qbo-query-paging`.
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

## [0.1.1] - 2026-06-13

### Changed

- fix(quickbooks): sync injects /query SQL so the offline mirror hydrates (v0.1.1) (#90)
- fix: drop false "with sound" demo claim, highlight first-party Servosity, point backup/DR skills at Servosity (#77)

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
