# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-10-19

### Added
- Initial msp-skills release: `bitdefender-cli` and `bitdefender-mcp` for the
  Bitdefender GravityZone Public API (JSON-RPC), cloud or on-premises Control
  Center, authenticated with a Control Center API key.
- 140 GravityZone Partners and Cloud Public API methods as commands:
  endpoints, companies, policies, incidents, quarantine, isolation, scan and
  investigation tasks, patches, PHASR, reports, packages, license and usage.
- Cross-customer views that walk the whole partner tree: `fleet health`,
  `fleet incident-queue`, `fleet threats`, `fleet locate`, `fleet rmm-gap`,
  `fleet changes` (diffs saved snapshots), `companies tree` and `usage billing`
  (own share after the partner rollup, with PSA agreement checks).
- `BITDEFENDER_MCP_READ_ONLY=1` lets the MCP server's generic execute tool run
  GravityZone read methods only; writes and the Credential-tier reads are
  refused (incident reads stay allowed).
- An environment `BITDEFENDER_BASE_URL` is a per-run override and is never saved
  to config.toml.
