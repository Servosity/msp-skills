# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.4] - 2026-07-10

### Fixed
- The `.mcpb` extension now installs via Claude Desktop → Settings → Extensions. The published manifest previously failed to install for three reasons, all corrected: (1) non-spec top-level keys (`printing_press_version`, `press_provenance`) made Claude Desktop's validator reject the whole manifest - provenance now lives under the spec's `_meta` slot; (2) only one of the required credentials was declared - all five (`CW_COMPANY_ID`, `CW_PUBLIC_KEY`, `CW_PRIVATE_KEY`, `CW_CLIENT_ID`, `CW_SITE`) are now in `user_config` and wired into the MCP server env; (3) the server command pointed at a binary name the release never ships (`ENOENT`) - `platform_overrides` now select the correct per-OS binary. Reported by @jborello (#185).

### Changed
- Bumped the Go toolchain to go1.26.5 (clears the `crypto/tls` standard-library advisory GO-2026-5856).

### Known limitations
- MCPB `platform_overrides` has no CPU-architecture axis, so this release covers Apple-Silicon macOS and amd64 Linux/Windows; Intel macOS and arm64 Linux still need a launcher shim or universal binaries (tracked upstream in cli-printing-press).

## [0.1.3] - 2026-06-22

### Fixed
- `workload` no longer times out on tenants with years of ticket history: it now loads **only open tickets** (filtered in SQL via `cwLoadOpenTickets`) instead of the full history, so it scales with open-ticket count. Reported by @Xenith-B (#146).
- `search` in auto mode no longer hangs on a slow live API for a large tenant: the live-search leg is now bounded and falls back to local FTS on a timeout (not only on a network error). Reported by @Xenith-B (#146).

### Changed
- Documented the **large/historical tenant playbook** across SKILL.md, guide.md, README, and the MCP `context` tool's `query_tips`: scope a heavy sync with a conditions filter (`sync --param "conditions=lastUpdated > [<iso>]"`), keep reads local (`--data-source local` / `--type`), and raise `--timeout` on big datasets. Corrected the prior claim that sync is incremental - ConnectWise list endpoints declare no temporal-filter parameter, so a plain sync re-fetches in full and `--since` has no effect on the sync itself (#146).
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.1] - unreleased

### Changed
- Maintenance and packaging updates.

## [0.1.0]

### Added
- Initial msp-skills release: `connectwise-manage-cli` + `connectwise-manage-mcp` covering
  the ConnectWise PSA (Manage) REST surface - service, time, company, finance, project,
  sales, procurement, and system - as typed subcommands and MCP tools.
- Cross-entity views the portal can't compose: `unbilled` (closed/touched tickets with no
  time logged), `account` (company 360 card), `agreement-burn` (hours vs allotment with
  over-limit flag), `board` (grouped triage view), `stale` (no-update tickets, oldest
  first), `workload` (open count + oldest age per tech).
- Typed conditions query builder: `condition build` assembles a validated ConnectWise
  conditions expression from flags; `condition explain` breaks an existing one into clauses.
- Offline SQLite sync (incremental, resumable) with FTS5 full-text `search` and
  `analytics` (count / group-by) over synced data.
- Agent ergonomics: `--agent` mode (JSON, non-interactive), `--dry-run` previews,
  `--select`/`--compact` field control, `profile` saved flag sets, `doctor` health check.
