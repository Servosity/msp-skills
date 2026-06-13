# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `pandadoc-cli` + `pandadoc-mcp`, covering the full
  PandaDoc Public API (documents, templates, contacts, content library, webhooks,
  workspaces, members).
- Offline SQLite mirror with full-text search via `sync` and `search`.
- Cross-document analytics the API has no endpoint for: `pipeline`, `stalled`,
  `aging`, `value`, `forecast`, `engagement`, `template-stats`, `cold-clients`,
  `followup`, `since`, `webhook-coverage`, and `reminder-gaps`.
- Agent-native output (`--agent`, `--json`, `--select`, `--compact`) and
  `--dry-run` previews for every mutating command.
