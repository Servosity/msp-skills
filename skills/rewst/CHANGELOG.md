# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: Rewst GraphQL CLI + MCP server with typed commands
  over the whole schema, an `api` command for full schema coverage, and an offline
  SQLite mirror.
- Six cross-org rollups the gateway has no single endpoint for: `health`,
  `failures`, `dormant`, `roi`, `drift`, and `coverage`.
- Per-region `REWST_BASE_URL` (US default) + bearer `REWST_API_TOKEN` auth.
- Automation/trigger, admin/identity, and credential writes gated human-in-the-loop
  in governance.md (creating/updating triggers and workflows affects live tenants).
