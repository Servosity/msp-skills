# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-08-19

### Fixed
- Open and resolved alerts now cache to the local SQLite mirror. Datto RMM
  identifies an alert only by `alertUid`, which the generic primary-key
  detection did not recognize, so every `sync` consumed alerts and stored none
  of them - `fleet storms`, alert search, and any offline alert query saw an
  empty table while live queries kept working. Reported with a full diagnosis by
  [@zendzipr](https://github.com/zendzipr) ([#258](https://github.com/Servosity/msp-skills/issues/258)).

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
