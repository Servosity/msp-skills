# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
