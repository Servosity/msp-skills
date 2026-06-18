# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - unreleased

### Fixed
- `sync` now stores correct rows for the device-scoped `/v2/queries/*` report
  endpoints. Every row in this family is keyed by its `deviceId` first, so rows no
  longer collapse. This fixes two distinct symptoms:
  - The endpoints the reporter hit - `queries-antivirus-status`,
    `queries-device-health`, `queries-network-interfaces`,
    `queries-logged-on-users`, `queries-policy-overrides`,
    `queries-raid-controllers`, `queries-raid-drives` - return objects with no
    top-level `id`, so they cached **zero** rows (`all_items_failed_id_extraction`).
  - A second, broader set silently dropped rows **across devices** because the
    generic key fell back to a patch `id` or a `name`: `queries-os-patches`,
    `queries-os-patch-installs`, `queries-software-patches`,
    `queries-software-patch-installs`, `queries-software`,
    `queries-operating-systems`, `queries-computer-systems`, `queries-processors`,
    `queries-volumes`, `queries-windows-services`, `queries-antivirus-threats`
    (e.g. every device running "Windows 11" collapsed to one stored row).
  Each row now keys on `deviceId` plus a per-row discriminator where the resource
  returns many rows per device (`productName`, `threatId`, `interfaceIndex`,
  `userName`, `controllerIndex`, `driveId`, the patch `id`, or `name`); single-row
  reports key on `deviceId` alone. Offline SQL, full-text `search`, and rollups
  like `av-sweep`, `software-audit`, `patch-compliance`, and `os-eol` now see the
  full per-device data. (The four `queries-custom-fields*` report variants are
  tracked separately - their row shape is not yet confirmed, see #137.)
- `sync` now follows the **object-valued** pagination cursor these same
  `/v2/queries/*` endpoints return (`"cursor": {"name": "<token>", …}`). The loop
  previously read only a string cursor, so it stopped after the first page and
  capped each resource at 100 rows (`pagination_cursor_missing`); it now follows
  the `cursor.name` token. String-cursor endpoints are unaffected.
  Thanks to @Xenith-B for the report (#136).

## [0.1.2]

### Fixed
- `sync` now fetches **every** page for NinjaOne's after-id (keyset) endpoints
  (`/v2/devices`, `/v2/organizations`, `/v2/locations`, …). These return a bare
  JSON array and paginate via `?after=<lastEntityId>` with no envelope cursor;
  the loop previously stopped at the first full page and reported a truncated
  dataset (e.g. 1,000 of 1,115 devices) as complete, and `--max-pages 0` had no
  effect. The loop now follows the after-id cursor to completion. Envelope-cursor
  endpoints (`/v2/queries/*`) are unaffected.
  Thanks to @AndrewITLive for the report (#88).

### Changed
- Regenerated on the printing-press 4.24.0 engine: robust numeric-ID handling
  and dependency security updates. Same commands and workflows, sturdier local
  mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `ninjaone-cli` CLI and `ninjaone-mcp` MCP server
  for the NinjaOne RMM public API, with OAuth2 client-credentials auth.
- Offline SQLite mirror (`sync`, incremental + resumable) with FTS5 full-text
  `search` so fleet-wide reads run with zero API calls.
- Fleet-wide rollups no single API call returns: `patch-compliance`,
  `backup-coverage`, `av-sweep`, `fleet-health`, `stale-devices`, `os-eol`,
  `software-audit`, and week-over-week `drift`.
- Agent-native output across every command: `--agent`/`--json`/`--select`/`--csv`,
  typed exit codes, and an `agent-context` introspection surface.
