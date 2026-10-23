# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - unreleased

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `google-workspace-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

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
