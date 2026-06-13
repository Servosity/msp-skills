# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated the vendored `acronis-cli` / `acronis-mcp` source from
  cli-printing-press 4.24.0 and re-rendered the templated skill surfaces. No
  changes to command names or flags; the local mirror, search, and cross-tenant
  rollups behave as before.

## [0.1.0]

### Added
- Initial msp-skills release: the `acronis-cli` CLI and `acronis-mcp` MCP server
  for Acronis Cyber Protect Cloud, with an offline SQLite mirror (`sync`) and
  full-text `search`.
- Cross-tenant backup rollups: `health`, `failures`, `freshness`, and
  `agents stale` answer "whose backups failed" and "which agents went offline"
  across every customer tenant in one table.
- Billing and posture views: `coverage --unprotected`, `reconcile usages`,
  `usages drift`, `agents compliance`, `tenants offering-items inventory`, and
  the `customer` 360 card.
- Tenant operations: `tenants` (list/get/create/update/delete/audit), `clients`,
  `agent-manager`, `task-manager`, plus full API coverage via `api`.
