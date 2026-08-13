# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Fixed
- **Cross-tenant fan-out found zero tenants on current CIPP builds.** `/ListTenants`
  now returns its array inside a `{"Results": [...]}` envelope; tenant enumeration
  only understood a bare array, so it treated the whole envelope as a single
  tenant, dropped it for having no domain, and reported an empty fleet against a
  healthy CIPP. Envelope responses are now unwrapped, so `--all-tenants` fan-out
  and `posture` see the whole fleet again.
- **`sync` pointed at a remedy that cannot work.** CIPP's API is tenant-scoped end
  to end and exposes no bulk-list endpoints, so `sync` is a structural no-op here.
  Its warning suggested `--resources`, which always errors "unknown sync resource";
  it now names the mechanism that actually populates the store:
  `cipp-cli fanout --endpoint <endpoint> --all-tenants --save`.

Thanks to Abhi Saini at Bearium Networks, who found both while running the
connector against a live CIPP tenant.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `cipp-cli` CLI + `cipp-mcp` MCP server for the
  CIPP (CyberDrain Improved Partner Portal) Microsoft 365 multi-tenant API.
- **Cross-tenant fan-out** (`fanout`): run one read across every client tenant
  with throttle-aware backoff, optional persistence, and resume-after-halt.
- **Cross-tenant posture matrix** (`posture`): one table of every tenant's MFA,
  Conditional Access, Standards, and BPA posture.
- **License waste reconciler** (`licenses waste`) and **stale-account sweep**
  (`users stale`) across all tenants from the local store.
- **Standards drift report** (`standards drift`): tenants whose security
  baseline regressed between two synced snapshots.
- **Throttle-aware bulk executor** (`bulk`): drive add-user / offboard /
  remove-user / set-forwarding from a CSV with 429 backoff and resume; plans by
  default, writes only with `--execute`.
- Offline SQLite store, `--agent` JSON mode, `--dry-run`, and `doctor` health
  check.
