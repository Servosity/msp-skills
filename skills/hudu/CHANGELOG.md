# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.6] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.5] - unreleased

### Fixed
- `sync` now detects a repeated `procedure-tasks` primary-key set before
  upserting it, even if Hudu changes item order or non-key fields. Hudu can
  return the complete collection while ignoring both `page` and `page_size`;
  previously, the duplicate-page guard ran only after the same collection had
  been upserted twice. That doubled `sync_complete.total` even though the cache
  contained one distinct-ID set. The reported total is now reconciled to the
  stored row count while paginated Hudu deployments remain fully supported.
  Thanks @Xenith-B (#183).

## [0.1.4] - unreleased

### Fixed
- `sync` now pages the `relations` resource at `page_size=25` instead of the
  global 100. Hudu's `/relations` endpoint returns HTTP 500 when walked at
  page_size 100 beyond the first page, so every `sync` failed on `relations`
  and exhausted its retries. (A direct `relations list` at page_size 100/25/1
  all succeed; only sync's page-2 request at size 100 500s, so `relations` is
  not non-paginated like the IPAM family - it just needs the smaller page.) 25
  is Hudu's documented default page size, and the size the mature
  n8n-nodes-hudu client uses to retrieve every relation, so the `?page=N` walk
  now fetches the full collection with no truncation. `relations list` also
  defaults to `--page-size 25` so manual paging does not hit the same 500.
  Thanks @Xenith-B (#167).

### Added
- `sync --exclude <a,b,c>` skips the named resources, applied after
  `--resources` (or the default set). Ideal for a scheduled daily sync that
  skips slow or rarely-changing resources without having to enumerate every
  resource to keep, e.g.
  `sync --exclude public-photos,procedures,procedure-tasks`. Unknown resource
  names fail loudly. Thanks @Xenith-B (#169).

### Documented
- `procedures`/`procedure-tasks` are not broken (#168): the multi-minute sync
  time is the cost of fetching the full collection now that pagination works
  in v0.1.3 (v0.1.2 silently truncated to the first 100 rows). The
  `pagination_cursor_missing` warning on `procedure-tasks` is a false alarm on
  Hudu's `?page=N` endpoints - verify completeness with the local cache row
  count, which exceeds one page when the data is complete. The SKILL and guide
  now cover keeping a daily sync fast (`--exclude` / `--since` / `--latest-only`
  / `--resources`) and the integration-ID discovery method for `matchers`.
  Thanks @Xenith-B (#168).

## [0.1.3] - unreleased

### Fixed
- `sync` and the `list` commands no longer send `page` to Hudu's IPAM-family
  endpoints (`ip-addresses`, `networks`, `vlans`, `vlan-zones`,
  `rack-storage-items`). The v0.1.2 fix correctly dropped `page_size` but then
  assumed these endpoints page by `page` - they don't: they are non-paginated
  and reject `page` with the same HTTP 400, so every sync after the first page
  failed and stored zero rows. They now fetch the full collection in a single
  request. The non-functional `--page`/`page` MCP arg is removed for them.
  Thanks @Xenith-B (#158).
- `sync` now advances through every page of the paginated resources instead of
  stopping after the first 100 rows. The pagination profile declared
  `cursorType ""`, which disabled the `?page=N` page-int fallback, so any
  resource with more than one page was silently truncated (v0.1.2's new
  "data may be truncated" warning is what surfaced it on `expirations`,
  `folders`, `public-photos`, `relations`, and `procedure-tasks`). Set to
  `cursorType "page"` so the walk runs until a short/empty page. Thanks
  @Xenith-B (#158).
- `sync` now skips `matchers` (with a warning) instead of failing on it every
  run. Hudu's `/matchers` requires an `integration_id` and returns HTTP 500
  without one; a flat sync cannot supply it, and the connector exposes no
  endpoint to enumerate integration IDs. Fetch matchers per integration with
  `hudu-cli matchers list --integration-id <id>`, or
  `hudu-cli sync --resource-param matchers:integration_id=<id>`. Thanks
  @Xenith-B (#159).

## [0.1.2] - unreleased

### Fixed
- `sync` and the `list` commands no longer send the unsupported `page_size`
  parameter to Hudu's IPAM-family endpoints (`ip-addresses`, `networks`, `vlans`,
  `vlan-zones`, `rack-storage-items`). Hudu rejected it with HTTP 400
  ("page_size is not a valid filter parameter.") and those resources stored zero
  rows. They are now paged by `page` alone and drained until an empty page, so
  they sync fully, without truncation. The non-functional `--page-size` flag is
  removed from those five commands. Thanks @Xenith-B for the detailed report (#153).

## [0.1.0]

### Added
- Initial msp-skills release: `hudu-cli` + `hudu-mcp` (123 MCP tools) for the Hudu
  IT-documentation API.
- Offline SQLite mirror (`sync`) with FTS5 full-text `search` over every synced
  resource, plus `--data-source auto|live|local`.
- Documentation-hygiene audits over the mirror: `audit completeness`,
  `audit stale-passwords`, `audit expirations`, `audit stale-articles`,
  `audit layout-drift`, and a worst-first cross-tenant `audit summary`.
- `onboard` to scaffold a new client's asset layouts, folders, and procedures from
  a saved house template (preview by default, `--apply` to write).
- `resolve` a Hudu URL or exact name to its asset/company/layout/relations, and
  `reconcile` PSA/RMM integrator records against live Hudu assets.
- Agent-native output (`--agent`, `--json`, `--compact`, `--select`) for use from
  Claude Code, Codex, and any MCP-capable agent.
