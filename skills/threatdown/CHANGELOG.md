# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.6] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.5] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.4] - 2026-11-07

### Security
- Rebuilt on CLI Printing Press 4.35.1 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)). The MCP server's HTTP mode refuses to start until `THREATDOWN_MCP_HTTP_TOKEN` is set (32 characters or more), listens on `127.0.0.1:7777` by default, needs `--tls-cert` and `--tls-key` on any other address, and refuses browser origins it was not told to trust.
- **MCP on/off switches take only a real boolean.** Every switch on a mirrored MCP tool accepts only a JSON `true` or `false`, so the string `"true"` can never stand in for `confirm`.
- **A credential this program holds stays out of output, files and other servers.** The CLI and the MCP server now run on the shared credential kit. Your client ID and secret, the token minted from them, `THREATDOWN_MCP_HTTP_TOKEN`, `SPACEFAST_TOKEN` and every credential in `config.toml`, the credential store and saved profiles are held from the moment either program starts. Any text that carries one, a piece of one, or one HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, is replaced by `[credkit: withheld from <where>: it carried a credential this program holds]`: errors, `--json` and `--agent` output, MCP results and the learn journal. `auth login` no longer prints a token endpoint error that repeats your client secret. A held credential is sent only to the ThreatDown API over HTTPS: never over plain http, never to a `--deliver` webhook whose address carries it, and never to a token URL on another server (that request now fails with exit code 5).
- `--deliver file:` writes a private 0600 file and never follows a planted symlink; `--deliver webhook:` does not follow redirects. `--csv` escapes cells a spreadsheet would run as a formula.

### Fixed
- **`fleet sync` mirrors the console.** Accounts, endpoints and detections that are gone from OneView, and vendor fields that were cleared, used to stay in the local store and in every report built on it. A full fleet sync now replaces what it read, and reads with the response cache off so a five-minute-old site list cannot decide what to remove.
- **A timestamp the CLI cannot read no longer hides a row.** Fleet reports used to drop a job or detection whose time field did not parse; it is now listed or counted as undated (`undated_detections` in `threats spread` and the customer report).

### Added
- `local-data purge` previews and (with `--yes`) erases the local store, response cache and learn journal. `THREATDOWN_NO_LOCAL_DATA=1` turns off the cache and store; it is declared on every install channel, with `THREATDOWN_MCP_HTTP_TOKEN` and `THREATDOWN_MCP_READ_ONLY`.

## [0.1.3] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error, an HTML page returned in place of JSON or an OAuth token endpoint error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.
- **Token requests compare redirect hosts exactly.** The check that keeps a token request on its server case-folded IPv6 interface names, so a redirect to fe80::1%LAN0 passed for fe80::1%lan0. IP addresses and their interface names now must match exactly.

### Fixed
- **`--dry-run` sends nothing, not even a token request.** With no saved token, a dry run used to request one first, sending your client ID and secret to the token URL and saving the token, although a dry run promises that no request is sent. `auth login --dry-run` now also only shows the request it would make, and `auth set-token --dry-run` and `auth logout --dry-run` no longer save or clear your stored credentials.

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

## [0.1.2] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `threatdown-cli cases create` ran `threatdown-cli cases action`, POST `/v1/casemgmt/accounts/{account_id}/cases/action`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `threatdown-cli cases action` (POST `/v1/casemgmt/accounts/{account_id}/cases/action`)
  - `threatdown-cli endpoint-jobs bulk-issue` (POST `/v1/jobs/bulk`)
  - `threatdown-cli suspicious-activity close` (POST `/v1/accounts/{account_id}/sa/close`; closes every suspicious-activity detection in detection_id_list)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.1] - 2026-10-15

### Fixed
- **Windows: the built-in Administrator account can save credentials.** On the built-in
  local Administrator (common on MSP servers and jump boxes), saving credentials failed
  with `token file DACL grants access to a non-owner principal (LA)`, and reads refused
  the saved file. The token-file permission check compared security-descriptor text
  instead of account SIDs; it now resolves each trustee to a SID first. The allowed
  principals are unchanged: your account, BUILTIN\Administrators and SYSTEM.
- **Windows: a token file with a NULL DACL is now refused.** A NULL DACL (no access
  control at all, so any account on the machine can read the file) written with its
  flags first, which is how Windows Server 2025 renders it, passed the check. The check
  now parses the DACL exactly and refuses anything it cannot parse.

## [0.1.0] - 2026-10-15

### Added
- Initial msp-skills release: ThreatDown OneView CLI + MCP server covering the
  OneView API (sites, endpoints, detections, jobs, policies, exclusions,
  subscriptions, users and more), with OAuth2 client-credentials auth.
- `fleet stale`: every endpoint not seen in N hours across all your sites, with exact
  hours since last seen (ThreatDown's "endpoints not seen for more than 24 hours"
  PowerShell example as one command).
- `fleet health`: one ranked row per site with stale, infected, suspicious, isolated,
  unprotected, reboot-required, scan-overdue and update-available counts.
- `seats reconcile`: per-site seat allocations against deployed and recently seen
  endpoints, with expiring subscriptions flagged.
- `fleet sync` plus `threats spread`, `threats queue` and `customer report`: a local
  SQLite mirror of sites, endpoints, detections and jobs, and cross-site triage on it.
- `respond isolate` / `restore` / `scan` / `reboot` / `update`: response jobs by host
  name across every site that preview by default, send only with `--confirm`, and
  refuse more than 25 endpoints; `respond pending` finds jobs that never ran.
- MCP server with `THREATDOWN_MCP_READ_ONLY=1` for reporting-only agents.
