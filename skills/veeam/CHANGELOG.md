# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: Veeam Service Provider Console (VSPC) v3 CLI + MCP
  server with a local multi-tenant SQLite mirror for offline, cross-company queries.
- Cross-tenant rollups: `fleet-health`, `stale-backups`, `company-overview`,
  `at-risk` (RPO), `alarms-triage`, `license-usage`, and `since` (fleet drift).
- Full VSPC v3 surface (~1000 commands): companies, backup servers, jobs, agents,
  protected workloads, alarms, discovery, infrastructure, licensing, and billing.
- Per-instance `VEEAM_BASE_URL` + bearer `VEEAM_TOKEN` auth for appliance-hosted
  consoles; write/infrastructure/destructive/credential tiers gated in governance.md.
