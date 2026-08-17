# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Describe the changes in this release.

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
