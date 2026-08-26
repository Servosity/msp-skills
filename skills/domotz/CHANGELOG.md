# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  It also dialled the shipped placeholder base URL (`https://api-{region}.domotz.com/public-api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `DOMOTZ_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `DOMOTZ_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `domotz-cli` + `domotz-mcp`, covering every Domotz
  Public API endpoint (agents, devices, variables, sensors, alerts, topology, RBAC).
- Cross-fleet rollups the agent-scoped API can't do in one call: `fleet health`,
  `fleet offline`, `fleet new`, `fleet inventory`, `fleet ip-conflicts`,
  `fleet unmonitored`, `fleet breakdown`, `fleet speedtest`, and more.
- Local SQLite mirror (`sync`) with full-text `search`, offline `topology`, and
  config/inventory `drift` between snapshots.
- Agent-native output modes (`--agent`, `--json`, `--csv`, `--select`) and a
  `doctor` health check.
