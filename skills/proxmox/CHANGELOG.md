# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - 2026-11-07

### Changed
- **Rebuilt on CLI Printing Press 4.35.1 (was 4.33.5).** See [GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c) for why the press was upgraded.
- **The MCP server has a token-protected HTTP mode for remote agents.** `proxmox-mcp --transport http` refuses to start until `PROXMOX_MCP_HTTP_TOKEN` (32 characters or more) is set, rejects any request without that bearer token, refuses a request body over 4 MiB or one slower than 30 seconds, serves `http://127.0.0.1:7777/mcp` by default and needs `--tls-cert` and `--tls-key` for any other address. stdio stays the default, and msp-skills does not support web-only AI apps (ChatGPT on the web, Copilot Studio, the Gemini app) on it yet; `mcp-install.md` documents the mode for reference.
- New install setting on every channel: `PROXMOX_NO_LOCAL_DATA` (set to `1` to keep no local copy: store writes off, `auto` reads go live, `sync` and `workflow archive` refuse to run). `local-data purge` previews what it would erase and deletes only with `--yes`.
- New `--ca-cert` (`PROXMOX_CA_CERT`) and `--tls-server-name` (`PROXMOX_TLS_SERVER_NAME`) trust a private CA for a Proxmox host that has no certificate fingerprint pin; a pinned host still accepts only its pin.
- `--csv` prefixes `'` on a cell that a spreadsheet would run as a formula. `--deliver file:` writes a private 0600 file; `--deliver webhook:` no longer follows redirects.
- `sync` no longer has the `ceph-metadata` and `cluster-config-nodes` partitions, and its `cluster` partition now reads `/cluster/config/nodes`. No fleet command (posture, backup coverage, verify-health, failures, capacity, restore-points, rpo) reads the local store, so their answers are unchanged.

### Security
- **Adopts the shared credential kit.** Nothing the CLI or the MCP server prints, writes to a file or to the local store, or sends to another host can carry the secret of a Proxmox token (from `PROXMOX_API_TOKEN`, `PROXMOX_PBS_API_TOKEN`, a clusters.toml entry, `config.toml` or `credentials.toml`), `PROXMOX_MCP_HTTP_TOKEN` or `SPACEFAST_TOKEN`; the kit prints a withheld notice in its place. Each token goes only to its own service or cluster host, over https. A token's id (`USER@REALM!TOKENID`) is not secret and still prints, so task lists that name it read in full.

### Fixed
- A NAS or PBS named by host name in one client's cluster and by IP address in another is recognized as one server, so a backup archive on it is no longer credited to the wrong client. A host name this machine cannot resolve is treated as possibly any server of that type and path: its archives are reported as shared, with the unresolved name, instead of being credited to one client.

## [0.1.2] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

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
