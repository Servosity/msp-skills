# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.0] - 2026-10-19

### Added
- Initial msp-skills release: `proxmox-cli` + `proxmox-mcp` for Proxmox VE and
  Proxmox Backup Server. Read-only by construction: every request sent to
  Proxmox is a GET, and a few read endpoints that return stored secrets are
  excluded.
- Backup assurance across every client cluster: `posture` (one verdict per
  guest from enabled-job coverage, real archive age, PBS verify state and the
  last vzdump attempt), `backup coverage`, `failures`, `verify-health`,
  `capacity`, `restore-points` and `rpo`.
- Multi-client setup with `clusters add` / `clusters list`: one entry per
  cluster, tokens referenced by env var name, never stored in the file.
- PBS calls routed to the PBS host with the `PBSAPIToken=` scheme, and SHA-256
  certificate pinning per service for self-signed Proxmox certificates.
