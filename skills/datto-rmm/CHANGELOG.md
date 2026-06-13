# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the Datto RMM CLI and MCP server with full v2 API
  coverage (sites, devices, alerts, jobs, audit, variables).
- Offline SQLite mirror of your whole multi-site fleet, with FTS5 full-text search.
- Fleet-wide analytics no single API call answers: `fleet stale`, `fleet storms`,
  `fleet patch-gaps`, `fleet av-gaps`, `fleet sprawl`, `fleet warranty`,
  `fleet scorecard`, `fleet agent-drift`, `fleet orphans`, and `fleet resolve-storm`.
- `fleet snapshot` and `fleet diff` so any number you report is reproducible later.
- `--agent` JSON mode and `--dry-run` previews for safe agent operation.
