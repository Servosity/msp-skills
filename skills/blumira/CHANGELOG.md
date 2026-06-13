# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `blumira-cli` CLI and `blumira-mcp` MCP server,
  vendored source-only under `cli/` (Apache-2.0).
- Offline SQLite mirror via `sync`, with full-text `search` and `evidence-search`
  over synced findings, evidence, agents, and detection rules.
- Cross-account MSP analytics no single API call composes: `triage` (one ranked
  open-findings queue across every sub-account), `drift`, `velocity` (MTTR /
  open-rate), `sla` (time-to-breach watchlist), `coverage` (detection drift vs the
  basis ruleset), `exposure` and `dc-roster` (stale / unprotected agents and
  domain controllers), `audit` (resolved-then-refired), `recurring`, `overview`,
  `reconcile`, and `workload`.
- Self-minting OAuth2 auth: `auth login` mints and caches a Blumira JWT from a
  Client ID + Secret, or set `BLUMIRA_API_TOKEN` directly.
- Agent-ready: `--agent` mode for JSON output, `--dry-run` previews for writes,
  and `agent-context` for capability discovery.
