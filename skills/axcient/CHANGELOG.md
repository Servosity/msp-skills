# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: `axcient-cli` and `axcient-mcp` for Axcient
  x360Recover (BCDR), covering the full public API - vaults, appliances, devices,
  jobs, restore points, AutoVerify, and usage.
- Offline SQLite mirror with full-text search, joining device, job, restore-point,
  and client data the per-entity API leaves unconnected.
- Fleet compounds the API can't answer directly: `health` (failed/stale backups
  grouped by client), `client-rollup` (per-client posture), `rpo` (restore-point
  breaches), `compliance` (per-device RPO + AutoVerify evidence, exportable),
  `billing` (per-client usage), and `appliance-map`.
- Agent-native output (`--agent`, `--select`, `--csv`, `--json`), named profiles,
  output delivery sinks, and a public-mock evaluation path (`AXCIENT_BASE_URL`).
