# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
