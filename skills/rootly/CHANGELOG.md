# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

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
