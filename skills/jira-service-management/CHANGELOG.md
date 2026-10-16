# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: Jira Service Management Cloud CLI + MCP server covering the
  request API (servicedeskapi) and Assets (CMDB), with a local SQLite copy for offline search.
- `breaches`: open tickets across every queue ranked by SLA time remaining, breached first,
  each ticket counted once.
- `request note`: agent-only internal notes that always post public=false and error if JSM
  stores the comment as public; `request timeline` labels every comment PUBLIC or INTERNAL.
- `roster` and `client`: client organization hygiene and a per-client open-ticket view.
- `sla-report`: breach rate and median time vs goal per desk, request type or SLA metric.
- `cmdb discover`, `cmdb tickets` and `cmdb reconcile`: find the Assets workspace, join Assets
  objects to their tickets, and diff an object type against an RMM or PSA export.
- Pagination sends the real wire names (`start` for JSM; `startAt` and `resultPerPage` for
  Assets) and clamps JSM page size to 50.
- Windows builds: `cmdb reconcile` opens its input file through per-OS code, so the connector
  ships Windows binaries.
