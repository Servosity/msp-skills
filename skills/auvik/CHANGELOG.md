# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.0] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

### Fixed

- **MCP tools are no longer default-denied.** Every tool that is not a Cobra
  mirror - `search`, `sql`, `context`, and the `auvik_search` / `auvik_get` /
  `auvik_execute` code-orchestration trio - returned
  `MCP tenant gate is not configured` instead of running. The generated tenant
  gate treated "no platform source registered" as a failure rather than as
  "nothing to gate", and no connector registers one. The previously released
  binary had 6 dead tools. See issue #249.

- **Windows binaries now exist.** `internal/cli/auvik_snapshot_lock.go` called
  `syscall.Flock`, which does not exist on Windows, with no build tag - so the
  Windows targets had never compiled and were silently absent from the release
  assets, even though `manifest.json` declared `win32` support. Split on
  `//go:build`, with `LockFileEx` on Windows.

## [0.1.1] - unreleased

### Changed
- Describe the changes in this release.

## [0.1.0]

### Added
- Initial msp-skills release: `auvik-cli` and `auvik-mcp` for the Auvik
  JSON:API (read-dominant - dismissing an alert is the only write it exposes), plus a local SQLite mirror.
- Full endpoint surface: inventory, devices, interfaces, networks, alerts,
  configurations, entity audit and notes, billing usage, statistics, and the
  Auvik SaaS Management (ASM) applications, users, and licences.
- Eight cross-client analyses the API cannot answer in one call: `eol`,
  `changes`, `configuration audit`, `inventory diff`, `usage reconcile`,
  `device discovery-gaps`, `alert noise`, and `asm shadow`.
- `inventory diff --snapshot` keeps a prior view of the fleet, which is the only
  way a device REMOVAL is detectable - the Auvik API emits no deletion event.
