# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `abnormal-cli` CLI and `abnormal-mcp` MCP server for the
  full Abnormal Security REST API (threats, cases, vendors, employees, dashboard aggregations).
- Offline SQLite mirror with full-text search (`sync`, `search`, `analytics`, `tail`).
- Ranked SOC `triage` queue of the newest, highest-severity, still-unremediated threats.
- Joined investigation views: `employee-risk` (profile + Genome identity + 30-day logins +
  open cases) and `vendor-risk` (details + activity + vendor cases).
- Blocking `remediate-watch` that confirms a remediation reached a terminal state, plus a
  consolidated `report-snapshot` for client-ready security reporting.
- Agent-friendly output (`--agent`, `--json`, `--csv`) and a `smoke` check against Abnormal's
  vendor-supplied Mock-Data payloads.
