# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `atera-cli` binary and `atera-mcp` server
  covering the full Atera RMM + PSA API surface (agents, tickets, customers,
  contracts, alerts, devices, rates, contacts, departments, custom fields).
- Local SQLite mirror with `sync` + FTS5 `search` for instant, offline,
  rate-limit-friendly queries.
- Cross-client analytics the live API can't express in one call: `agents stale`,
  `agents inventory`, `agents noisy`, `agents patch-status`, `alerts triage`,
  `tickets sla`, `tickets workload`, `customers book`, `customers coverage`,
  `contracts expiring`, and `since`.
- Agent-native output (`--agent`, `--json`, `--compact`) and typed exit codes
  for scripting and AI-agent use.
