# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.12] - 2026-11-12

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.2.11] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.2.10] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.2.9] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.2.8] - 2026-11-07

### Added
- **Teams Phone reads.** `phone-numbers list` and `phone-numbers get` show the tenant's Teams telephone numbers with their type, activation and assignment (needs `TeamsTelephoneNumber.Read.All`). `teams-users list` and `teams-users get` show each user's Teams configuration: numbers, Enterprise Voice and effective Teams policy assignments. Both are read-only, read the whole collection (every page, not the first 500 numbers) and work in every tenant the credential reaches.
- **Teams meeting transcripts, own tenant only.** `transcripts list`, `transcripts download` (WebVTT to stdout or a 0600 file) and `transcripts all`. They need `MICROSOFT_GRAPH_HOME_TENANT` set to your own tenant ID and refuse a token for any other tenant, a user who signed in from another tenant (GDAP or guest), and an app-only token unless `MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1` is set after a Teams application access policy limits the app to named users. Nothing is kept on disk. Through MCP they return the text, never write a file, and `MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1` switches them off.
- `pull` also syncs Teams Phone numbers and Teams user configurations, reading every page (`$skip` windows and `@odata.nextLink`) through the configured host only.

### Changed
- **Rebuilt on CLI Printing Press 4.34.2** ([GHSA-jh26-vhqh-692c](https://github.com/Servosity/msp-skills/security/advisories/GHSA-jh26-vhqh-692c)). The MCP server's HTTP mode (`--transport http`) now refuses to start until `MICROSOFT_GRAPH_MCP_HTTP_TOKEN` is set, checks that bearer token on every request, binds to loopback by default and needs TLS on any other address. `MICROSOFT_GRAPH_MCP_READ_ONLY=1` exposes only read-only tools; `pull` and `sync`, which rewrite the local store, are dropped too.
- One credential layer, the shared credential kit every msp-skills connector uses: every credential from the environment, config, flags, saved profiles or the credentials file is held before anything prints, and any output line, error, MCP message, file write, local-store record or request that would carry one is withheld whole, with a notice in its place. A credential is sent only to the configured Graph host (redirects to another host are refused), and the client id and secret only to the Entra token endpoint.
- Credentials `auth login` mints live in a separate 0600 credentials file under the data directory; the response cache is off, and `MICROSOFT_GRAPH_NO_LOCAL_DATA=1` keeps no local copy of API responses. `local-data purge` erases what the CLI keeps.
- `pull` replaces each resource's local snapshot only when every page of it was read, so a failed read keeps the previous snapshot instead of leaving a partial one. `apps consent` reads every page of every collection it joins.
- **Every list reads the whole collection.** Each list command (`users list`, `groups list`, `groups members`, `devices list`, `security alerts`, `users messages` and the rest), `export`, `tail`, `sync`, `workflow archive` and every MCP list tool (with the MCP `sync` and `workflow_archive` tools) follows Graph's `@odata.nextLink` to the last page through the configured host, or fails; the first page is never shown or stored as the whole tenant. `sync` used to store and report a short first page as the whole resource (50 of 120 alerts when Graph answered in pages of 50). Now a `sync` read that cannot be finished (a page that fails or is not a Graph collection, or a nextLink naming another host or a page already read) is a `sync_error` that stores nothing of that resource, and `--max-pages` still reports `max_pages_cap_hit`. `--top` sets the page size. Teams Phone numbers are read in `$skip` windows to the end.
- A credential a Graph response carries (`password`, `secretText`, `accessToken`, a temporary access pass, a recovery `key` and the like) is held as the response arrives, so nothing can repeat it later.
- `doctor` reports whether the credential works (valid, rejected, scope-limited or not verified) and where it came from, instead of a redacted value. When the probe fails, the error is shown under `probe_error`; an error that repeats a credential is withheld.

### Removed
- `import`, the bulk create command. No command writes to your tenant in this release.

## [0.2.7] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. This covers the token endpoint errors of `microsoft-graph-cli auth login` too. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.2.6] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.2.5] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.2.4] - 2026-10-15

### Changed
- **Rebuilt with Go 1.26.9 (was 1.26.6).** Go 1.26.9 includes upstream Go security
  fixes, among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and
  GO-2026-6612. The connector's own code is unchanged.
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but this connector's previous `.mcpb` contained only the MCP
  server, so a one-click install on a machine without the CLI failed at the first tool
  call. This release's bundle ships both binaries side by side for macOS (universal),
  Windows x64 and Linux x64.
- **The MCP server reports its real version.** `serverInfo.version` read `0.0.0-dev` in
  the previous release because the release build stamped the version into the CLI
  only. The release now stamps both and checks the built server's `initialize` reply
  against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256
  sidecars before touching anything, and replace the old binaries in a single
  transaction that is undone if any step fails. (Served from the repository, so this
  already applies to every install, not only this version.)

## [0.2.3] - 2026-09-10

### Fixed
- **A tool-call value could smuggle a refused flag past the MCP server.** The MCP server
  handed each tool argument to the CLI as two separate words, the flag and then its value.
  On a yes/no flag the CLI does not read the next word as a value, so a value that itself
  began with `--` was read as a brand-new flag, including flags the server deliberately
  refuses such as `--deliver`, which can send command output to a URL. Every argument that
  carries a value now travels glued to its flag as one word (`--flag=value`), so the CLI only
  ever reads it as the value of that one named flag, or rejects it, and it can never become a
  second flag. A plain yes/no flag is still a bare `--flag`, and an empty value is still left out.
  Reported privately through SECURITY.md; the same fix is in the pending DataGate connector.

## [0.2.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

- **Two test fixtures used the year 2099 to mean "just now".** `security triage` keeps alerts
  raised inside its look-back window and `managed-devices drift` flags devices that have not
  checked in recently, both measured against the current time. The fixtures they read were dated
  `2099-01-01`, which reads as "recent" only while the calendar is short of it: past 2099 the
  alert drops out of the 24-hour window and the device picks up a `stale` reason it was never
  meant to have. Both are now seeded an hour before the clock, and the drift test additionally
  pins that the device is *not* stale, so the seeded sync time is doing real work. No shipped
  behaviour changed - the commands were correct throughout.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.2.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `MICROSOFT_GRAPH_BASE_URL`.

## [0.2.0] - 2026-07-04

### Changed

- feat(microsoft-graph): read-only apps consent audit + governance-snapshot kit (#177)
- fleet: re-vendor 48 connectors to printing-press 4.24.0 engine (#96)
- chore(fleet): press-version provenance + back-fill hand-fix ledgers (#91)

## [0.1.0]

### Added
- Initial msp-skills release: the `microsoft-graph-cli` CLI and `microsoft-graph-mcp`
  MCP server - a lightweight, cross-platform successor to the retiring mgc (no .NET or
  PowerShell runtime).
- Local SQLite mirror via `pull` (follows `@odata.nextLink`) powering offline cross-entity
  analytics: `licenses waste`, `licenses orphans`, `licenses map`, `admins audit`,
  `security triage`, `managed-devices drift`, `groups risk`, and `tenant snapshot`.
- Read coverage of the MSP-relevant Graph surface: users, groups, directory roles,
  licenses, devices, managed devices, and security alerts/incidents.
- Agent-friendly output (`--agent`, `--json`, `--select`, `--compact`), full-text
  `search`, `export` to JSONL/JSON, named profiles, and `--dry-run` preview on the sole
  write path (`import`).
