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
  authenticate. Now declared on every install channel: `AFI_BASE_URL`.

### Changed
- Regenerated the vendored `afi-cli` / `afi-mcp` source from cli-printing-press
  4.24.0 and re-rendered the templated skill surfaces. No changes to command
  names or flags; `fleet-sync`, the fleet-wide coverage/staleness reports, and
  the offboard helper behave as before.

## [0.1.0]

### Added
- Initial msp-skills release: the `afi-cli` CLI and `afi-mcp` MCP server for
  Afi SaaS backup (Microsoft 365 / Google Workspace).
- `fleet-sync` walks the whole Afi hierarchy - installations, orgs, tenants,
  then each tenant's resources, protections, policies, archives, quotas, and
  task stats - into a local SQLite store in one respectful, rate-limited pass.
- Fleet-wide reports that answer offline: `coverage-gaps` (resources with no
  protection), `backup-stale` (protected-but-silently-failing), `fleet-health`
  (all-tenant task + quota rollup), `tenant-scorecard` (one tenant's posture),
  and `reconcile-licenses` (purchased vs protected seats).
- `resolve` maps a Microsoft 365 / Google Workspace ID, email, or name to the
  canonical Afi resource or tenant, including Multi-Geo fan-out.
- `offboard` runs a guarded archive-then-release: it triggers a final backup,
  verifies a fresh archive landed, and only then removes the protection.
- Full public-API coverage via the friendly top-level commands and the `api`
  passthrough, plus `--agent` mode (JSON, non-interactive) for AI agents.
