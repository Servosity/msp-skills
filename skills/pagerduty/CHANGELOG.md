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
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `PAGERDUTY_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `pagerduty-cli` CLI + `pagerduty-mcp` MCP server
  for PagerDuty incident response and on-call management.
- Offline SQLite mirror (`sync`) with FTS5 full-text `search` across synced data.
- Live triage: `pulse` buckets open incidents by service and SLA risk.
- On-call intelligence: `oncall who` (now / next / handoff) and `oncall hours`.
- Post-incident analytics computed offline: `insights mttr` (MTTA/MTTR),
  `insights responders` (workload + off-hours load), `insights noisy`,
  `insights stale`.
- Coverage audits: `audit coverage` (broken escalation chains, single points of
  failure) and `audit schedule-gaps` (future windows with nobody on call).
- Incident forensics: `incidents timeline` and `incidents changes` (what shipped
  right before an incident broke).
- Full PagerDuty REST surface (incidents, services, schedules, escalation
  policies, event orchestrations, and more) with `--agent` JSON mode and
  `--dry-run` previews for writes.
