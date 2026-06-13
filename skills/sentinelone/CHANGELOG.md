# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `sentinelone-cli` CLI and `sentinelone-mcp`
  MCP server for the SentinelOne v2.1 Management API.
- Offline SQLite mirror with full-text `search`, incremental `sync`, and a
  per-sync history snapshot that powers the time-aware analytics.
- Cross-site threat analytics: `threats triage` (ranked worklist),
  `threats blast-radius` (endpoint-joined containment), `threats recurrence`
  (unkilled root causes), `threats mttr` (SLA breaches), and
  `threats verdicts --changed` (verdict/incident flips between syncs).
- Fleet and coverage views: `fleet-health summary` / `fleet-health stale`,
  `coverage gaps`, `versions rollout`, `ranger exposure`, `agents dossier`,
  `exclusions audit`, `sites risk`, and the per-tenant `posture` scorecard.
- `whatchanged` overnight drift report diffing the fleet against an earlier
  snapshot (new threats, agents offline, version and protection-mode changes).
- Agent-first ergonomics: `--agent` JSON mode, `--dry-run` previews,
  `--data-source` (auto/live/local), `--rate-limit`, profiles, and `doctor`.
