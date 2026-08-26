# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `PIPEDRIVE_BASE_URL`.

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
