# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3]

### Fixed
- **A tool-call value could smuggle a refused flag past the MCP server.** The MCP server
  handed each tool argument to the CLI as two separate words, the flag and then its value.
  On a yes/no flag the CLI does not read the next word as a value, so a value that itself
  began with `--` was read as a brand-new flag, including flags the server deliberately
  refuses such as `--deliver`, which can send command output to a URL. Each value now
  travels glued to its flag as one word (`--flag=value`), so it is either rejected outright
  or kept as literal text and can never become a second flag. Reported privately through
  SECURITY.md; the same fix already ships in the DataGate connector.

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

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
