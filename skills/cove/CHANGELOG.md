# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.6] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

## [0.1.5] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `COVE_BASE_URL`, `COVE_PARTNER`, `COVE_PASSWORD`, `COVE_USERNAME`.

## [0.1.4] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.3]

### Fixed
- Clarified that `COVE_PARTNER` must be the **full customer/partner string exactly as shown in the Cove Management Console customer dropdown** - usually including the account email in parentheses, e.g. `Acme Corp (admin@acme.com)`, not just the company name. Cove returns the same `2100 "Unknown partner/username or bad password"` error for a wrong `COVE_PARTNER` format as for a bad password, so the error message, `auth` help, and docs now call this out. Also decoded the `13501 "Operation failed because of security reasons"` visa error (the API User's role may not permit a method; use `cove-cli call <Method>`, which manages the session visa automatically) and documented that the raw generated endpoint commands (partners/devices/storage `list`·`get`, `promoted *`) need an explicit `--visa "$(cove-cli auth token)"` while `cove-cli call` injects it for you. Documentation and error-string only; no new flags or commands. Thanks to @AvlCompCo for the detailed report (#133).

## [0.1.2]

### Fixed
- Corrected the authentication guidance for N-able's current API model. Cove access now requires a dedicated **API User** (Cove Management Console > Users > API Users), not the removed per-user "API access" checkbox. Documented that `COVE_PARTNER` (the customer the API user was created for) is **required** for API Users, that `COVE_USERNAME`/`COVE_PASSWORD` are the API user's login name and API token, and that the token is the login *password* (not a bearer header, and not itself a visa, so passing it to `--visa` fails by design). Sharpened the `2100`/credential error hints to point at an empty `COVE_PARTNER` as the usual cause. No new flags or commands; the existing login path already supports API Users once `COVE_PARTNER` is set. Thanks to @AvlCompCo for the detailed report (#115).

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `cove-cli` CLI and `cove-mcp` MCP server for
  N-able Cove Data Protection (backup.management JSON-RPC API).
- Fleet triage across the whole partner tree: `devices failures`, `devices stale`,
  and `fleet health` with F00 status codes decoded to plain names.
- Local snapshot history (`snapshot`) powering trend commands `storage growth`
  and `devices changes` the vendor console does not keep.
- Month-end billing: `billing usage` and `billing changes` with SKU and M365 seat
  column codes decoded.
- Offline SQLite mirror (`sync`), generic `call` escape hatch to every documented
  JSON-RPC method with automatic visa injection, and `--agent` JSON mode.
