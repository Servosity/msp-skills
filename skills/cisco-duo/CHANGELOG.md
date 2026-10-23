# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.4] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

## [0.1.3] - 2026-10-22

### Security
- The MCP server no longer passes `import --input` to the CLI, so a tool call cannot point the import at a file on the machine that runs the server.

## [0.1.2] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `cisco-duo-cli identity-verification create` ran `cisco-duo-cli identity-verification cancel`, POST `/admin/v2/identity_verification/{user_id}/cancel`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `cisco-duo-cli identity-verification cancel` (POST `/admin/v2/identity_verification/{user_id}/cancel`)
  - `cisco-duo-cli registered-devices block-device` (POST `/admin/v1/registered_devices/blocked/{compkey}`)

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
