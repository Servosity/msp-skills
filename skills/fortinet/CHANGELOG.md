# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-10-21

### Security
- **MCP tool calls can no longer read local files through `teach`.** Over MCP, `teach --playbook-file` / `--playbook-notes-file` and `teach-playbook --playbook-file` / `--notes-file` copied any file the MCP server's account could read into the local learning store, where the `playbook_list` and `recall` tools handed it back. The MCP server now leaves these four parameters out of the tool schemas and refuses a call that names them. Pass the content inline instead (`playbook-json`, `playbook-notes`, `notes`). The CLI is unchanged: run from your own shell, these flags still read your own files.

## [0.1.1] - 2026-10-15

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

## [0.1.0] - 2026-10-15

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
