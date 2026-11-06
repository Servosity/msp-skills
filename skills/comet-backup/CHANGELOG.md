# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-11-06

### Added
- Initial msp-skills release: Comet Backup CLI and MCP server for the Comet
  Server admin API (users, devices, jobs, policies, storage, server status),
  printed with CLI Printing Press 4.34.2.
- `health`: every protected item across every tenant that failed, is overdue
  against its schedule, or never backed up.
- `triage`: failed and stuck jobs in a window with the first error lines from
  each job log, optionally grouped by error.
- `usage`: per-tenant storage against quota, billable devices, booster engines
  and Microsoft 365 accounts.
- `devices stale` and `devices outdated`: offline devices that stopped backing
  up, and devices on a client version different from the server.
- `snapshot` and `changes`: a local history of fleet health and inventory, and
  what opened or resolved between two snapshots.
- `rerun-failed`: plans re-runs of failed scheduled backups on online devices
  and dispatches them only with `--yes`.
- Credential safety through the shared credential kit: the admin login goes only
  to the configured Comet Server over https, credential fields in responses are
  redacted and never stored, output repeating a held credential is withheld whole,
  and the MCP server runs writes (local ones included) and credential-returning
  reads only with `confirm: true`. A write carrying the `[redacted]` marker is
  refused, so a redacted profile can never overwrite the real keys.
- `doctor` makes one authenticated call: its credentials verdict is positive only
  when that call proved the login.
