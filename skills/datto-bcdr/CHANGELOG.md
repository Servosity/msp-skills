# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: Datto BCDR CLI + MCP server with a local SQLite mirror
  of every device, agent, share, and alert.
- Fleet-wide recovery-assurance commands the per-appliance Partner Portal can't answer:
  `screenshots` (backup-bootability audit), `recoverability` (fresh + screenshot-verified
  KPI), and `stale-backups` (local/offsite snapshot recency).
- Cross-client triage and reporting: `client-risk`, `alert-triage`, `storage-runway`,
  `forgotten-assets`, `agent-versions`, and the QBR-ready `client-report`.
- Offline mirror with full-text `search`, `analytics`, and resumable incremental `sync`;
  agent-native output via `--agent` / `--json` / `--select` and a `doctor` health check.
