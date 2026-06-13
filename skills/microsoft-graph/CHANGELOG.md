# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

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
