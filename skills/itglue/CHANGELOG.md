# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `itglue-cli` CLI and `itglue-mcp` MCP server for
  the IT Glue / MyGlue API.
- Offline SQLite mirror (`sync`) with FTS5 full-text `search` across every synced
  organization, contact, password (metadata), configuration, and document.
- Documentation- and credential-hygiene analytics the API can't answer:
  `coverage` (completeness ranking), `passwords stale` (rotation audit, metadata
  only), `contacts dupes`, `orphans`, `changes`, and `org show`.
- Read plus non-destructive create/update for organizations, contacts, passwords,
  configurations, and documents; no delete for any IT Glue resource.
- Agent-friendly output (`--agent`, `--json`, `--compact`), `--dry-run` previews,
  saved `profile`s, and `export` / `import` for JSONL.
