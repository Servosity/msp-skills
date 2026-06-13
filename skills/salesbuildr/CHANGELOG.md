# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `salesbuildr-cli` + `salesbuildr-mcp` covering the
  full Salesbuildr Public API - companies, contacts, products, opportunities,
  quotes, pricing books, and templates.
- Offline SQLite mirror with full-text `search`, `sql`, `export`, and `sync`.
- Cross-object analytics with no portal equivalent: `quote stale`, `quote thin`,
  `quote funnel`, `pricing drift`, `opportunity velocity`/`winrate`/`mrr-forecast`,
  `product velocity`, `company whitespace`, and `reconcile-psa`.
- Agent-native output (`--agent`, `--json`, `--dry-run`) and MCP server for any
  MCP-capable agent.
