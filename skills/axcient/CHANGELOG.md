# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.11] - 2026-08-26

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

## [0.2.10] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `AXCIENT_BASE_URL`.

## [0.2.9] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.
- `golang.org/x/text` bumped v0.38.0 -> v0.39.0 for **GO-2026-5970** (infinite
  loop on invalid input).

## [0.2.8] - 2026-06-22

### Changed

- fix(axcient): MCP search returns concise projection by default (Refs #101) (#152)

## [0.2.7] - 2026-06-20

### Changed

- fix(axcient): search finds 3-letter all-caps names + MCP search guidance (#148) (#149)

## [0.2.6] - 2026-06-17

### Changed

- fix(axcient): MCP forwards the per-command client fleet filter (#130) (#131)

## [0.2.5] - 2026-06-16

### Changed

- fix(axcient): restore_point sync descends into nested per-vault array (v0.2.5) (#84) (#123)
- chore(deps): patch x/crypto, quic-go, x/sys in axcient + connectwise-control (#120)

## [0.2.4] - 2026-06-16

### Changed

- fix(axcient): search projection + MCP auth-hint + restore_point/autoverify storage-key regression (v0.2.4) (#102)
- fleet: re-vendor 48 connectors to printing-press 4.24.0 engine (#96)
- chore(fleet): press-version provenance + back-fill hand-fix ledgers (#91)

## [0.2.3] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.2.2] - 2026-06-12

### Changed

- fix(axcient): one canonical sync-refresh command + restore_point wiring regression (v0.2.2) (#89)

## [0.2.1] - 2026-06-12

### Changed

- fix(axcient): restore_point sync ID extraction + dependent-name flat-sync failure + strict error names (v0.2.1) (#85)
- feat(maintainer): hand-fix ledger + enforcing gate so connector fixes survive reprints (#81)

## [0.2.0] - 2026-06-11

### Fixed
- MCP numeric path/query parameters (e.g. 7-digit `device_id` / `appliance_id`)
  no longer serialize to scientific notation (`1.234567e+06`), which previously
  returned HTTP 404 and broke every per-device and per-appliance by-id command,
  plus restore_point sync, through the MCP server.
- `sync --dry-run` no longer mutates the local sync-state for dependent
  (cascaded) resources. A preview is now fully side-effect-free, where it
  previously stamped a zero count and a fresh timestamp.

## [0.1.0]

### Added
- Initial msp-skills release: `axcient-cli` and `axcient-mcp` for Axcient
  x360Recover (BCDR), covering the full public API - vaults, appliances, devices,
  jobs, restore points, AutoVerify, and usage.
- Offline SQLite mirror with full-text search, joining device, job, restore-point,
  and client data the per-entity API leaves unconnected.
- Fleet compounds the API can't answer directly: `health` (failed/stale backups
  grouped by client), `client-rollup` (per-client posture), `rpo` (restore-point
  breaches), `compliance` (per-device RPO + AutoVerify evidence, exportable),
  `billing` (per-client usage), and `appliance-map`.
- Agent-native output (`--agent`, `--select`, `--csv`, `--json`), named profiles,
  output delivery sinks, and a public-mock evaluation path (`AXCIENT_BASE_URL`).
