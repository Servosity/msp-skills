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
- Initial msp-skills release: the HubSpot CLI and MCP server for the terminal and
  any MCP-capable agent.
- Offline SQLite mirror with full-text search - sync your CRM once, then run
  reads against local data with zero API calls.
- Pipeline analytics: `pipeline-health` (per-stage count, dollars, and $ at
  risk), `owner-load` (open-deal load per rep per stage), and `deals top`
  (composite-ranked top-N deals).
- Stale-detection and nurture queues: `stale deals` / `stale contacts`,
  `nurture queue`, and `nurture-mine` for the daily who-to-call list.
- Cross-object engagement timelines via `engagements of` (calls, emails,
  meetings, notes, and tasks for any contact, deal, or company).
- Property-history reporting: `sync --with-history` snapshots, plus
  `meetings ever-had` and `meetings status-report` for "was ever in state X"
  monthly reports.
- Agent output modes: `--agent`, `--json`, `--compact`, `--csv`, and `--dry-run`
  for safe, scriptable, low-token automation.
