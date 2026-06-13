# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
