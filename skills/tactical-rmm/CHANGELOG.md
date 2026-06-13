# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `tactical-rmm-cli` + `tactical-rmm-mcp` for self-hosted
  Tactical RMM, with an offline SQLite mirror and full-text search.
- Cross-entity fleet views the web UI can't assemble: `fleet health`, `triage`,
  `patch posture`, `clients scorecard`, `coverage`, `since`, `checks worst`/`flapping`,
  `alerts digest`, `services down`, `software find`.
- Preview-first cohort actions: `agents bulk-run` and `maintenance set` resolve and
  print the target cohort and run only with `--execute`.
- Typed commands across the full Tactical RMM API (agents, clients, sites, checks,
  alerts, scripts, automation, autotasks, software, services, winupdate, accounts,
  core) with `--agent` JSON output, `--dry-run`, profiles, and `--deliver` sinks.
