# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Fixed
- `sync` now fetches **every** page for NinjaOne's after-id (keyset) endpoints
  (`/v2/devices`, `/v2/organizations`, `/v2/locations`, …). These return a bare
  JSON array and paginate via `?after=<lastEntityId>` with no envelope cursor;
  the loop previously stopped at the first full page and reported a truncated
  dataset (e.g. 1,000 of 1,115 devices) as complete, and `--max-pages 0` had no
  effect. The loop now follows the after-id cursor to completion. Envelope-cursor
  endpoints (`/v2/queries/*`) are unaffected.
  Thanks to @AndrewITLive for the report (#88).

### Changed
- Regenerated on the printing-press 4.24.0 engine: robust numeric-ID handling
  and dependency security updates. Same commands and workflows, sturdier local
  mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `ninjaone-cli` CLI and `ninjaone-mcp` MCP server
  for the NinjaOne RMM public API, with OAuth2 client-credentials auth.
- Offline SQLite mirror (`sync`, incremental + resumable) with FTS5 full-text
  `search` so fleet-wide reads run with zero API calls.
- Fleet-wide rollups no single API call returns: `patch-compliance`,
  `backup-coverage`, `av-sweep`, `fleet-health`, `stale-devices`, `os-eol`,
  `software-audit`, and week-over-week `drift`.
- Agent-native output across every command: `--agent`/`--json`/`--select`/`--csv`,
  typed exit codes, and an `agent-context` introspection surface.
