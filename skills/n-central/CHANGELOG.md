# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `N_CENTRAL_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `n-central-cli` + `n-central-mcp` covering the
  N-able N-central REST API - devices, customers, sites, service orgs, org
  units, users, access groups, scheduled tasks, and server info - as typed
  subcommands and MCP tools.
- Offline SQLite mirror of the org tree (`sync`), with `whereis` (device name
  fragment to full server > service org > customer > site path) and `fanout`
  (one full-text query unioned across every server's mirror - the multi-tenant
  search no console has).
- NOC rollups the console can't compose: `triage` groups active issues by
  customer, device, or monitor and ranks by severity; `props audit` reports
  custom-property coverage gaps by customer; `maint coverage` finds devices
  with no maintenance window before a patch wave.
- `guardian`: a CI-wireable health check that validates the access token,
  tracks the API user's password expiry (the 90-day default that silently
  invalidates the JWT), and detects N-central's HTTP-200-with-error-body
  responses.
- Full-text `search` and `analytics` (count/group-by) over synced data; `export`
  to JSONL/JSON; `import` from JSONL with opt-in `--dry-run` preview.
- Agent ergonomics: `--agent` mode (JSON, non-interactive), `--select`/`--compact`
  field control, `profile` saved flag sets, `doctor` health check,
  `--allow-partial-failure` for N-central's partial-failure response bodies.
