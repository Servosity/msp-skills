# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

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
