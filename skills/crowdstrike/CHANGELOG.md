# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `crowdstrike-cli` CLI and `crowdstrike-mcp` MCP server.
- Cross-tenant Flight Control fleet rollups over a CID-keyed local SQLite store:
  `fleet sync`, `fleet scorecard`, `fleet alerts`, `fleet vulns`, `fleet stale`,
  `fleet policy-drift`, `fleet remediate`, `fleet trend`, `fleet tenants`, `fleet search`.
- Per-CID Falcon coverage: alerts (modern Alerts API), devices/hosts, Spotlight
  vulnerabilities, prevention policies, and MSSP CID/user-group management.
- Offline sync to local SQLite with full-text `search` and `analytics`, plus
  agent-native JSON (`--agent`) and `--dry-run` preview on every command.
