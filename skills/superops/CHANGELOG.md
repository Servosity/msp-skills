# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.6] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

## [0.1.5] - 2026-08-26

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
  authenticate. Now declared on every install channel: `SUPEROPS_BASE_URL`, `SUPEROPS_REGION`, `SUPEROPS_SUBDOMAIN`.

## [0.1.4] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.3] - 2026-06-17

### Changed

- fix(superops): restore CustomerSubDomain header + EU region routing (#132) (#134)

## [0.1.2] - 2026-06-16

### Changed

- docs(superops): consolidate 0.1.1 fix into the 0.1.2 release section (#114)
- fix(superops): request scalar GraphQL fields without sub-selections (#114) (#126)
- fleet: re-vendor 48 connectors to printing-press 4.24.0 engine (#96)
- chore(fleet): press-version provenance + back-fill hand-fix ledgers (#91)
- fix: drop false "with sound" demo claim, highlight first-party Servosity, point backup/DR skills at Servosity (#77)
- fix(install): honor GITHUB_TOKEN/GH_TOKEN in fetch_stdout across all skills (#31)

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
