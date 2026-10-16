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
- Initial msp-skills release: ThreatDown OneView CLI + MCP server covering the
  OneView API (sites, endpoints, detections, jobs, policies, exclusions,
  subscriptions, users and more), with OAuth2 client-credentials auth.
- `fleet stale`: every endpoint not seen in N hours across all your sites, with exact
  hours since last seen (ThreatDown's "endpoints not seen for more than 24 hours"
  PowerShell example as one command).
- `fleet health`: one ranked row per site with stale, infected, suspicious, isolated,
  unprotected, reboot-required, scan-overdue and update-available counts.
- `seats reconcile`: per-site seat allocations against deployed and recently seen
  endpoints, with expiring subscriptions flagged.
- `fleet sync` plus `threats spread`, `threats queue` and `customer report`: a local
  SQLite mirror of sites, endpoints, detections and jobs, and cross-site triage on it.
- `respond isolate` / `restore` / `scan` / `reboot` / `update`: response jobs by host
  name across every site that preview by default, send only with `--confirm`, and
  refuse more than 25 endpoints; `respond pending` finds jobs that never ran.
- MCP server with `THREATDOWN_MCP_READ_ONLY=1` for reporting-only agents.
