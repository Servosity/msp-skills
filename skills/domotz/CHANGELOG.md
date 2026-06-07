# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
