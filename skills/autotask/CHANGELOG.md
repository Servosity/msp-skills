# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Fixed
- `sync` no longer fails on the first page of every POST-query resource. Autotask
  requires a filter on `query` endpoints and rejected the filterless first request
  with HTTP 500 `Value cannot be null. Parameter name: filters`, so companies,
  contacts, tickets, projects and the other 39 POST resources never synced a single
  row. The first page now carries the same `id`-walk condition every later page
  already sent. Reported by @zendzipr in
  [#257](https://github.com/Servosity/msp-skills/issues/257).
- `--param` / `--resource-param` overrides for array and comma-separated body
  fields (`filter`, `includeFields`) are now sent as JSON arrays instead of the
  literal string, matching what the `query` commands have always done.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `autotask-cli` CLI and `autotask-mcp` MCP server
  for Datto Autotask PSA, covering every Autotask REST entity plus zone discovery
  and an incremental local SQLite mirror with full-text search.
- Cross-object views computed offline from the mirror: `unbilled`, `reconcile`,
  `retainer`, `contract-burn`, `ticket-aging`, `sla-breaches`, `triage`, `workload`,
  `stale`, `since`, `account-brief`, `company-360`, `project-health`, and `data-gaps`.
- `picklist` decoder for resolving Autotask's integer picklist IDs to labels, and
  `--agent` / `--dry-run` controls for safe agent operation.
