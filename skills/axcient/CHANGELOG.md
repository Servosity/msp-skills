# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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

## [0.2.8] - unreleased

### Fixed
- MCP `search` now returns the concise `id/name/type/match` projection by default
  instead of dumping whole raw records, matching the CLI `search` command and the
  connector guide. The v0.2.4 projection fix only covered the CLI path; the MCP
  `search` tool is a separate hand-wired handler that kept marshalling the full
  records via `db.Search`, so MCP/agent users still saw a wall of nested JSON
  (e.g. complete client objects with all `devices_counters` blocks) through
  v0.2.7. A new `full` boolean parameter restores the whole records, mirroring the
  CLI's `--full`. (reported via #101)

## [0.2.7] - unreleased

### Fixed
- Search now finds records whose name is a 3-letter all-caps label (e.g. `WEB`,
  `SQL`, `DNS`, `DMZ`, `APP`, `VPN`). The local full-text index dropped any
  3-letter all-caps string (a rule meant for currency/country codes like `USD`),
  but it was key-blind, so it also discarded a record's `name`/`alias` when the
  name was a short host label. Those records were fully present (list/get
  returned them) but `search` by name returned nothing, which is why it showed up
  only through the MCP `search` tool. The index filter is now key-aware: it keeps
  the value for display-name fields and still drops bare codes elsewhere. The
  FTS-content schema version is bumped so existing local databases re-index
  through the fixed filter on the next `sync`. (reported by Barret, #148)
- MCP `search` now gives actionable guidance instead of failing opaquely: when the
  local database has not been synced it returns "run the `sync` tool first" rather
  than a cryptic SQLite error, and a no-match returns a clear hint instead of a
  bare `null`. (reported by Barret, #148)

## [0.2.6] - unreleased

### Fixed
- MCP server now forwards the `client` fleet filter to the underlying CLI. The
  client-scoped fleet commands (`health`, `compliance`, `rpo`, `appliance-map`)
  advertise a `client` parameter in their MCP tool schema but the shell-out layer
  silently dropped it, so every MCP call returned the whole fleet regardless of the
  value passed. The CLI binary applied `--client` correctly, only the MCP path did
  not. Cause: the generated `blockedRootFlags` denylist (meant for root
  credential/base-url/config/delivery flags) included `client`, but `client` is not
  a root flag on this connector: it is a per-command Int64 fleet filter. Removed
  `client` from the denylist; the genuine control-plane flags
  (`base-url`/`token`/`config`/`deliver`/`profile`/`args`) remain blocked. Added a
  regression test asserting `client` forwards as `--client <id>` while control-plane
  flags stay dropped. (reported by @Xenith-B, #130)

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
