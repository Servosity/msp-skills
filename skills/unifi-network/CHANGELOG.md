# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [Unreleased]

### Changed
- `cli/NOTICE` now names the product `unifi-network-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

## [0.1.8] - 2026-10-22

### Security
- The MCP server drops the flags whose values name a file or directory on the machine that runs it (for example --db, --output) from its tool schemas and refuses them by name, instead of advertising them and ignoring them silently. The command line keeps every flag.

## [0.1.7] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given this command the hidden alias `create`, so typing `create` in its place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `unifi-network-cli sites clients create` ran `unifi-network-cli sites clients execute-connected-action`, POST `/v1/sites/{siteId}/clients/{clientId}/actions`. The alias is removed; the command still runs under its own name. If a script reached it through `create`, call it by the name below:
  - `unifi-network-cli sites clients execute-connected-action` (POST `/v1/sites/{siteId}/clients/{clientId}/actions`)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.6] - 2026-10-15

### Fixed
- **Windows: the built-in Administrator account can save credentials.** On the built-in
  local Administrator (common on MSP servers and jump boxes), saving credentials failed
  with `token file DACL grants access to a non-owner principal (LA)`, and reads refused
  the saved file. The token-file permission check compared security-descriptor text
  instead of account SIDs; it now resolves each trustee to a SID first. The allowed
  principals are unchanged: your account, BUILTIN\Administrators and SYSTEM.
- **Windows: a token file with a NULL DACL is now refused.** A NULL DACL (no access
  control at all, so any account on the machine can read the file) written with its
  flags first, which is how Windows Server 2025 renders it, passed the check. The check
  now parses the DACL exactly and refuses anything it cannot parse.

### Changed
- **Rebuilt with Go 1.26.9 (was 1.26.6).** Go 1.26.9 includes upstream Go security
  fixes, among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and
  GO-2026-6612.
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but this connector's previous `.mcpb` contained only the MCP
  server, so a one-click install on a machine without the CLI failed at the first tool
  call. This release's bundle ships both binaries side by side for macOS (universal),
  Windows x64 and Linux x64.
- **The MCP server reports its real version.** `serverInfo.version` read `2026.8.1`, a
  hard-coded default, in the previous release because the release build stamped the
  version into the CLI only. The release now stamps both and checks the built server's
  `initialize` reply against the tag before publishing.
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

### Fixed
- **`recall` and `playbook list` no longer claim to be read-only.** Both open the writable
  learn store and record a row, but were annotated `mcp:read-only=true` - and that annotation
  is what an MCP host reads to decide what to auto-approve without asking you. They are now
  `mcp:local-write`, a tier this engine already defines and already uses for `teach`: writes
  land only in the CLI's own local store, never in external state and never in a user-visible
  file. Measured at the live MCP server, both tools moved from `readOnlyHint=true,
  openWorldHint=true` to `readOnlyHint=false, destructiveHint=false, openWorldHint=false`.
  No behaviour changed - both commands write exactly what they wrote before. The promise was
  the defect, not the write.

### Changed
- Install and remote-agent documentation corrected against the shipped binaries. The remote
  section named `mcp-remote`, which bridges the opposite direction and cannot publish a local
  stdio server at all; connectors that parse `--transport http` now point at the native flag
  and the rest at `supergateway`. The HTTP endpoint is `/mcp`, not the bare root the docs gave.
  The Windows install path and a fallback paragraph describing an `npx` install that is not
  offered were both wrong and are gone. A new `check_install_docs` gate holds these claims
  against the binaries and installers so they cannot drift again.

## [0.1.3] - 2026-08-26

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

## [0.1.2] - 2026-08-26

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
  authenticate. Now declared on every install channel: `PRINTING_PRESS_CLIENT_PROFILE`, `UNIFI_BASE_PATH`, `UNIFI_BASE_URL`.

## [0.1.1] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

### Fixed

- **MCP tools are no longer default-denied.** Every tool that is not a Cobra
  mirror - `search`, `sql`, `context`, and the `unifi_search` / `unifi_get` /
  `unifi_execute` code-orchestration trio - returned
  `MCP tenant gate is not configured` instead of running. The generated tenant
  gate treated "no platform source registered" as a failure rather than as
  "nothing to gate", and no connector registers one. The previously released
  binary had 6 dead tools. See issue #249.

## [0.1.0]

### Added
- Initial msp-skills release: `unifi-network-cli` and `unifi-network-mcp` for the
  local UniFi Network integration API on a self-hosted UniFi OS gateway.
- Full site surface: adopted devices, connected clients, firewall zones and policies
  (including evaluation ordering), ACL rules, networks and VLANs, DNS policies, WiFi
  broadcasts, hotspot vouchers, switching (LAG, MC-LAG, switch stacks), VPN, WANs,
  RADIUS profiles, device tags, and traffic-matching lists.
- Local SQLite mirror with FTS5 full-text `search`, plus `sync`, `export`, `analytics`.
- `drift` - diffs site config (networks, firewall, WiFi, DNS) against a snapshot it
  captures itself, because the API exposes no config-versioning or audit-trail endpoint.
- `newcomer` - maintains a first-seen record per device and client so new hardware
  surfaces against a real baseline.
- `port-audit` - per-port link state and PoE status across every switching or gateway
  device on a site, fetching the interface detail the list endpoints omit.
- `rule-predict` - simulates firewall policy matching in the gateway's own
  ascending-index, first-match-wins order, flagging unresolvable policies as uncertain.
- `topology` - groups every synced client under the device it is attached to.
- `guest report` - joins hotspot vouchers with currently connected guest clients.
- Agent-native surface: `--agent`, `--json`, `--select`, `--csv`, `--dry-run`, typed
  exit codes, and a provenance envelope marking each result live or local.

### Changed from the upstream print
- `topology` is documented as the device-to-client tree it actually builds. The
  upstream description claimed a gateway-to-switch-to-AP physical tree, which the
  command does not produce - device-to-device uplink chaining is not available from
  the list endpoints.
- Auth is declared as the credentials the binary actually reads, `UNIFI_API_KEY` and
  `UNIFI_GATEWAY_HOST`, replacing an inherited `PRINTING_PRESS_CLIENT_PROFILE`
  declaration that did not match the code.
- Governance adds a **Device / port control** tier for `sites devices adopt`,
  `execute-adopted-action`, `execute-port-action`, and `sites clients
  execute-connected-action`, and reclassifies `sites devices remove` as destructive -
  it factory-resets an online device.
- Slug is `unifi-network`, scoping this skill to the local controller API and leaving
  `unifi` free for the UniFi Site Manager cloud API.
