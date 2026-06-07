# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: `liongard-cli` and the `liongard-mcp` MCP server,
  covering the full Liongard API surface (environments, systems, launchpoints,
  agents, inspectors, metrics, detections, timeline, users, access keys).
- Offline local SQLite mirror via `sync`, with FTS5 full-text `search` and
  `analytics` over your whole estate.
- Cross-estate rollups that no single API call returns: `drift` (change feed
  joined to environment and system), `health` (one estate scorecard with a typed
  exit code), `launchpoints stale`, `agents offline`, `detections failures`,
  `coverage`, and `inspectors coverage`.
- Reporting helpers: `metrics pivot` (one metric across every system, CSV-ready)
  and `metrics breach` (systems crossing a numeric threshold).
- Per-environment `environments overview` and per-system `systems history` views.
- `--agent` JSON mode on every command, plus `doctor` for auth/connectivity
  checks and `tail` for live change polling.
