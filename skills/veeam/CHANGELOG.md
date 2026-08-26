# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  It also dialled the shipped placeholder base URL (`https://your-vspc-host:1280/api/v3`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `VEEAM_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

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
