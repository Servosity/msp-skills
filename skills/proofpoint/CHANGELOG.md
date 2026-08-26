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
  authenticate. Now declared on every install channel: `PROOFPOINT_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: Proofpoint TAP CLI + MCP server.
- Quota-aware threat ops: `backfill` reconstructs up to 7 days of SIEM click and
  message events by auto-looping the API's mandatory 1-hour windows.
- Cross-endpoint analytics from a local SQLite store: `risk-overlap` (Very Attacked
  People who are also top clickers), `user` (every event touching one person), and
  `incident` (a full brief from a single threatId).
- IOC extraction: `iocs` flattens TAP's nested forensic evidence into a paste-ready
  indicator table; `forensics` and `campaign-threats` pivot from threats to campaigns.
- `url` decode for urldefense-rewritten links (works without credentials), SIEM
  feeds (`siem list-*`), `people` (VAP and top-clicker lists), `sync`/`workflow`
  offline mirror, full-text `search`, and `--agent` JSON output mode.
