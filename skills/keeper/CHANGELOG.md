# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.2] - 2026-11-10

### Fixed
- **Writing to the local store costs almost no CPU.** The credential check on rows written to the local store read every row through each decoder it knows. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).

## [0.1.1] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **Local store memory no longer climbs.** The credential check on rows written to the local store no longer grows with each page or spikes on one large record. The check is exactly as strict as before (#612).
- **One withheld row no longer drops the rest.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch, so every other row is stored (#612).

## [0.1.0] - 2026-11-05

### Added
- First release: read-only Keeper admin and MSP reports through Keeper Commander Service Mode, as a CLI and an MCP server.
- `posture report`, `posture diff`, `posture runs` and `events list`: cross-client 2FA, password health, BreachWatch, inactivity and seats, with stored history and a change feed.
- `users risk`, `users hygiene` and `users find` across every managed company, from the newest stored run.
- A fixed command allowlist (`keeper-cli allowlist`); no path to a vault record.
- The shared credential kit: the Service Mode API key is held from the first statement of the program and withheld from every output, file, log and MCP result.

### Security
- The API key goes only to `KEEPER_BASE_URL` over https. Plain http is refused on every host, localhost included, and no redirect is followed.
