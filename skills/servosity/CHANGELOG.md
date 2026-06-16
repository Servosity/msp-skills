# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.3.1] - 2026-06-16

### Fixed
- Authentication: `servosity-cli` / `servosity-mcp` now send the partner token
  with the required `Token ` scheme on the `Authorization` header. The Servosity
  API authenticates `SERVOSITY_MSP_TOKEN` via Django REST Framework's
  `TokenAuthentication`, which rejects a bare token value with HTTP 403 on every
  data endpoint; the documented bare-token setup now works as written. The MSP
  token path normalizes to the `Token ` scheme, so a value that already carries
  a scheme prefix (the `SERVOSITY_MSP_TOKEN="Token <token>"` workaround) is not
  double-prefixed. Thanks to @sonofcar102 for the detailed report (#78).

### Changed
- Regenerated the vendored `servosity-cli` / `servosity-mcp` source from
  cli-printing-press 4.24.0 and re-rendered the templated skill surfaces. No
  changes to command names or flags; the fleet mirror, snapshot history, search,
  and the reporting/revenue commands (`qbr`, `fleet-health`, `email-draft`,
  `backup-facts`, `storage-trend`, `unprovisioned`, `triage`) behave as before.

## [0.2.0]

### Changed
- Engine upgrade from the 2026-06-05 reprint: the fleet CLI grows a reporting and
  revenue surface on top of the morning-sweep commands.

### Added
- `qbr` / `qbr-all`: the backup section of a client's Quarterly Business Review as
  Markdown, HTML, or PDF - one client or the whole book in one pass.
- `email-draft --stale`: ready-to-paste follow-up email bodies for every client
  with a stale backup, filled from the local store.
- `fleet-health`: one fleet-wide scorecard (24h job success rate, stale companies,
  open issues) with week-over-week deltas.
- `bill --reconcile`: line-by-line comparison of your monthly Servosity bill
  against a CSV of what you invoice your clients - surfaces over/under-charges.
- `unprovisioned`: agents installed at clients but not yet pulling backups,
  ranked by client - the lost-revenue surface.
- `storage-trend`: linear-regression forecast of when a client hits a storage
  capacity threshold, from locally captured measurements.
- `restore-queue watch`: one terminal pinned on every client's restore queue
  during a DR event, printing diffs per tick.
- `backup-facts`: one row-per-backup view across every backup type with
  freshness-derived health.

### Removed
- `clear`, `stale-issues`, `company show`, `find`, and `restore-queue list` from
  the previous engine. `triage` now carries the batch issue mutations
  (ignore/archive/reactivate/comment) with the opt-in `--dry-run` preview;
  `search` replaces `find`; `restore-queue watch --once` replaces
  `restore-queue list`.

## [0.1.1] - 2026-06-02

### Changed
- First marketplace-ready release: one-click `.mcpb` install, validated plugin
  manifest, and registry metadata aligned for submission. No CLI/behavior change
  from 0.1.0.

## [0.1.0] - 2026-05-26

### Added
- Initial msp-skills release: Servosity CLI (`servosity-cli`) + MCP server
  (`servosity-mcp`).
- Fleet-wide backup triage, stale-backup detection, and cross-engine analytics.
- Local fleet mirror with snapshot history so the partner portal's per-page
  views become one query.
- Cross-agent install: Claude Desktop `.mcpb`, Claude Code / Codex / Cowork,
  GitHub Copilot, Gemini CLI, ChatGPT (remote), Microsoft 365 Copilot (remote),
  Hermes, and OpenClaw.
