# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `kaseya-bms-cli` + `kaseya-bms-mcp` covering the full
  Kaseya BMS PSA surface (472 commands across service desk, CRM, contracts, finance,
  projects, inventory, and integrations).
- Offline SQLite mirror with full-text `search` and incremental `sync`.
- Six cross-object analytics not in the BMS console: `queue-health`, `stale-tickets`,
  `workload`, `contract-burn`, `unbilled`, and `pipeline`.
- Token-mint auth via `auth login` (username/password/tenant -> JWT), or a pre-minted
  `KASEYA_BMS_TOKEN`; agent-friendly `--agent` JSON mode and `--dry-run` write previews.
