# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

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
