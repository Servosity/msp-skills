# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

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
