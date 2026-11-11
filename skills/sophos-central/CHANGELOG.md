# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.7] - unreleased

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
- **One shared credential guard covers every output.** The CLI and the MCP server now hold every credential they read or mint (environment, flags, the config and credential files, saved profiles, minted and refreshed tokens) and withhold any terminal line, file, store row, cache entry, MCP result, child process or outbound request that would carry one, with `[credkit: withheld from <sink>: it carried a credential this program holds]` in its place. A token endpoint error that repeats the client secret no longer shows it on `auth login`.
- **Credentials go only to Sophos.** The API token goes only to the configured base URL, `*.central.sophos.com` and `id.sophos.com`; the client secret and refresh token only to the base URL and `id.sophos.com`. A token URL on any other host is refused.
- **The CLI no longer prints a tamper protection password or a migration token.** `endpoints get-tamper-protection`, `endpoints set-tamper-protection` and the `migrations` commands print the withheld notice where the secret would be; the rest of the answer still prints. Read those secrets in the Sophos Central console.

### Changed
- **Rebuilt on CLI Printing Press 4.35.1 (was 4.33.5).** See [GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c) for why the press was upgraded.
- **The MCP server's HTTP mode for remote agents is back.** `sophos-central-mcp --transport http` refuses to start until `SOPHOS_CENTRAL_MCP_HTTP_TOKEN` (32 characters or more) is set, rejects any request without that bearer token, refuses a request body over 4 MiB or one that takes longer than 30 seconds to arrive, serves `http://127.0.0.1:7777/mcp` by default and needs `--tls-cert` and `--tls-key` for any other address. The remote-agent steps are back in `mcp-install.md`; stdio stays the default.
- **Every MCP call that changes Sophos Central data previews first.** `sophos-central_execute` returns what it would send and sends nothing until the call carries `confirm: true`. `contain` and `fleet tamper` stay preview-only over MCP.
- New install setting on every channel: `SOPHOS_CENTRAL_NO_LOCAL_DATA=1` keeps no local copy of API responses (reads go to the API, the response cache is off, and `sync`, `fleet refresh` and `workflow archive` refuse to run).
- `sync` with no resource list no longer includes `licenses` or `web_control-categories` (`fleet refresh` still reads licences). The `health_check` sync partition is now `/scores/regional` and `web_control` is `/web-control/categories`.
- Sync checkpoints are now kept per tenant and scope, so the first `sync` after upgrading walks each resource once from the start.

### Fixed
- `endpoints bulk-isolate` no longer answers to the hidden alias `create` (the 4.35.1 generator had added it), so `sophos-central-cli endpoints create` no longer isolates endpoints.
- `fleet revenue --tenant` reports only the selected customers.
- A section the Account Health Check returns as null (Sophos documents it as an unexpected error) is a failed check: the tenant reads `partial`, `fleet gaps` reports `health_check_failed`, and `fleet health` gives a change only between two health checks that scored the same checks (otherwise `delta` is empty and `delta_note` says why).
- Fleet rows always carry the tenant they were fetched for, and a row naming another tenant is refused. Rows an older build stored without a tenant are removed on the next `fleet refresh`; each tenant then reads as not refreshed until that refresh reaches it.
- `fleet gaps` reads unlicensed protection from `deviceSoftware` too, counts a licence with quantity 0 as an entitlement, and treats a licence as valid through the end of its end date (UTC).

## [0.1.3] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error, an HTML page returned in place of JSON or an OAuth token endpoint error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.
- **Token requests compare redirect hosts exactly.** The check that keeps a token request on its server case-folded IPv6 interface names, so a redirect to fe80::1%LAN0 passed for fe80::1%lan0. IP addresses and their interface names now must match exactly.

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

## [0.1.2] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `sophos-central-cli alerts create` ran `sophos-central-cli alerts act-on`, POST `/common/v1/alerts/{alertId}/actions`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `sophos-central-cli alerts act-on` (POST `/common/v1/alerts/{alertId}/actions`)
  - `sophos-central-cli blocked-addresses delete-blocked-address-sets` (POST `/endpoint/v1/settings/blocked-addresses/bulk/delete`)
  - `sophos-central-cli endpoints bulk-delete` (POST `/endpoint/v1/endpoints/delete`)
  - `sophos-central-cli health-check post-snooze` (POST `/account-health-check/v1/snooze`; snoozes account health-check findings for the policies and exclusions in the body)
  - `sophos-central-cli migrations create-receiving-job` (POST `/endpoint/v1/migrations`; starts a job that migrates endpoints from another tenant)
  - `sophos-central-cli software add-special-static-package` (POST `/endpoint/v1/software/packages/static/{staticPackageId}/add`; adds a support-supplied software package to the tenant; Sophos calls it one-way)
  - `sophos-central-cli xdr cancel-query-run` (POST `/xdr-query/v1/queries/runs/{runId}/cancel`)

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
- Initial msp-skills release: Sophos Central CLI + MCP server covering the partner,
  organization, common, endpoint, Live Discover, XDR query, Account Health Check
  and SIEM REST APIs, with every call routed to the right tenant and regional host
  from `whoami` (X-Partner-ID / X-Organization-ID / X-Tenant-ID handled for you).
- `fleet refresh`, `fleet triage`, `fleet health`, `fleet gaps` and `fleet revenue`:
  cross-customer views over a local store with a per-tenant ok/forbidden/error ledger.
- `fleet tamper`: find machines with tamper protection off across customers and,
  with `--yes`, turn it back on.
- `contain`: isolate machines by hostname, IP or endpoint ID without knowing the
  tenant, with optional scan, forensic log and alert acknowledgement.
- `fleet siem-collect`: per-tenant SIEM events and alerts as NDJSON from saved
  cursors, with a gap warning when a cursor fell outside Sophos's 24-hour window.

### Security
- Responses carrying a tamper protection password or a migration token are never
  written to the response cache or the local store, and those commands are kept out
  of the MCP server.
- A client secret supplied through `SOPHOS_CENTRAL_CLIENT_SECRET` is never written
  to disk, and `--dry-run` never mints a token.
