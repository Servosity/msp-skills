# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: Fortinet CLI (`fortinet-cli`) and MCP server
  (`fortinet-mcp`), read-only against FortiOS, FortiManager and FortiGate Cloud.
- Fleet posture audit: `fleet audit` grades every FortiGate in a `devices.toml`
  file PASS / WARN / FAIL / UNKNOWN for firmware, SSL-VPN exposure, admin accounts
  and FortiGuard / FortiCare license expiry; `--new` shows only findings the
  previous run did not have, `--fail-on` exits 6 for a scheduled job.
- Per-check audits with evidence: `audit firmware`, `audit sslvpn`,
  `audit admins`, `audit licenses`.
- `fleet health` (HA sync, IPsec and SD-WAN members down, CPU and memory,
  expiring certificates), `policies hygiene` (any-any accept, unused,
  logging-disabled, disabled policies), and `fleet backup` plus `fleet changes`
  for configuration diffs between backups.
- `fmg` commands for FortiManager ADOMs and managed devices (JSON-RPC `get`
  only) and `cloud` commands for the FortiGate Cloud inventory.
- Typed list / get / monitor / settings commands across the FortiOS 7.4 REST
  API, with a local SQLite mirror (`sync`, `search`).
- FortiOS-encrypted secrets (`ENC ...` values) are redacted from every read,
  sync, export and MCP result, and `plain-text-password` requests are refused;
  `fleet backup` keeps the full configuration on purpose.
  Redaction runs on the decompressed body and decodes every JSON escaping of
  the prefix.
- `audit sslvpn` resolves zones per VDOM, resolves named source addresses and
  groups, exclusions included (a list covering every address is as open as `all`), and grades a 404
  on a VDOM it cannot confirm as UNKNOWN; the FortiGate Cloud token cache is ignored when other users
  can read it.
- `fleet health` grades HA sync only from real checksum objects; `null` on every
  member is UNKNOWN, not in sync.
- Every client refuses a plain `http://` FortiGate URL (loopback test hosts
  excepted), and `--timeout` bounds each request rather than the whole fleet run,
  so large fleets finish and `fleet audit --new` keeps its history.
  `fleet backup` also times out on a device that never answers or stalls mid-file
  (five minutes per download with the default `--timeout`; one `--timeout` when
  you set it).
- A devices file named with `--devices` or `FORTINET_DEVICES` must exist and list
  `[[device]]` entries; a typo is an error, not a silent one-box audit.
- `doctor` makes one authenticated read (`GET /monitor/system/status`) and names
  `FORTINET_BASE_URL` when it is still the shipped placeholder.
