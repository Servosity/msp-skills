# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.5] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.4] - 2026-08-26

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
  authenticate. Now declared on every install channel: `HUBSPOT_BASE_URL`, `HUBSPOT_OWNER_EMAIL`.

## [0.1.3] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.2] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.1] - 2026-06-06

### Changed

- skill: hubspot - UPDATE to 4.22.0 reprint (zero-review pipeline) (#37)
- fix(install): honor GITHUB_TOKEN/GH_TOKEN in fetch_stdout across all skills (#31)
- feat(surfaces): generate every skill-enumerating surface; media on GitHub; Servosity live-verified (#21)

## [0.1.0]

### Added
- Initial msp-skills release: the HubSpot CLI and MCP server for the terminal and
  any MCP-capable agent.
- Offline SQLite mirror with full-text search - sync your CRM once, then run
  reads against local data with zero API calls.
- Pipeline analytics: `pipeline-health` (per-stage count, dollars, and $ at
  risk), `owner-load` (open-deal load per rep per stage), and `deals top`
  (composite-ranked top-N deals).
- Stale-detection and nurture queues: `stale deals` / `stale contacts`,
  `nurture queue`, and `nurture-mine` for the daily who-to-call list.
- Cross-object engagement timelines via `engagements of` (calls, emails,
  meetings, notes, and tasks for any contact, deal, or company).
- Property-history reporting: `sync --with-history` snapshots, plus
  `meetings ever-had` and `meetings status-report` for "was ever in state X"
  monthly reports.
- Agent output modes: `--agent`, `--json`, `--compact`, `--csv`, and `--dry-run`
  for safe, scriptable, low-token automation.
