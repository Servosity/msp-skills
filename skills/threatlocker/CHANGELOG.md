# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `threatlocker-cli` CLI and `threatlocker-mcp` MCP
  server for the ThreatLocker Portal API, with a cross-tenant offline SQLite mirror.
- Cross-tenant approval triage (`approvals triage`) and one-command approve-across-tenants
  (`approvals approve-batch`), deduping requests by file hash.
- Audit evidence past the 31-day retention cliff: `audit export` (JSONL/CSV, per-tenant
  or all-tenants) and `audit retention-check`, plus `audit drift` for security-relevant changes.
- Fleet health: `devices health` classifies every endpoint online / offline / stale /
  isolated, and `applications hunt` locates a file by hash, certificate, or path across
  every tenant and endpoint.
- Full ThreatLocker write surface (applications, policies, computer maintenance and
  protection) with `--dry-run` previews and `--agent` JSON output for AI agents.
