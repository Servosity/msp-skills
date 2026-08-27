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

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

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
  authenticate. Now declared on every install channel: `ACTION1_BASE_URL`, `ACTION1_OAUTH2`, `ACTION1_ORG_ID`.

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
