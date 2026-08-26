# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## Unreleased

### Fixed
- **A test asserted the calendar, not the code.** `changes --since` is measured against the
  current time, but the test that checks "a window later than everything we have matches
  nothing" hard-coded the boundary as `2030-01-01`. On 2030-01-01 that boundary stops being in
  the future, every seeded record falls inside it, and the test fails on the date rather than on
  a change to the connector. The boundary is now computed as one year ahead of the clock, so it
  is later than the fixtures by construction on any run date. No shipped behaviour changed - the
  command was correct throughout.

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
  authenticate. Now declared on every install channel: `ITGLUE_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `itglue-cli` CLI and `itglue-mcp` MCP server for
  the IT Glue / MyGlue API.
- Offline SQLite mirror (`sync`) with FTS5 full-text `search` across every synced
  organization, contact, password (metadata), configuration, and document.
- Documentation- and credential-hygiene analytics the API can't answer:
  `coverage` (completeness ranking), `passwords stale` (rotation audit, metadata
  only), `contacts dupes`, `orphans`, `changes`, and `org show`.
- Read plus non-destructive create/update for organizations, contacts, passwords,
  configurations, and documents; no delete for any IT Glue resource.
- Agent-friendly output (`--agent`, `--json`, `--compact`), `--dry-run` previews,
  saved `profile`s, and `export` / `import` for JSONL.
