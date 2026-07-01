# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: the full Resource Guru API as typed CLI commands plus an MCP server.
- Per-day utilization analytics computed locally - `utilization` (booked-vs-available per resource per day, with `--heatmap`), `overbooked` (fleet-wide resource-days over capacity), `bench` (under-utilized resources), and `capacity` (remaining bookable minutes).
- Offline SQLite mirror via `sync`, with `search`, `load`, `stale`, `orphans`, and `since` reading it without extra API calls.
- Agent-native surface: `--agent` mode, `--select` field projection, `--dry-run` previews, and a provenance envelope on store/API reads.
