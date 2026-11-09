# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.1] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.0] - 2026-11-04

### Added
- First msp-skills release of the NetSapiens CLI (`netsapiens-cli`) and MCP server
  (`netsapiens-mcp`), built on CLI Printing Press 4.34.2, covering every NetSapiens v2
  endpoint on any white-label platform (`NETSAPIENS_SERVER`).
- Reseller-wide audits over a local SQLite mirror: `fleet seats`, `fleet offline`,
  `fleet dids`, `fleet e911` and `fleet changes`, plus live `fleet missed` and
  `fleet queues` from CDRs and queue statistics.
- `trace` resolves one number, extension, MAC or email to its client, routing, user
  and phones.
- Every MCP write previews first and sends only with `confirm: true`;
  `NETSAPIENS_MCP_READ_ONLY=1` refuses every write. HTTP mode requires
  `NETSAPIENS_MCP_HTTP_TOKEN`.
- Credentials are held from startup by the shared credential kit and filtered out of
  every output: terminal, errors, files, MCP results, the local mirror and the learning
  journal. A credential only goes to the configured server over HTTPS.
