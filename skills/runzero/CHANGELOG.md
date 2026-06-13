# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `runzero-cli` CLI and `runzero-mcp` MCP server
  for runZero asset discovery and attack-surface management.
- Local SQLite copy of the whole attack surface (`inventory sync`) that powers
  every analysis command offline, at zero additional API quota.
- Cross-entity analysis the live API cannot return in one call: `triage`
  (criticality x services x vulnerabilities), `affected` (CVE-to-asset blast
  radius), `exposure-map`, `software rollup`, `stale`, and `certs-expiring`.
- Point-in-time comparison across syncs: `diff` and `exposure-delta`.
- `scan-watch` to launch a scan on a site and follow it to completion with a
  typed exit code.
- Full-text `search`, agent mode (`--agent`), and the complete runZero
  `account` / `org` API surface.
