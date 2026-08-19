# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `huntress-cli` and the `huntress-mcp` MCP server,
  covering the full Huntress API - accounts, organizations, agents, incident
  reports, remediations, signals, escalations, identities, external recon,
  reports, invoices, reseller subscriptions, and SIEM ES|QL.
- Offline SQLite mirror with `sync` and FTS5 `search` for instant, repeatable
  queries that cost zero API calls.
- Cross-tenant rollups the per-org API and portal can't return: `fleet-incidents`,
  `fleet-summary`, `coverage-gaps`, `blast-radius`, `billing-reconcile`,
  `triage-age`, `org-scorecard`, and `reseller-rollup`.
- History the point-in-time API throws away: `drift`, `mttr`, `handoff`, and
  `canary-watch`.
- Agent-native output everywhere: `--json`, `--select`, `--agent`, `--dry-run`,
  and typed exit codes.
