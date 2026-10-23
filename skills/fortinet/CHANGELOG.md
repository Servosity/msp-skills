# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.4] - unreleased

### Fixed
- **Windows: two runs refreshing the FortiGate Cloud token at once no longer delete the cached token.** Saving the token cache failed with "Access is denied" while another run had it open, and the failure deleted the cache, so the next run had to log in again. The cache is now written the same way as the credentials file, with the same brief retry.

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `fortinet-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.3] - 2026-10-22

### Security
- The MCP server no longer passes `--devices` (on every fleet, audit and policies command) or `fleet changes --dir` to the CLI. A devices file names each device's API URL and token file, so a tool call could otherwise have pointed the CLI at a devices file of its choosing. Both flags, and FORTINET_DEVICES, still work on the command line.

## [0.1.2] - 2026-10-21

### Security
- **MCP tool calls can no longer read local files through `teach`.** Over MCP, `teach --playbook-file` / `--playbook-notes-file` and `teach-playbook --playbook-file` / `--notes-file` copied any file the MCP server's account could read into the local learning store, where the `playbook_list` and `recall` tools handed it back. The MCP server now leaves these four parameters out of the tool schemas and refuses a call that names them. Pass the content inline instead (`playbook-json`, `playbook-notes`, `notes`). The CLI is unchanged: run from your own shell, these flags still read your own files.
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

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
