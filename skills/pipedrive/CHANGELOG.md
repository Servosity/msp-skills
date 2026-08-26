# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## Unreleased

### Fixed
- **Two `who` tests depended on a hard-coded date still being in the future.** The command's
  "last activity" and "next activity" are both measured against the current time. One test
  seeded a completed activity dated `2099-01-01` to prove a future-dated one must not be
  reported as the most recent; in 2099 that row becomes the newest *past* activity, wins, and
  the test asserts the opposite of what it is named for. The other seeded an open follow-up
  dated `2030-01-01` and expected it as "next activity"; in 2030 there is no next activity left.
  Both dates are now computed from the clock, so the relationship each test is about holds on
  any run date. No shipped behaviour changed - the command was correct throughout.

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
