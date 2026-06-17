# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.5] - unreleased

### Fixed
- `restore_point` sync now populates the typed table against the real live API
  shape. The per-device endpoint groups restore points by Cloud Vault: a
  top-level array of `{vault_id, restore_point:[...]}` wrappers whose
  timestamp-string `restore_point_id` lives only on the inner array elements.
  Sync was storing the outer vault wrappers (no `restore_point_id`), so every one
  failed id extraction (`all_items_failed_id_extraction`) and the table stayed at
  0 rows, even though fleet summaries and live per-device lookups were unaffected.
  `syncDependentResource` now descends one level into the nested `restore_point[]`
  array, carrying the wrapper's `vault_id` down onto each point and keying it as
  `rp:<device_id>:<restore_point_id>`. The earlier flat-array regression fixture
  (0.2.1/0.2.2) never reproduced this because it fed the inner shape directly; the
  wiring test now drives the real nested grouped-by-vault payload. (#84)

### Tests
- Replaced the flat-array `restore_point` sync-wiring fixture with the live
  grouped-by-vault nested shape (the test now fails without the flatten), and
  added a flat-array passthrough test so the single-device write-through path
  stays covered. Added the nested-flatten marker to the hand-fix ledger so a
  future reprint cannot silently drop it.

## [0.2.4] - unreleased

### Fixed
- `restore_point` and `autoverify` rows are keyed by their clean synthesized
  composite again (`rp:<device_id>:<restore_point_id>`, `av:<device_id>:...`).
  The 0.2.3 engine refresh added a generic parent-composite storage key that
  appended `<id>\x00<parent>` for every parent-scoped resource, which double-keyed
  these two (whose synthesized id already embeds the parent) into a NUL-containing
  key that broke clean-key offline lookups. The parent append is now skipped when
  the id already carries the `rp:`/`av:` prefix; a bare native id still gets the
  parent composite so same-id-different-parent rows don't collapse. (#101)

### Changed
- `search` now returns a concise `id/name/type/match` projection by default
  instead of dumping whole raw JSON records (a token sink for agents). Pass
  `--full` for the whole records. (#101)
- MCP tool auth-error hints no longer tell MCP-only users to "Run `axcient-cli
  doctor`" (a CLI command they can't reach) and instead point to setting
  `AXCIENT_API_KEY` in the MCP server's configuration. The CLI's own error hints
  still reference `doctor`, which is correct for CLI users. (#101)

### Tests
- Restored the hand-authored regression tests for the `restore_point`,
  `autoverify`-sync, and `--strict`-naming fixes that the 0.2.3 engine refresh
  silently dropped, and added them to the hand-fix ledger so a future reprint
  cannot drop them again. Added tests for the concise `search` projection.

## [0.2.3] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.2.2] - unreleased

### Changed
- Standardized the local-store refresh step on one canonical command, bare
  `axcient-cli sync`, across the quick-start, the morning-sweep recipe, and every
  fleet command's help text and empty-store hint. Previously the docs and
  per-command hints suggested five different `--resources` scopes
  (`clients,device`, `clients,device,autoverify`, `clients,device,appliance`,
  `clients,device,vault,appliance`, and bare), so "refresh the mirror" was
  ambiguous depending on which surface you followed. Bare `sync` pulls every
  top-level resource plus all dependents (restore points, AutoVerify, client-device
  maps), so it is the complete, unambiguous refresh. (Reported in #86.)

### Tests
- Added a sync-wiring regression test that drives the per-device restore_point
  endpoint with the live item shape (timestamp-string `restore_point_id`, no
  `device_id`) through `syncDependentResource`, proving the typed table populates
  with the `rp:<device_id>:<restore_point_id>` key - end-to-end coverage that
  complements the store-layer `restore_point` tests shipped in 0.2.1.

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
