# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.7] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.6] - 2026-10-15

### Changed
- **Rebuilt with Go 1.26.9 (was 1.26.6).** Go 1.26.9 includes upstream Go security
  fixes, among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and
  GO-2026-6612. The connector's own code is unchanged.
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but this connector's previous `.mcpb` contained only the MCP
  server, so a one-click install on a machine without the CLI failed at the first tool
  call. This release's bundle ships both binaries side by side for macOS (universal),
  Windows x64 and Linux x64.
- **The MCP server reports its real version.** `serverInfo.version` read `0.0.0-dev` in
  the previous release because the release build stamped the version into the CLI
  only. The release now stamps both and checks the built server's `initialize` reply
  against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256
  sidecars before touching anything, and replace the old binaries in a single
  transaction that is undone if any step fails. (Served from the repository, so this
  already applies to every install, not only this version.)

## [0.1.5] - 2026-09-10

### Fixed
- **A tool-call value could smuggle a refused flag past the MCP server.** The MCP server
  handed each tool argument to the CLI as two separate words, the flag and then its value.
  On a yes/no flag the CLI does not read the next word as a value, so a value that itself
  began with `--` was read as a brand-new flag, including flags the server deliberately
  refuses such as `--deliver`, which can send command output to a URL. Every argument that
  carries a value now travels glued to its flag as one word (`--flag=value`), so the CLI only
  ever reads it as the value of that one named flag, or rejects it, and it can never become a
  second flag. A plain yes/no flag is still a bare `--flag`, and an empty value is still left out.
  Reported privately through SECURITY.md; the same fix is in the pending DataGate connector.

## [0.1.4] - 2026-09-01

_Version 0.1.3 was never published. Its GitHub release was created empty by a release-pipeline
defect, and because releases in this repository are immutable it could not be repaired; it was
withdrawn and the number retired. Everything below was written for 0.1.3 and ships here._

### Added
- **`XERO_NO_CONFIG_WRITE=1` keeps every credential off disk.** Set it and the CLI mints a token per
  invocation instead of caching one in `config.toml`. The MCP server honours the same switch,
  which it previously could not: it resolved its own config path and ignored `--config`
  entirely, so a Claude Desktop setup kept a plaintext token cache even when the CLI had been
  told not to write one. `auth logout` still clears a config that already exists - the switch
  suppresses credential writes, not the erase.

### Changed
- Install and remote-agent documentation corrected against the shipped binaries. The remote
  section named `mcp-remote`, which bridges the opposite direction and cannot publish a local
  stdio server at all; connectors that parse `--transport http` now point at the native flag
  and the rest at `supergateway`. The HTTP endpoint is `/mcp`, not the bare root the docs gave.
  The Windows install path and a fallback paragraph describing an `npx` install that is not
  offered were both wrong and are gone. A new `check_install_docs` gate holds these claims
  against the binaries and installers so they cannot drift again.

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `XERO_AUTHORIZATION_URL`, `XERO_BASE_URL`, `XERO_CLIENT_ID`, `XERO_CLIENT_SECRET`, `XERO_TENANT_ID`, `XERO_TOKEN_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `xero-cli` + `xero-mcp` for the Xero Accounting
  API across invoices, contacts, accounts, payments, bank transactions, items,
  and the immutable journals feed.
- Offline SQLite mirror with incremental `sync`, FTS5 `search`, and a `since`
  org delta - cross-object analytics computed locally instead of per-question
  API calls.
- Receivables and reconciliation analytics not available in any other Xero tool:
  `aging`, `exposure`, `reconcile`, `bank-recon`, `tie-out`, `ledger`, and a
  one-call `snapshot`.
- Agent-native plumbing: `--agent` mode, `--select` field projection, `--dry-run`
  previews, named profiles, and `--deliver` output sinks.
