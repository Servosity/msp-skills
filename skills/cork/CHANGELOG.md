# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - 2026-09-01

### Fixed
- **`recall` and `playbook list` no longer claim to be read-only.** Both open the writable
  learn store and record a row, but were annotated `mcp:read-only=true` - and that annotation
  is what an MCP host reads to decide what to auto-approve without asking you. They are now
  `mcp:local-write`, a tier this engine already defines and already uses for `teach`: writes
  land only in the CLI's own local store, never in external state and never in a user-visible
  file. Measured at the live MCP server, both tools moved from `readOnlyHint=true,
  openWorldHint=true` to `readOnlyHint=false, destructiveHint=false, openWorldHint=false`.
  No behaviour changed - both commands write exactly what they wrote before. The promise was
  the defect, not the write.

### Changed
- Install and remote-agent documentation corrected against the shipped binaries. The remote
  section named `mcp-remote`, which bridges the opposite direction and cannot publish a local
  stdio server at all; connectors that parse `--transport http` now point at the native flag
  and the rest at `supergateway`. The HTTP endpoint is `/mcp`, not the bare root the docs gave.
  The Windows install path and a fallback paragraph describing an `npx` install that is not
  offered were both wrong and are gone. A new `check_install_docs` gate holds these claims
  against the binaries and installers so they cannot drift again.

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `CORK_BASE_URL`, `CORK_USER_AGENT`, `PRINTING_PRESS_CLIENT_PROFILE`.

## [0.1.0]

### Added
- Initial msp-skills release of the Cork connector: `cork-cli` and the `cork-mcp`
  MCP server, covering the full Cork REST surface (clients, devices, domains,
  inboxes, score history, compliance events and event types, vulnerabilities,
  software packages, integrations and their tenants/devices/users/credentials,
  warranties, invoices and line items, distributor, and the authenticated user).
- Offline SQLite mirror with `sync` and full-text `search`, so cross-client
  questions answer from local data instead of an O(clients) API fan-out.
- Eight commands the Cork API cannot answer in one call:
  - `score attribute` differences the claims, compliance, coverage, and
    vulnerability components of a client's risk score across a window and ranks
    which one drove the move.
  - `score regressions` ranks the whole book of business by score delta.
  - `vulnerabilities triage` orders software products by exploitability
    (known-exploited first, then EPSS, then CVSS) with a blast-radius device
    count. The API's server-side `sort_by` accepts only `sw_vendor` and
    `sw_product`, so this ordering is impossible upstream.
  - `vulnerabilities exposure` answers "are we exposed to CVE-X" by scanning for
    a CVE id that exists only nested inside `cves[]` with no endpoint filter.
  - `compliance overdue` joins compliance events against the event-type catalog
    to find events past their remediation window. The two halves live on
    different endpoints and nothing joins them.
  - `integrations health` catches the connector reporting `connection_status: ok`
    while its `last_synced_at` has gone stale.
  - `coverage gaps` diffs connector-reported devices against client-attributed
    devices on `associated_endpoints[].integration_identifier`.
  - `warranties exposure` ranks unwarranted and lapsed clients by current risk.
- `--agent` mode (JSON, non-interactive), `--dry-run` previews on every mutating
  command, typed exit codes, and `export`/`import` for JSONL backup and migration.
