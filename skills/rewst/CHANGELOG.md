# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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

## [0.1.0]

### Added
- Initial msp-skills release: Rewst GraphQL CLI + MCP server with typed commands
  over the whole schema, an `api` command for full schema coverage, and an offline
  SQLite mirror.
- Six cross-org rollups the gateway has no single endpoint for: `health`,
  `failures`, `dormant`, `roi`, `drift`, and `coverage`.
- Per-region `REWST_BASE_URL` (US default) + bearer `REWST_API_TOKEN` auth.
- Automation/trigger, admin/identity, and credential writes gated human-in-the-loop
  in governance.md (creating/updating triggers and workflows affects live tenants).
