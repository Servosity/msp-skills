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

## [0.1.0] - 2026-11-06

### Added
- First release of the 3CX CLI and MCP server, built on the official 3CX V20
  Configuration API (XAPI) specification: every one of its 606 operations as a
  command, against any single phone system selected with `--pbx`.
- Fleet commands for MSPs with one PBX per client, each PBX with its own API
  credential: `fleet add`, `fleet list`, `fleet remove`, `fleet health`,
  `fleet trunks`, `fleet licences`, `fleet versions`, `fleet backups`,
  `fleet changes` and the read-only `fleet exec`.
- A health verdict is `unknown`, never `pass`, when the phone system did not
  report a fact the check depends on.
- Credentials are withheld from every output and file by the shared credential
  kit; requests and redirects to any host other than the configured phone
  system are refused; `--dry-run`
  sends nothing; MCP writes preview until `confirm: true`.
