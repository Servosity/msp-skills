# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
