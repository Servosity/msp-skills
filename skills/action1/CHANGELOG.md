# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `action1-cli` + `action1-mcp` covering the full
  Action1 API (endpoints, updates, vulnerabilities, automations, scripts,
  software repository, organizations, users, roles, reports, audit).
- Fleet-wide views the org-siloed API cannot return in one call: `fleet
  patch-posture`, `fleet vuln-triage` (CVSS + CISA KEV), `fleet stale`, `fleet
  org-scorecard`, `fleet reboot-pending`, `fleet health-score`, `fleet
  software-rollup`, `fleet patch-drift`, and `fleet automation-health`.
- Offline SQLite mirror with `sync`, full-text `search`, `analytics`, and JSONL
  `export`; `--agent` JSON mode and `--dry-run` previews for safe automation.
- OAuth2 client-credentials auth that mints and refreshes the bearer token
  automatically, with `us`/`eu`/`au` region selection.
