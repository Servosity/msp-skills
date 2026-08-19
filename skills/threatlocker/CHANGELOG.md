# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.3.1] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

### Fixed

- **MCP tools are no longer default-denied.** Every tool that is not a Cobra
  mirror - `search`, `sql`, `context`, and the `threatlocker_search` / `threatlocker_get` /
  `threatlocker_execute` code-orchestration trio - returned
  `MCP tenant gate is not configured` instead of running. The generated tenant
  gate treated "no platform source registered" as a failure rather than as
  "nothing to gate", and no connector registers one. The previously released
  binary had 6 dead tools. See issue #249.

## [0.3.0] - 2026-08-14

### Changed

- fix(threatlocker): sync the last four resources, and stop doctor recommending a command that cannot run (#227)

## [0.2.0]

### Fixed
- `sync` stored zero rows for every resource while still reporting success and exiting 0,
  which left the local mirror empty and every offline-backed feature with nothing to read
  (`devices health`, `search`, `analytics`, and any `--data-source local` command).
  Two compounding causes: `sync` was pointed at the ThreatLocker portal's dropdown helper
  endpoints instead of the real list endpoints, and no primary-key field was configured, so
  every row failed id extraction. `sync` now walks the real paginated list endpoints and
  keys rows on their actual id fields. Reported with live-tenant measurements by
  @geekbrownbear in #208.
- `doctor` reported `Cache: stale` with 0 rows for all nine resources as a consequence of
  the above; a completed sync now reports a populated, fresh cache.
- Responses were cached without regard to which customer tenant they belonged to. One
  API token addresses many organizations and the tenant is chosen by a request header,
  so two `--org` runs against the same command could share a cached response and return
  the wrong organization's data. Cached responses are now partitioned by tenant.
- The MCP server never applied tenant scoping at all: it built its API client on a
  different path from the CLI and so ignored `THREATLOCKER_ORG_ID`. Both surfaces now
  share one implementation.
- `sync` could still report success while storing nothing useful. A resource that
  returned rows but stored none, a run that ended on an unreadable response, and rows
  that reached the shared table but failed their per-resource table all counted as
  clean successes. Each is now reported as a warning, so an empty local mirror can no
  longer look like a healthy sync.
- `sync` no longer runs resources that cannot be listed: a duplicate copy of computer
  groups, and scheduled actions, which the API rejects without a parameter sync does
  not send. `applications` and `approvals` now sync from their real endpoints, so
  `applications hunt` and `approvals triage` have data to read.

### Changed
- Regenerated on the printing-press 4.30.2 engine (was 4.24.0). The sync fix above comes
  out of the generator and the spec rather than being hand-maintained: the engine now
  supports POST-with-body list endpoints for sync and resolves per-entity primary keys.
  Also brings corrected pagination across large result sets, robust numeric-ID handling,
  the self-learning `teach`/`recall` surface, and dependency security updates.
- `computers list`, `organizations list`, `applications search` and `approvals list` now
  cover the authenticating organization's whole managed tree by default. One request with the child-organizations flag set returns
  the full tree, so this is the correct default for an MSP and costs no extra API calls.
  Pass `--child-orgs=false` (computers) or `--all-children=false` (organizations) for the
  authenticating organization alone.
- Go toolchain moved to go1.26.6, clearing GO-2026-6218 (see #210).

### Known limitations
- Four resources still store no rows and are tracked separately: `computer-groups`
  (its sync endpoint returns a different shape from the identically-pathed list command),
  `reports` (a grouped wrapper with no row-level key), `online-devices` (returns no rows),
  and `scheduled-actions` (HTTP 417; needs a parameter sync does not send).

## [0.1.0]

### Added
- Initial msp-skills release: the `threatlocker-cli` CLI and `threatlocker-mcp` MCP
  server for the ThreatLocker Portal API, with a cross-tenant offline SQLite mirror.
- Cross-tenant approval triage (`approvals triage`) and one-command approve-across-tenants
  (`approvals approve-batch`), deduping requests by file hash.
- Audit evidence past the 31-day retention cliff: `audit export` (JSONL/CSV, per-tenant
  or all-tenants) and `audit retention-check`, plus `audit drift` for security-relevant changes.
- Fleet health: `devices health` classifies every endpoint online / offline / stale /
  isolated, and `applications hunt` locates a file by hash, certificate, or path across
  every tenant and endpoint.
- Full ThreatLocker write surface (applications, policies, computer maintenance and
  protection) with `--dry-run` previews and `--agent` JSON output for AI agents.
