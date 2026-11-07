# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-11-06

### Security
- **Updated two compression libraries to fix GO-2026-6888 and GO-2026-6865.** `github.com/andybalholm/brotli` moves from v1.0.6 to v1.2.8: a crafted Brotli stream could make the program allocate excessive memory while decompressing it (GO-2026-6888). `github.com/klauspost/compress` moves from v1.18.7 to v1.20.2: a crafted zstd stream with two-block frames could make the program crash with a panic while decompressing it (GO-2026-6865). No other dependency changed and commands behave the same.

## [0.1.0] - 2026-11-05

### Added
- Initial msp-skills release: Exchange Online CLI and MCP server over Microsoft's REST admin
  endpoints (the Admin API v2.0 and the cmdlet route), one customer tenant per call, no
  PowerShell session.
- 60 cmdlet commands for mailboxes, permissions, Send As and Send on behalf, folder
  permissions, inbox and transport rules, client protocols, mobile devices, message trace,
  quarantine, policies and connectors.
- Compound commands: `forwarding-audit`, `inbox-rules sweep`, `bec triage`, `ews-posture`,
  `access-map`, `offboard` (per-step receipt, `--plan` to preview), `where-is`,
  `quarantine snapshot` and the cross-tenant `quarantine queue`.
- MCP: `exchange-online_read` serves the Get-* cmdlets (also under
  `EXCHANGE_ONLINE_MCP_READ_ONLY=1`); every state-changing cmdlet and `offboard` preview
  until the call carries `confirm: true`.
- Credential safety: requests go only to the configured Exchange Online host (redirects and
  off-host paging links are refused), the on-disk response cache is off (reads, sweeps and snapshots go to the local store with secret fields stripped), and a held
  credential is withheld from every output, file and outbound request.
