# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
