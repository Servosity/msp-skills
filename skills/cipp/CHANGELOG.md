# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.4] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

- **Credential handling hardened.** The token exchange refused to run over plain HTTP (the
  request body carries the client secret); a vendor error message can no longer echo the secret
  back; an absurd `expires_in` no longer wraps into the past and re-mint on every call; an
  existing config file has its permissions repaired to owner-only instead of keeping whatever
  it had; and a token that cannot be written to disk is reused for the rest of the process
  rather than re-minted on every single request.

## [0.1.3] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  It also dialled the shipped placeholder base URL (`https://your-cipp-instance.example.com`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `CIPP_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `CIPP_BASE_URL`.

- **The connector stopped working after one token lifetime.** `auth login` cached the access token and
  its expiry, but nothing ever read that expiry and no 401 triggered a refresh, so roughly an hour later
  every call failed until you re-ran `auth login` by hand. The client now refreshes from the client
  credentials it already persisted. If a refresh fails while the cached token is still valid it warns and
  carries on rather than breaking a working install, and the token exchange refuses cross-host redirects
  so the client secret cannot be replayed to a redirect target.

  Existing installs need one `cipp-cli auth login` to record the tenant id the refresh needs; until then
  the connector says so explicitly instead of failing with an opaque 401.

## [0.1.2] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

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
