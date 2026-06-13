# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `sherweb-cli` CLI and `sherweb-mcp` MCP server
  for the Sherweb Partner API (Distributor billing + Service Provider customers,
  subscriptions, platforms, catalog, and orders).
- Composed authentication: OAuth2 client-credentials bearer token plus the APIM
  gateway subscription-key header sent on every call.
- Offline SQLite mirror via `sync` + `deep-sync`, with resumable incremental
  sync and full-text search.
- Cross-entity analytics that join payable and receivable data: `margin`,
  `margin-trend`, `orphans`, `usage-leak`, `right-size`, `drift`, `sub-changes`,
  `fleet-subs`, and read-only `amend-preview`.
- Agent-friendly output (`--agent`, `--json`, `--compact`, `--select`), a
  natural-language `which` command resolver, and a `doctor` health check.
