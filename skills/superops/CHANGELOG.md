# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Fixed
- Typed `list`/`get` commands (`clients`, `assets`, `tickets`, and every other entity) no longer fail with GraphQL `SubSelectionNotAllowed` errors. The queries requested association/enum fields (`accountManager`, `primaryContact`, `hqSite`, `client`, `site`, `status`, `priority`, `requester`, `technician`, and others) with object sub-selections, but the SuperOps schema returns those fields as scalar leaf types (JSON/String); they are now requested as the scalars they are. Thanks to @AvlCompCo for the detailed report (#114).

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `superops-cli` + `superops-mcp` covering the SuperOps
  PSA+RMM GraphQL surface - tickets, assets, alerts, clients, sites, users, contracts,
  invoices, worklogs, technicians, service items, IT docs, and KB - as typed
  `list`/`get` subcommands and MCP tools.
- Cross-entity views the console can't compose in one screen: `sla-watch` (open tickets
  breaching or near their resolution SLA, grouped by tech or client), `client-360` (one
  client's sites, users, contracts, open tickets, assets, and open invoices), `unbilled`
  (billable logged worklog totaled per client - the month-end reconciliation target),
  `at-risk-assets` (endpoints both unpatched and actively alerting), `alert-coverage`
  (alerts split resolved vs unresolved per client), `stale-tickets` (open tickets idle
  past N days), and `context-ticket` (one ticket + worklogs + client + SLA as an
  agent-shaped, `--select`-friendly bundle).
- Offline SQLite sync (`sync`, incremental and resumable; access-denied resources are
  warnings, not failures) with FTS5 full-text `search` and `analytics` over synced data.
- Read-only by design: every typed command reads; the single write path is `raw mutation`
  (with `raw query` for reads), and `--dry-run` previews the exact GraphQL request.
- Agent ergonomics: `--agent` mode (JSON, non-interactive), `--select`/`--compact` field
  control, `--deliver` output sinks, named `profile` flag sets, `which` capability lookup,
  and `doctor` for an auth/connectivity health check.
