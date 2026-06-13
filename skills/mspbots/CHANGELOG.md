# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `mspbots-cli` + `mspbots-mcp` covering the MSPbots
  Public API (dataset and widget reads) as typed subcommands and MCP tools - the
  first published tool we could find for this API (June 2026).
- Local alias registry (`registry add` / `list` / `rm`): name a 19-digit resource
  ID once, use the alias in every other command - the discovery surface the API
  itself doesn't ship.
- Readable filter compiler: `pull --where "Update Date >= 2026-06-01"` compiles
  human-readable predicates into the API's comma-encoded operator DSL (ranges,
  on-or-after, contains, is-empty).
- KPI history the platform doesn't keep: `snapshot` captures timestamped copies
  into local SQLite; `trend` aggregates a numeric column across snapshots
  (sum/avg/count/min/max with deltas); `diff` reports row-level
  added/removed/changed between two snapshots.
- `describe`: samples live rows and infers column names, types, null rates, and
  example values - the metadata endpoint the API lacks.
- Full-table `export` to CSV or JSONL with automatic pagination and an honest
  partial-dump flag when `--max-pages` is hit.
- Agent ergonomics: `--agent` mode (JSON, non-interactive), `--select`/`--compact`
  field control, `profile` saved flag sets, `doctor` health check, offline `sync`
  store with `--data-source` control.
