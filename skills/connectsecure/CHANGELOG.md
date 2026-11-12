# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.9] - unreleased

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.1.8] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.7] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.6] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.5] - 2026-11-07

### Changed
- Rebuilt on CLI Printing Press 4.35.1 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)).
- HTTP mode (`connectsecure-mcp --transport http`) is documented again, for reference. It refuses to start until `CONNECTSECURE_MCP_HTTP_TOKEN` is set to at least 32 characters, every request must send that token as a bearer header, any address other than loopback also needs `--tls-cert` and `--tls-key`, and browser origins other than loopback are refused unless listed in `CONNECTSECURE_MCP_HTTP_ALLOWED_ORIGINS`.
- New `local-data purge` previews and, with `--yes`, erases the local store, response cache and learn journal. `CONNECTSECURE_NO_LOCAL_DATA=1` runs every read live and writes nothing locally.
- `--csv` prefixes `'` on cells that a spreadsheet would run as a formula. `--deliver file:` writes a private 0600 file; `--deliver webhook:` does not follow redirects.
- `sync` no longer mirrors three report-builder resources (standard report settings, the default template download and the standard report list). None was in the default sync set or read by an analysis command.

### Fixed
- `report-queries registry-problems-remediation-asset-details` reads the rows its own sync writes. Rows saved by an earlier version are not migrated; the next sync of that report fills them in.
- Active Directory computer rows are keyed by `object_guid`, so two clients' computers with the same name stay separate rows in the local store.

### Security
- Credentials are handled by the shared msp-skills credential kit: every output, file, local-store write, MCP result and outbound request that would carry a credential this program holds (the client secret, the API key pair, a pre-minted or freshly minted access token, a saved token, a configured header) is withheld whole (a `[credkit: withheld from ...]` notice), and a credentialed request goes only over https to the configured pod. With no pod configured, no credential is sent anywhere.
- MCP file uploads take the file as base64 content. A path is accepted only when `CONNECTSECURE_MCP_UPLOAD_ROOT` names a folder, and only for files inside it.

## [0.1.4] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `connectsecure-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

## [0.1.3] - 2026-10-22

### Security
- The MCP server no longer opens a local file named by a binary multipart (upload) field and sends it to the vendor; run such an upload with the CLI.

## [0.1.2] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `connectsecure-cli agents create` ran `connectsecure-cli agents reset`, POST `/w/company/reset_agents`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `connectsecure-cli agents reset` (POST `/w/company/reset_agents`)
  - `connectsecure-cli patching patch-now` (POST `/w/company/patch_now`)

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
- Initial msp-skills release: `connectsecure-cli` and `connectsecure-mcp` for every
  ConnectSecure (formerly CyberCNS) API operation, authenticating per tenant pod by
  minting a short-lived token from the tenant name, client ID and client secret.
- Seven cross-client commands: `exposure` (open findings ranked across every client
  at asset level by EPSS, CVSS and known-exploited status), `portfolio` (one risk row
  per client), `blind-spots` (assets whose scan data cannot be trusted),
  `suppression-audit` (expired, open-ended and self-approved suppressions),
  `ad-hygiene` (risky Active Directory accounts and weak password policies),
  `cve-impact` (which clients a named CVE hits, with fixes) and `qbr` (one client's
  QBR risk pack).
- Local SQLite mirror: a bare `sync` reads a curated set of 15 reports; credential
  records, integration credentials, integration rules, scheduled jobs (whose rows carry
  a password field), attack-surface results (creds and hashes fields), custom domains
  (undocumented certificate details) and the agent uninstall secret are never written
  to the mirror or the HTTP response cache, and `workflow archive` skips them instead
  of aborting.
- Secret handling on the wire: `--dry-run` previews mask secret fields, an API error
  (or token-mint error) that echoes a secret you sent is withheld instead of printed,
  a request the server refuses with HTTP 200 and `"status": false` is reported as a
  failure (a refused read is never printed or cached as an empty result), and a write is never sent to another origin on a redirect (or reported
  as a success when a redirect turned it into a read).
- `qbr` and `portfolio` leave a client's risk score blank when another client shares
  its name, and `cve-impact` flags a client name several companies share: those
  ConnectSecure reports are keyed by company name only.
- A `sync` narrowed with `--param`, `--global-param` or `--resource-param` neither
  prunes other rows (even with `--full`) nor marks the resource fully synced.
- The MCP `tail` tool is not marked read-only (it can stream credential records), so
  hosts ask before running it and `CONNECTSECURE_MCP_READ_ONLY=1` hides it. `doctor`
  never writes to the response cache.
- MCP recipe tool `which_clients_does_this_cve_hit` refuses a flag-like value before
  the CLI runs, so a tool-call argument cannot smuggle a root flag such as
  `--deliver`.
