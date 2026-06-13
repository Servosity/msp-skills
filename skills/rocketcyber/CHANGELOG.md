# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `rocketcyber-cli` + `rocketcyber-mcp` for the
  RocketCyber Customer API v3 (incidents, agents, detection events, Defender and
  Microsoft 365 posture, firewalls, suppression rules, CSV report export).
- Local SQLite mirror with full-text `search` and incremental `sync` for offline,
  multi-account rollups.
- SOC analytics the console doesn't compute: `triage` (cross-account board),
  `agents stale`, `incidents mttr`, `defender riskiest`, `office trend`, and
  `suppression audit`.
- `--agent` JSON output for AI-agent use; `import` with `--dry-run` for the only
  write path.
