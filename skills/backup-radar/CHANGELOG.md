# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.8] - unreleased

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.1.7] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.6] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.5] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.4] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.3] - 2026-11-05

### Changed
- Rebuilt on cli-printing-press 4.34.2, the press release behind advisory [GHSA-jh26-vhqh-692c](https://github.com/Servosity/msp-skills/security/advisories/GHSA-jh26-vhqh-692c). MCP HTTP mode now sets request size and time limits, plain HTTP is allowed only on a loopback address, and any other address needs `--tls-cert` and `--tls-key`. See mcp-install.md for the HTTP mode reference.
- `--dry-run` now sends nothing on every command. The API's own retire preview moved to `backups retire --server-dry-run` (MCP argument `server_dry_run`): it sends the request with `dry_run: true` and retires nothing. A `--stdin` body with `dry_run: true` still asks the API for its preview.
- GET responses are never cached on disk. `BACKUP_RADAR_NO_LOCAL_DATA=1` (now declared on every install channel) keeps no local copy: reads go to the API, nothing new is written to the store, and `sync` and `workflow archive` refuse to run.
- `check`, `unwatched`, `stale` and `scorecard` name every collection whose last sync did not finish, so a partial sync is never read as complete.

### Security
- **Every credential check now runs through the fleet's shared credential kit.** Both keys and the MCP HTTP token are held from startup; any output, file, store write, MCP result or child process that would carry one is replaced by a notice that names the sink. `BACKUP_RADAR_API_KEY` is sent only to the configured v3 host and `BACKUP_RADAR_V2_API_KEY` only to `https://api.backupradar.com`; a request that would carry either anywhere else, over plain http, or across a redirect to another host is refused before it is sent.

## [0.1.2] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `backup-radar-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

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
