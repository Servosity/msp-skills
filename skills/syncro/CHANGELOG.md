# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `syncro-cli` + `syncro-mcp` for Syncro PSA and RMM.
- Local SQLite mirror with full-text `search` and `sync` for offline,
  cross-customer analysis that never touches your API rate limit.
- Billing-leakage analytics: `billing uninvoiced`, `billing drift`,
  `billing ar-aging`, and `customers margin`.
- Service-desk and RMM rollups: `tickets aging`, `assets patch-gaps`,
  `alerts noise`, `alerts orphans`, and the cross-entity `customers profile` card.
- Full PSA + RMM command surface (tickets, invoices, estimates, contracts,
  customers, assets, RMM alerts) with `--agent` JSON mode and `--dry-run`
  preview for every write.
