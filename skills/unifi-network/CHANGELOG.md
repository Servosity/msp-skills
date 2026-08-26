# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

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
