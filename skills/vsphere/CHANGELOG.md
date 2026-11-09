# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.0] - 2026-11-06

### Added
- First release: `vsphere-cli` and the `vsphere-mcp` server for VMware vCenter,
  read-only, built on both the vSphere Automation API and the VI/JSON API.
- Migration planning: `migration blockers` (per-VM verdict with reason codes),
  `migration capacity` (target sizing totals) and `migration proxmox-map`
  (Proxmox VE settings per VM).
- `snapshots audit`: snapshot age, chain depth, delta-disk bytes and VMs that
  need consolidation.
- `licence exposure`: licensable cores under Broadcom's 16-core-per-CPU minimum,
  with an optional estimate at your own per-core price.
- `rvtools export`: the RVTools CSV layout from macOS, Linux or Windows.
- List and get commands for VMs, hosts, clusters, datacenters, datastores,
  networks, folders, resource pools, tags and VM hardware.

### Security
- The password goes only to vCenter's login endpoints; session ids stay in memory.
- Requests are pinned to the configured vCenter; redirects elsewhere are refused.
- Credentials are guarded by the shared credential kit: output, files, requests,
  SQL statements, child processes and MCP messages that carry one are withheld whole.
- Optional certificate pinning with `VSPHERE_TLS_FINGERPRINT` for self-signed vCenters.
- Output, files and MCP results never repeat a credential or session id.
