# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `xero-cli` + `xero-mcp` for the Xero Accounting
  API across invoices, contacts, accounts, payments, bank transactions, items,
  and the immutable journals feed.
- Offline SQLite mirror with incremental `sync`, FTS5 `search`, and a `since`
  org delta - cross-object analytics computed locally instead of per-question
  API calls.
- Receivables and reconciliation analytics not available in any other Xero tool:
  `aging`, `exposure`, `reconcile`, `bank-recon`, `tie-out`, `ledger`, and a
  one-call `snapshot`.
- Agent-native plumbing: `--agent` mode, `--select` field projection, `--dry-run`
  previews, named profiles, and `--deliver` output sinks.
