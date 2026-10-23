# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.4] - unreleased

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

## [Unreleased]

### Changed
- `cli/NOTICE` now names the product `bitdefender-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

## [0.1.3] - 2026-10-22

### Security
- The MCP server no longer passes `import --input` to the CLI, so a tool call cannot point the import at a file on the machine that runs the server.

## [0.1.2] - 2026-10-22

### Security
- The MCP server no longer passes `fleet rmm-gap --devices` or `usage billing --agreements` to the CLI, so an MCP tool call cannot point either command at a file on the machine that runs the server. Both flags still work on the command line.

## [0.1.1] - 2026-10-21

### Security
- **MCP tool calls can no longer read local files through `teach`.** Over MCP, `teach --playbook-file` / `--playbook-notes-file` and `teach-playbook --playbook-file` / `--notes-file` copied any file the MCP server's account could read into the local learning store, where the `playbook_list` and `recall` tools handed it back. The MCP server now leaves these four parameters out of the tool schemas and refuses a call that names them. Pass the content inline instead (`playbook-json`, `playbook-notes`, `notes`). The CLI is unchanged: run from your own shell, these flags still read your own files.
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

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
