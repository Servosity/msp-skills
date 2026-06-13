# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: KnowBe4 KMSAT CLI + read-only MCP server with an
  offline SQLite mirror and full-text search.
- Cross-test, cross-report insight commands the console can't answer:
  `repeat-clickers`, `untrained-clickers`, `risk-drift`, `coverage-gaps`,
  `phish-prone-trend`, `risk-leaderboard`, `group-risk-contribution`,
  `report-rate`, and the one-command `qbr` quarterly review.
- `freshness` to confirm synced data is current before trusting a clicker hunt.
- Optional `events` write path (push/delete custom risk events via the separate
  KnowBe4 User Event API key) and bulk `import`, both preview-able with `--dry-run`.
