# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.1] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.0] - 2026-11-06

### Added
- Initial msp-skills release: Jamf Pro CLI (`jamf-pro-cli`) and MCP server
  (`jamf-pro-mcp`) covering the Jamf Pro API and the Classic API: inventory, policies,
  scripts, packages, prestages, groups, patch management and MDM commands.
- A registry of client instances (`instance add`, `list`, `test`, `remove`): each client's
  server, API client ID and the name of the variable holding its secret.
- Reports across every client, read in parallel with each client's own credential:
  `fleet find`, `fleet posture`, `fleet patch`, `fleet app`, `fleet policy-hygiene`,
  `fleet health`, `fleet baseline` and `fleet exec`.
- A local SQLite mirror per instance for offline search (`sync`, `search`).
- `doctor` requests a token and reads the Jamf Pro version, so it tells a working API
  client from a rejected one and names the variable that fixes a missing server.

### Security
- Credentials go only to the configured Jamf Pro server; requests and redirects to any
  other host are refused, and a saved token or secret stays with the server it was saved
  for.
- Endpoints that return LAPS passwords, FileVault keys, recovery-lock passwords and lock
  PINs are not included. Password, key, token and PIN fields in other responses are
  redacted, and a downloaded document that carries a credential is withheld whole.
- Any output that repeats a held credential, in any encoding, is withheld whole by the
  shared credential kit: on the CLI, in MCP results, in files, in local store statements
  and in requests.
- MCP tools that change Jamf Pro return a preview and act only with `confirm: true`.
