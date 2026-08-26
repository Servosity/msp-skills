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

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `GRADIENT_BASE_URL`, `GRADIENT_PARTNER_API_KEY`, `GRADIENT_VENDOR_API_KEY`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: Gradient MSP Synthesize CLI + MCP server covering the
  full vendor API (accounts, services, mappings, vendor, integration, alerts).
- `usage push` - bulk-push a CSV/JSON file of unit counts with a single billing rebuild.
- `usage drift` - local push-ledger report of which accounts' counts changed between
  your last two pushes.
- `alert send --wait` and `alert trace --stuck` - dispatch an alert and confirm (or
  trace) the PSA ticket it should create.
- `hygiene unmapped` work queue and `status ready` go/no-go integration check.
- Offline SQLite mirror (`sync`) with analytics over locally synced data.
