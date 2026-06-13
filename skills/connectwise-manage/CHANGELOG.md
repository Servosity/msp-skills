# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.1] - unreleased

### Changed
- Maintenance and packaging updates.

## [0.1.0]

### Added
- Initial msp-skills release: `connectwise-manage-cli` + `connectwise-manage-mcp` covering
  the ConnectWise PSA (Manage) REST surface - service, time, company, finance, project,
  sales, procurement, and system - as typed subcommands and MCP tools.
- Cross-entity views the portal can't compose: `unbilled` (closed/touched tickets with no
  time logged), `account` (company 360 card), `agreement-burn` (hours vs allotment with
  over-limit flag), `board` (grouped triage view), `stale` (no-update tickets, oldest
  first), `workload` (open count + oldest age per tech).
- Typed conditions query builder: `condition build` assembles a validated ConnectWise
  conditions expression from flags; `condition explain` breaks an existing one into clauses.
- Offline SQLite sync (incremental, resumable) with FTS5 full-text `search` and
  `analytics` (count / group-by) over synced data.
- Agent ergonomics: `--agent` mode (JSON, non-interactive), `--dry-run` previews,
  `--select`/`--compact` field control, `profile` saved flag sets, `doctor` health check.
