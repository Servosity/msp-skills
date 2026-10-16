# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
