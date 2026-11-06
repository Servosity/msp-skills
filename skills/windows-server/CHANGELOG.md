# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - unreleased

### Added
- First release: a Claude Code Skill, CLI (`windows-server-cli`) and MCP server
  (`windows-server-mcp`) for Windows Server 2016 to 2025 over WinRM (WS-Management over
  HTTPS, NTLM or Basic), with no agent on the server.
- `health`: one ok, warn, crit or unknown verdict per configured server across reboot,
  disks, stopped automatic services, System log errors, patch age, pending updates,
  certificates, Defender, time sync, failed scheduled tasks and Hyper-V VMs, with the
  reason and how long each check has failed.
- `services down`, `eventlog new`, `local-admins changes`, `inventory snapshot` and
  `inventory diff`, and `events list` (health-state changes with a sequence cursor).
- Reads for system, volumes, services, event logs, installed and pending updates, roles
  and features, local administrators, shares, scheduled tasks, certificates, Defender,
  time sync and Hyper-V VMs.
- Two server actions, each previewed first: `services restart` and `services start`
  (CLI `--yes`, MCP `confirm: true`).
- Several servers per config (`[[servers]]`, chosen by name with `--server`), a password
  per server in the 0600 credential store (`servers login`, read from stdin).
- Safety: only fixed, hash-pinned scripts run (no arbitrary PowerShell); certificate
  verification always on (system roots, a CA file or a pinned fingerprint); hosts come
  from config only; held passwords are withheld from every output and every file in any
  encoding.
