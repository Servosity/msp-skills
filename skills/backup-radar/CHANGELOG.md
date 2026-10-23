# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `backup-radar-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

## [0.1.1] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.0] - 2026-10-20

### Added
- Initial msp-skills release of the Backup Radar connector: `backup-radar-cli` plus the
  `backup-radar-mcp` MCP server, printed by cli-printing-press 4.33.5 from the ScalePad
  Backup Radar v3 API and the read-only Backup Radar API v2, with either key optional.
- Local SQLite mirror of v3 client health (a year of history), v3 devices (90 days) and
  the v2 backups, inactive and retired lists, with `search` and `analytics` over it (and a `sql` tool over MCP).
- Morning triage: `check` ranks today's failing, silent and at-risk backups, new or
  ongoing, with days in status and open ticket count.
- Blind spots: `unwatched` lists inactive, unmapped, ticketless, no-PSA and
  warning-masked backups, one row per reason, and reports never-synced reasons as skipped.
- `scorecard` (per-client compliance rate, weekly trend, worst devices, or every client
  ranked), `timeline` (one device's day-by-day history across both API generations),
  `stale` (retire candidates with pinned `dry_run: true` bodies), `coverage diff`
  (vendor device list versus Backup Radar, both directions) and `push` (validated,
  batched CSV/JSONL ingest under the 10,000-result limit).
- `doctor` probes each configured key against the API and reports valid, rejected or
  not verified per key.
- MCP: `BACKUP_RADAR_MCP_READ_ONLY=1` (or `true` / `yes`) removes every tool that writes to Backup Radar plus `sync`, `export`, `workflow_archive` and the learning-store deletes; `backups_retire` is
  annotated destructive; file-reading and file-writing flags are refused from MCP callers.
