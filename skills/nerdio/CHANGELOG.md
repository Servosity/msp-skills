# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `nerdio-cli` CLI and `nerdio-mcp` MCP server for
  the Nerdio Manager for MSP (NMM) Partner REST API.
- Cross-account fleet commands: `fleet autoscale-audit` (pools with autoscale off
  or drifting), `fleet host-estate` (every session host and its power state), and
  `fleet billing-rollup` (per-customer billed/unpaid/usage rollup).
- `usages drift` for month-over-month consumption comparison across accounts.
- `job wait` - poll any async NMM job to a terminal state with a typed exit code,
  plus `scripted-actions fan-run` to execute one action across many accounts and
  wait for all of them.
- Coverage for host pools, session hosts, desktop images, Intune devices, backup
  and recovery vaults, reservations, networks, scripted actions, and secure
  variables.
- Offline SQLite mirror (`sync`) with full-text `search`, OAuth2 client-credentials
  auth against each MSP's own NMM instance, and `--agent` JSON output mode.
