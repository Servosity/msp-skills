# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: `auvik-cli` and `auvik-mcp` for the Auvik
  read-only JSON:API, plus a local SQLite mirror.
- Full endpoint surface: inventory, devices, interfaces, networks, alerts,
  configurations, entity audit and notes, billing usage, statistics, and the
  Auvik SaaS Management (ASM) applications, users, and licences.
- Eight cross-client analyses the API cannot answer in one call: `eol`,
  `changes`, `configuration audit`, `inventory diff`, `usage reconcile`,
  `device discovery-gaps`, `alert noise`, and `asm shadow`.
- `inventory diff --snapshot` keeps a prior view of the fleet, which is the only
  way a device REMOVAL is detectable - the Auvik API emits no deletion event.
