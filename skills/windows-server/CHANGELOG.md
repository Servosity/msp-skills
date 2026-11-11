# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - unreleased

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.2] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.1] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.0] - 2026-11-06

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
