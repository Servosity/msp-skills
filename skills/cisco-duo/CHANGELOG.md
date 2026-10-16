# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
- Initial msp-skills release: Cisco Duo CLI (`cisco-duo-cli`) and MCP server
  (`cisco-duo-mcp`) for the Duo Admin API, with every request signed (Duo v5
  HMAC-SHA512) and MSP subaccount routing by ID or name (`--account-id`).
- Lockout desk: `lockout triage` (read-only diagnosis and next step),
  `lockout fix` (unlock, single-use bypass code, Duo Mobile reactivation) and
  `phone-swap`; both fixes preview by default and change Duo only with `--apply`.
- MSP fleet views across every subaccount: `fleet summary`, `fleet risk` and
  `fleet find-user`.
- Tenant hygiene: `audit bypass`, `audit stale`, `enrollment chase` and
  `failures`, with a local SQLite mirror for the audits.
- `doctor` verifies the credential with one signed read and names the variable
  to fix when the API host is unset or wrong.
- `CISCO_DUO_MCP_READ_ONLY=1` hides the MCP tools that change Duo and refuses
  every non-GET API call.
