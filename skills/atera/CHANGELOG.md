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
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `ATERA_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `atera-cli` binary and `atera-mcp` server
  covering the full Atera RMM + PSA API surface (agents, tickets, customers,
  contracts, alerts, devices, rates, contacts, departments, custom fields).
- Local SQLite mirror with `sync` + FTS5 `search` for instant, offline,
  rate-limit-friendly queries.
- Cross-client analytics the live API can't express in one call: `agents stale`,
  `agents inventory`, `agents noisy`, `agents patch-status`, `alerts triage`,
  `tickets sla`, `tickets workload`, `customers book`, `customers coverage`,
  `contracts expiring`, and `since`.
- Agent-native output (`--agent`, `--json`, `--compact`) and typed exit codes
  for scripting and AI-agent use.
