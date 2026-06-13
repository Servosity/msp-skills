# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: Gradient MSP Synthesize CLI + MCP server covering the
  full vendor API (accounts, services, mappings, vendor, integration, alerts).
- `usage push` - bulk-push a CSV/JSON file of unit counts with a single billing rebuild.
- `usage drift` - local push-ledger report of which accounts' counts changed between
  your last two pushes.
- `alert send --wait` and `alert trace --stuck` - dispatch an alert and confirm (or
  trace) the PSA ticket it should create.
- `hygiene unmapped` work queue and `status ready` go/no-go integration check.
- Offline SQLite mirror (`sync`) with analytics over locally synced data.
