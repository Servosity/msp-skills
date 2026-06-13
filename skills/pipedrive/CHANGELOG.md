# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release of the Pipedrive skill: `pipedrive-cli` and the
  `pipedrive-mcp` server.
- Full Pipedrive CRUD across deals, persons, organizations, activities,
  pipelines, stages, leads, products, notes, files, and fields.
- Local SQLite mirror (`sync`) with FTS5 full-text `search`, `export`, and
  `import`, so cross-entity questions answer offline with zero API calls.
- Local-join analytics not in the Pipedrive API: `stale` (deals at risk by
  dollar value), `forecast` (weighted pipeline by stage probability), `aging`
  (deals stuck past their stage's dwell time), `leaderboard` (per-rep
  contribution), `next-activity` (deals with no next step), `lost`
  (re-engagement lists), `dupes` (duplicate detection), `who` (one-card contact
  view), `digest` (standup rollup), and `changes`.
- Agent-ready output: `--agent` for non-interactive JSON, `--dry-run` previews,
  and a `doctor` health check.
