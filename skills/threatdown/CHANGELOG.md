# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
