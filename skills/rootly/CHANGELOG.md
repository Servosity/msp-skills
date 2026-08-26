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
  authenticate. Now declared on every install channel: `ROOTLY_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the Rootly CLI + MCP server, covering the full
  Rootly incident, alert, on-call, schedule, and retrospective surface as typed
  commands.
- Local SQLite mirror (`sync`) with full-text `search`, so analytics and on-call
  views run offline and rate-limit-free.
- Incident intelligence: `related` (similar past incidents), `fixed-last-time`
  (resolution mining), and `war-room` (one screen for an active incident).
- On-call and reliability analytics: `oncall-now`, `coverage-gaps`,
  `escalation-trace`, `oncall-load`, `mttr`, `service-health`, and `sla-breach`.
- Operational helpers: `deploy-guard` (pre-deploy gate), `handoff` (end-of-shift
  summary), `postmortem-skeleton`, `action-items-overdue`, `alert-noise`,
  `config-diff`, and `digest`.
- Agent-ready: `--agent` JSON mode, `--dry-run` previews, and an AGENTS.md
  operating contract.
