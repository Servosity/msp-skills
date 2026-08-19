# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-06-16

### Changed

- chore(deps): patch x/crypto, quic-go, x/sys in axcient + connectwise-control (#120)
- chore(connectwise-control,rewst): pin toolchain go1.26.4 (patched stdlib) (#118)
- feat(connectwise-control): new Remote Access connector (ConnectWise Control / ScreenConnect, press 4.24.0) (#99)

## [0.1.0]

### Added
- Initial msp-skills release: ConnectWise Control (ScreenConnect) instance CLI +
  MCP server with an offline SQLite mirror for session lookups.
- Sessions: list, get-detail, run-command (guest command execution), add-event-to
  (control events), get-access-token, update-name, update-custom-property.
- Session groups, instance security configuration + user management, and audit-log
  queries (GetAuditInfo / QueryAuditLog).
- Per-instance `CONNECTWISE_CONTROL_BASE_URL` + HTTP Basic auth
  (`CONNECTWISE_CONTROL_USERNAME` / `CONNECTWISE_CONTROL_PASSWORD`).
- Session/host control, access-grant, and user-admin writes gated human-in-the-loop
  in governance.md (run-command executes on a real guest machine).
