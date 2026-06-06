# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
