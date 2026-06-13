# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

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
