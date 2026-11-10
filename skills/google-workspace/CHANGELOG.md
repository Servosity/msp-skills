# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.7] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.6] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.5] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.4] - 2026-11-04

### Security
- **Rebuilt on Printing Press 4.35.1 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)).** The MCP server's HTTP mode now listens on `127.0.0.1:7777` by default, refuses to start until `GOOGLE_WORKSPACE_MCP_HTTP_TOKEN` is set (at least 32 characters), requires `Authorization: Bearer <token>` on every request, refuses browser origins other than loopback unless listed in `GOOGLE_WORKSPACE_MCP_HTTP_ALLOWED_ORIGINS`, and needs `--tls-cert` and `--tls-key` for any address other than loopback. The ChatGPT desktop app runs the connector locally; web-only AI apps (ChatGPT on the web, Copilot Studio, the Gemini app) are not supported by msp-skills yet; see [mcp-install.md](./mcp-install.md).
- **An unfilled Claude Desktop field is never used as the HTTP token.** If `GOOGLE_WORKSPACE_MCP_HTTP_TOKEN` arrives as the literal `${user_config...}` placeholder (with or without surrounding spaces), the server treats it as unset and HTTP mode does not start.
- **The raw API tool is split in two.** A read tool answers GET endpoints with no confirmation; the execute tool returns a preview and sends only when the call passes `confirm: true`.
- **The service-account token request never follows a redirect.** It goes only to `https://oauth2.googleapis.com/token`.
- `GOOGLE_WORKSPACE_MCP_READ_ONLY=1` now also removes the raw API execute tool and every tool that writes the local learning store, so only reads reach Google.
- A saved run profile never supplies `--yes`, and `profile save` no longer stores it.

### Added
- `GOOGLE_WORKSPACE_NO_LOCAL_DATA=1` keeps no local copy of Google data: reads go to the API, the response cache is off, and `sync` and `workflow archive` are refused. It is optional and declared on every install channel.
- `local-data purge` previews the local store files, response cache and learning journal it would erase, and deletes them only with `--yes`.

### Fixed
- **`export`, `tail` and `sync` read every page of a Google list.** A page with no records that still carried `nextPageToken` (with or without `kind`) ended the read early, so an export could finish with records missing; a record-less last page is no longer written out as a record.
- **`leavers` counts admin roles held through a group.** Role assignments are now listed with `includeIndirectRoleAssignments`, so a suspended account that is an admin through group membership is flagged.

### Changed
- `--deliver webhook:` no longer follows redirects. An Apps Script web app answers a delivered POST with a 302; that is now reported as "redirects are not followed" after the POST was sent once.

## [0.1.3] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `google-workspace-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.2] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

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
- Initial msp-skills release: Google Workspace Admin CLI + MCP server covering the
  Directory, Reports, Reseller, Enterprise License Manager, and Data Transfer APIs.
- Multi-client tenant registry: `tenants add` / `list` / `scopes` / `test` / `remove`.
  Each client is a service account with domain-wide delegation impersonating that
  client's super admin, minted with least-privilege scope profiles (`read`, `full`,
  `policy`, `reseller`). `--tenant` picks one client; `--all-tenants` fans out.
- Cross-client reports, read live in parallel with per-tenant failures listed in
  `fetch_failures`:
  - `mfa-gaps` - accounts without 2-Step Verification, unenrolled admins first.
  - `stale-licenses` - Workspace licenses on suspended, never-signed-in, or idle accounts.
  - `leavers` - suspended accounts still holding a license, groups, an admin role,
    OAuth tokens, or an unfinished Drive transfer.
  - `posture` - one row per client: 2SV coverage, admins without 2SV, stale licenses,
    Drive, Calendar and app-sharing setting changes, and (with the `policy` profile) current sharing and 2SV policy.
  - `sharing-drift` - Drive, Calendar and app-sharing setting changes and files that newly
    went external or public.
  - `oauth-apps` - third-party OAuth apps across every user and client, ranked by
    scope risk.
  - `login-alerts` - suspicious sign-ins, accounts Google disabled, 2SV turned off,
    and out-of-domain forwarding.
- `offboard` - the offboarding runbook (revoke access, remove groups, transfer Drive
  and Calendar, release licenses with `--release-license`, suspend last so backup-code
  cleanup still works),
  previewed with live reads until `--yes` is typed; preview-only over MCP.
- Per-tenant local SQLite mirror (`sync`, `search`, `analytics`), `--agent` JSON
  mode, `--dry-run` previews, and typed exit codes.
