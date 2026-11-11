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
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.5] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.4] - 2026-11-07

### Security
- **Rebuilt on Printing Press 4.35.1.** The MCP server's HTTP mode (`connectwise-rmm-mcp --transport http`) now refuses to start until `CONNECTWISE_RMM_MCP_HTTP_TOKEN` is set to at least 32 characters, binds `127.0.0.1:7777` by default, needs `--tls-cert` and `--tls-key` for any other address, limits request size and time, and refuses a browser `Origin` that is not loopback or listed in `CONNECTWISE_RMM_MCP_HTTP_ALLOWED_ORIGINS`. The default stdio transport is unchanged. Remote agents (ChatGPT, Copilot Studio, the Gemini app) can use the hosted HTTPS path again; see [mcp-install.md](./mcp-install.md).
- **MCP tools check argument types.** A mirrored MCP tool now refuses an argument whose JSON type does not match the CLI flag behind it (for example the string `"true"` for a true/false flag) instead of passing it on as text.
- **`scopes` token requests use the same guard as every other token request.** They refuse plain HTTP and stay on the configured token host, like `auth login` and the automatic token request.
- **One shared credential layer.** The CLI and MCP server now use the msp-skills credential kit: the client secret, the client ID, every token minted from them, the MCP HTTP token and the Spacefast token are known before anything prints, and any output unit that repeats one in any encoding (stdout, stderr, files, the local store, MCP results) is replaced whole by a notice. Each credential is sent only to its own API origin: the configured base URL, the ConnectWise RMM regional token host (picked from the client ID, or set with `CONNECTWISE_RMM_TOKEN_URL`), or `https://api.spacefast.com` for the Spacefast token; the MCP HTTP token is sent nowhere.
- **The learn journal no longer records text typed in place of a flag name,** and a `CONNECTWISE_RMM_LEARN_SESSION` value is stored hashed.

### Fixed
- **`fleet reboot-due` no longer puts an endpoint with a logged-on session in `reboot_now` because the session lacks a username or a status.** Both are optional in the ConnectWise RMM system-state response. Only a session reported `Disconnected` is ignored now; any other session, named or not, with any or no status, puts the endpoint in `coordinate`.
- The MCP `execute` tool's preview of a DELETE request now shows the request body it would send.

### Added
- `local-data purge` lists the local store, response cache and learn journal and deletes them only with `--yes`; `CONNECTWISE_RMM_NO_LOCAL_DATA=1` runs with no local copy, and `CONNECTWISE_RMM_DATA_SOURCE` sets the `--data-source` default.
- `--ca-cert` / `CONNECTWISE_RMM_CA_CERT` adds a PEM CA bundle for a TLS-inspecting proxy with a private CA, and `--tls-server-name` / `CONNECTWISE_RMM_TLS_SERVER_NAME` sets the name checked against the certificate.
- `--csv` prefixes `'` on a cell that a spreadsheet would run as a formula; `--deliver file:` writes a private 0600 file; `--deliver webhook:` no longer follows redirects.

## [0.1.3] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error, an HTML page returned in place of JSON or an OAuth token endpoint error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.
- **Token requests compare redirect hosts exactly.** The check that keeps a token request on its server case-folded IPv6 interface names, so a redirect to fe80::1%LAN0 passed for fe80::1%lan0. IP addresses and their interface names now must match exactly.

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

## [0.1.2] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given this command the hidden alias `create`, so typing `create` in its place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `connectwise-rmm-cli tasks create` ran `connectwise-rmm-cli tasks schedule`, POST `/api/platform/v1/automation/endpoints/schedule-tasks`. The alias is removed; the command still runs under its own name. If a script reached it through `create`, call it by the name below:
  - `connectwise-rmm-cli tasks schedule` (POST `/api/platform/v1/automation/endpoints/schedule-tasks`)

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
- Initial msp-skills release: a ConnectWise RMM (Asio) CLI and MCP server covering 66
  partner API operations - companies, sites, endpoints, patching, custom fields,
  policies, hypervisors, scripts, tasks, and Asio service tickets.
- Local SQLite mirror (`sync`, `search`) of companies, sites, endpoints and tickets, so
  cross-client questions run against a local copy.
- Fleet roll-ups: `fleet stale` (offline and silent agents across every client),
  `fleet patch-posture` (clients ranked by patch risk), `fleet reboot-due` (the
  patch-week reboot queue), `fleet whois` (which machine a user is logged on to),
  `fleet brief` (one endpoint's card), `fleet scorecard` (one client's per-site health
  card), and `fleet task-failures` (failed script runs grouped by identical error).
- `run` resolves a script and target endpoints by name, previews the request, and only
  schedules with `--confirm`; `--wait` collects per-endpoint results.
- `scopes` finds which OAuth scopes your API key was granted and prints the scope
  setting to use, since one ungranted scope fails the whole token request.
- Region follows the client ID prefix (North America, Europe, Australia); tokens are
  minted with the JSON body ConnectWise requires.
- `doctor` mints a token and verifies it against an authenticated endpoint instead of
  reporting the credential as present but unchecked.
