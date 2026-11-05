# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-11-05

### Added
- Initial msp-skills release: Dropsuite (NinjaOne SaaS Backup) CLI and MCP server
  (stdio), built on press 4.34.2.
- Every Dropsuite partner API read, with `--org` (or `DROPSUITE_ORG`) resolving each
  client organization's token from your reseller credential.
- Fleet commands across every client: `health` (failed, stuck and stale backups, with
  Nagios exit codes), `coverage` (tenant mailboxes not added to backup), `billing`
  (seat counters against a PSA invoice CSV), `hygiene` (deactivated mailboxes still
  holding storage, unaccepted invitations) and `locate` (which client owns an address).
- Local evidence: health snapshots for `drift`, a restore-test ledger
  (`restore-test record`, `list`, `due`) and `evidence`, a dated proof-of-protection pack.
- The shared credential kit: both reseller tokens and every organization token are
  held from startup and withheld from all output, files and the local store; requests
  go only to the configured API host, redirects included; no response cache.
- Over MCP, the portal sign-in link tools preview until `confirm: true`, and
  `DROPSUITE_MCP_READ_ONLY=1` removes them.
