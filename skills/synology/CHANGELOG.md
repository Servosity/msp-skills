# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: Synology CLI (`synology-cli`) and MCP server
  (`synology-mcp`) for Synology DSM and Active Backup for Business.
- Multi-NAS fleet: `fleet add` registers each client NAS with its own login
  (read from environment variables, never stored), two-factor sign-in via a
  cached trusted-device token, and certificate pinning for DSM's self-signed
  certificate (`--pin-cert`).
- `fleet sync` mirrors Active Backup tasks, job results, per-device runs and a
  Storage Manager snapshot from every registered NAS into one local SQLite
  database; one unreachable NAS does not stop the others.
- Fleet reports from the mirror: `fleet status` (one row per NAS), `abb failures`
  (failed, partial and missed runs), `abb stale` (devices with no recent good
  backup), `abb device` (find a machine by host name), `abb trend` (success rate,
  run duration and overruns) and `fleet health` (degraded pools, bad SMART disks,
  hot disks, nearly full volumes with days to full). A NAS that did not sync reads
  as not synced, never as OK, and the reports exit 6 when anything needs attention.
- `abb triage`: why one task failed, from its newest failed run's detail lines and
  the Active Backup log for that window.
- Live per-NAS reads for Active Backup, Hyper Backup, storage, SMART, system
  health, logs, packages, shared folders, users and groups.
- `abb backup` and `abb cancel` to start or cancel a backup, previewable with
  `--dry-run`; `SYNOLOGY_MCP_READ_ONLY=1` removes every write tool from the MCP
  server.
