# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

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
