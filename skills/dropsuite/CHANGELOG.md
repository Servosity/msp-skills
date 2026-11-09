# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

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
