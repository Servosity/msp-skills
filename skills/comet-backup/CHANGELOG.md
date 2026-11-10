# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Fixed
- **Writing to the local store costs almost no CPU.** The credential check on rows written to the local store read every row through each decoder it knows. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).

## [0.1.1] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **Local store memory no longer climbs.** The credential check on rows written to the local store no longer grows with each page or spikes on one large record. The check is exactly as strict as before (#612).
- **One withheld row no longer drops the rest.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch, so every other row is stored (#612).

## [0.1.0] - 2026-11-06

### Added
- Initial msp-skills release: Comet Backup CLI and MCP server for the Comet
  Server admin API (users, devices, jobs, policies, storage, server status),
  printed with CLI Printing Press 4.34.2.
- `health`: every protected item across every tenant that failed, is overdue
  against its schedule, or never backed up.
- `triage`: failed and stuck jobs in a window with the first error lines from
  each job log, optionally grouped by error.
- `usage`: per-tenant storage against quota, billable devices, booster engines
  and Microsoft 365 accounts.
- `devices stale` and `devices outdated`: offline devices that stopped backing
  up, and devices on a client version different from the server.
- `snapshot` and `changes`: a local history of fleet health and inventory, and
  what opened or resolved between two snapshots.
- `rerun-failed`: plans re-runs of failed scheduled backups on online devices
  and dispatches them only with `--yes`.
- Credential safety through the shared credential kit: the admin login goes only
  to the configured Comet Server over https, credential fields in responses are
  redacted and never stored, output repeating a held credential is withheld whole,
  and the MCP server runs writes (local ones included) and credential-returning
  reads only with `confirm: true`. A write carrying the `[redacted]` marker is
  refused, so a redacted profile can never overwrite the real keys.
- `doctor` makes one authenticated call: its credentials verdict is positive only
  when that call proved the login.
