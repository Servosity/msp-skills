# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release of the Better Stack connector: `betterstack-cli` and
  the `betterstack-mcp` MCP server, covering the full Better Stack Uptime surface -
  monitors, heartbeats, incidents, on-call calendars, policies, monitor/heartbeat
  groups, and status pages (list/get/create/update/delete, plus incident
  acknowledge/resolve).
- Offline SQLite mirror with `sync` and FTS5 full-text `search`, so cross-resource
  questions answer from local data instead of repeated API calls.
- Cross-object analytics the API can't answer in one call: `fleet`, `down`,
  `coverage`, `mttr`, `flapping`, `oncall-gaps`, `heartbeat-risk`,
  `statuspage-audit`, `group-health`, and `triage`.
- `--agent` mode (JSON, non-interactive) and `--dry-run` previews for every
  mutating command; `export`/`import` for JSONL backup and migration.
