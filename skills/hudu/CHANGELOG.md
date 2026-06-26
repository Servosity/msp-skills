# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
