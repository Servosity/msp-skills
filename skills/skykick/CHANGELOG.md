# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `skykick-cli` plus the `skykick-mcp` MCP server
  (20 tools), targeting the current `apis.cloudservices.connectwise.com` host.
- `fleet-sync` - one command pulls every subscription plus per-tenant settings,
  retention, autodiscover, snapshot stats, mailboxes, sites, and alerts into a
  run-versioned local SQLite store (last 5 runs kept so `drift` can diff).
- Cross-tenant posture views the per-subscription API can't produce:
  `fleet-health`, `stale-snapshots`, `coverage-gaps`, `retention-audit`,
  `autodiscover-audit`, `partner-rollup`.
- `drift` - diffs the two most recent fleet-syncs for protection-state changes.
- `alert-sweep` - fleet-wide ranked alert triage with optional bulk mark-complete
  (`--complete <ids> --apply`); `watch-operation` polls async discovery to a
  terminal state.
- Offline SQLite mirror with full-text search, `--agent` JSON mode, and
  `--data-source auto|live|local` on every read.

