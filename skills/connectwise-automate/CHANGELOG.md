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
  It also dialled the shipped placeholder base URL (`https://{server}/cwa/api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `CONNECTWISE_AUTOMATE_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `CONNECTWISE_AUTOMATE_BASE_URL`.

## [0.1.0]

### Added
- Initial msp-skills release: ConnectWise Automate CLI + MCP server with a local
  SQLite mirror for offline, cross-client queries.
- Cross-client roll-ups the per-server console can't do: `fleet-health`,
  `stale-agents`, `patch-compliance`, `client-rollup`.
- Triage and inventory: `alert-triage`, `os-inventory` (EOL flagging), and `since`
  for overnight drift.
- Full Automate API coverage: computers, clients, locations, alerts, patching,
  scripts, monitors, groups, contacts, and network devices.
- Endpoint and fleet actions (`computers command-execute`, `patching deploy-*`) and
  token minting (`apitoken`), gated as human-in-the-loop in governance.md.
