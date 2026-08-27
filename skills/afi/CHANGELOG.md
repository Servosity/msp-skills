# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - 2026-08-26

### Fixed
- **Every sync of the `resources` collection warned and stored nothing in its own table.**
  Afi has a collection literally named `resources`, and the generated database gave its
  table that exact name - the same name the connector already uses for its general-purpose
  mirror of every record. The general-purpose table won, so each sync printed
  `warning: N/N resources items: typed-table upsert failed` and the per-collection table stayed
  empty, which is what offline lookups by tenant read from. The per-collection table is now
  named `tenants_resources`, matching how the connector already names `orgs_tenants`.

  What this means for an existing database: every record you have already synced is still in
  the general-purpose table and still returned by `list`, `search` and `sql` - the rename
  touches nothing there and no existing database needs converting. The new
  `tenants_resources` table starts empty and fills on your next successful `sync`. Rows for
  records the vendor has since deleted will not reappear in it, because a sync only writes
  what the API still returns; those stay readable in the general-purpose table.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

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
