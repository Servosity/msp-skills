# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

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
