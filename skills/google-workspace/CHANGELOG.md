# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
