# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.1] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.0] - 2026-11-05

### Added
- Initial msp-skills release: `prtg-cli` and the `prtg-mcp` MCP server for
  PRTG Network Monitor API v2, built on CLI Printing Press 4.34.2.
- Fleet view across every client's PRTG core: register cores with `core add`,
  then `fleet alarms`, `fleet status`, `fleet handoff`, `fleet silenced`,
  `fleet drift` and `fleet exec` ask every core in one run; `fleet sync` keeps an
  offline mirror per core.
- Previewed bulk actions across cores: `fleet ack`, `fleet pause` and
  `fleet resume` show what they would do on each core and send only with `--yes`
  (over MCP, only with `confirm: true`).
- Every PRTG API v2 operation as a typed command, pointable at one core with
  `--core`.

### Security
- Credentials are held from every source before any output, and every output
  path (terminal, MCP results, errors, local files) withholds a held value in any
  encoding; the license key returned by `system license-info` is never stored.
- Requests go only to the configured PRTG core: a redirect or a request to any
  other host is refused.
- Every MCP tool that changes PRTG previews until called with `confirm: true`.
- A bulk action reports failure when any item in PRTG's result failed.
