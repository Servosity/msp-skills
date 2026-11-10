# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - unreleased

### Fixed
- **Writing to the local store costs almost no CPU.** The credential check on rows written to the local store read every row through each decoder it knows. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).

## [0.1.2] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **Local store memory no longer climbs.** The credential check on rows written to the local store no longer grows with each page or spikes on one large record. The check is exactly as strict as before (#612).
- **One withheld row no longer drops the rest.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch, so every other row is stored (#612).

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
