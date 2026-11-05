# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
