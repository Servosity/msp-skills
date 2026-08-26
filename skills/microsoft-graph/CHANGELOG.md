# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## Unreleased

### Fixed
- **Two test fixtures used the year 2099 to mean "just now".** `security triage` keeps alerts
  raised inside its look-back window and `managed-devices drift` flags devices that have not
  checked in recently, both measured against the current time. The fixtures they read were dated
  `2099-01-01`, which reads as "recent" only while the calendar is short of it: past 2099 the
  alert drops out of the 24-hour window and the device picks up a `stale` reason it was never
  meant to have. Both are now seeded an hour before the clock, and the drift test additionally
  pins that the device is *not* stale, so the seeded sync time is doing real work. No shipped
  behaviour changed - the commands were correct throughout.

## [0.2.1] - 2026-08-26

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
  authenticate. Now declared on every install channel: `MICROSOFT_GRAPH_BASE_URL`.

## [0.2.0] - 2026-07-04

### Changed

- feat(microsoft-graph): read-only apps consent audit + governance-snapshot kit (#177)
- fleet: re-vendor 48 connectors to printing-press 4.24.0 engine (#96)
- chore(fleet): press-version provenance + back-fill hand-fix ledgers (#91)

## [0.1.0]

### Added
- Initial msp-skills release: the `microsoft-graph-cli` CLI and `microsoft-graph-mcp`
  MCP server - a lightweight, cross-platform successor to the retiring mgc (no .NET or
  PowerShell runtime).
- Local SQLite mirror via `pull` (follows `@odata.nextLink`) powering offline cross-entity
  analytics: `licenses waste`, `licenses orphans`, `licenses map`, `admins audit`,
  `security triage`, `managed-devices drift`, `groups risk`, and `tenant snapshot`.
- Read coverage of the MSP-relevant Graph surface: users, groups, directory roles,
  licenses, devices, managed devices, and security alerts/incidents.
- Agent-friendly output (`--agent`, `--json`, `--select`, `--compact`), full-text
  `search`, `export` to JSONL/JSON, named profiles, and `--dry-run` preview on the sole
  write path (`import`).
