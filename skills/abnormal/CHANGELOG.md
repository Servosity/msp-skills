# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `ABNORMAL_BASE_URL`.

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
