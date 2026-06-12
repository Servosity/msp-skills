# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.1] - unreleased

### Fixed
- `restore_point` sync no longer stores zero rows. The live x360Recover endpoint
  keys recovery points by a timestamp string (`restore_point_id`,
  `YYYY_MM_DD_HH_MM_SS`) with no numeric `id`; ID extraction now recognizes it
  and composes a fleet-collision-safe `rp:<device_id>:<restore_point_id>` key, so
  the restore-point table (and the offline/fleet history that depends on it)
  populates. The live `device restore-point` write-through caches the same items
  instead of warning "not cached locally".
- `sync --resources <dependent>` (e.g. the documented
  `--resources clients,device,autoverify`) no longer reports a spurious failure.
  Dependent resources (`autoverify`, `restore_point`, `client_device`) have no
  flat list endpoint and are synced per-parent; naming one previously enqueued it
  as a flat resource that failed ("unknown sync resource"), visible under
  `--strict` as `1 resource(s) failed to sync`. They are now excluded from the
  flat pass and still sync via the parent cascade.
- `sync` failure errors now name the failing resource(s)
  (`N resource(s) failed to sync: <names>`) instead of only a count, making a
  `--strict` failure diagnosable.

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
