# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.7] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given this command the hidden alias `create`, so typing `create` in its place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `acronis-cli agent-manager create` ran `acronis-cli agent-manager force-agent-update`, POST `/api/agent_manager/v2/agents/update:force`. The alias is removed; the command still runs under its own name. If a script reached it through `create`, call it by the name below:
  - `acronis-cli agent-manager force-agent-update` (POST `/api/agent_manager/v2/agents/update:force`)

## [0.1.6] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.5] - 2026-09-28

### Changed
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but the `.mcpb` used to contain only the MCP server, so a one-click
  install on a machine without the CLI failed at the first tool call. The bundle now ships
  both binaries side by side for macOS (universal), Windows x64 and Linux x64. Older
  bundles are unchanged; this release's bundle is the first for this connector to include it.
- **The MCP server reports its real version.** `serverInfo.version` used to read
  `0.0.0-dev` (or a hard-coded literal) in every released MCP binary because the release
  build stamped the version into the CLI only. The release now stamps both and checks the
  built server's `initialize` reply against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256 sidecars
  before touching anything, and replace the old binaries in a single transaction that is
  undone if any step fails. (Served from the repository, so this applies to every install
  from now on, not only this version.)
- No connector code changed in this release; it exists so the published bundle and
  binaries pick up the release-pipeline fixes above.

## [0.1.4] - 2026-09-11

### Fixed
- Resolve the API client's tenant subtree for default sync and search; sync agents,
  per-tenant usages, and offering items as well as the existing resources.
- Follow Acronis nested cursors, including short pages with a next cursor. Reject
  partial pages, failed child requests, and dropped rows; incomplete mirrors cannot
  drive fleet rollups. Existing mirrors require a successful unfiltered full sync.
- Translate task filter and sort syntax in CLI and MCP, applying exact tenant
  filtering locally across all pages. Translate remote search parameters.
- Map nested task tenant/result fields and preserve distinct billing editions and
  infrastructure rows. Report unmatched legacy agent tenant identities explicitly.
- Persist the datacenter API URL during login and stop mislabeling parameter
  validation errors as credential failures.
- Thanks @asterisk79 for the multi-tenant runtime report in #325. Offline fixtures
  cover these contracts; confirmation against the partner's tenant remains pending.

## [0.1.3] - 2026-09-10

### Fixed
- **A tool-call value could smuggle a refused flag past the MCP server.** The MCP server
  handed each tool argument to the CLI as two separate words, the flag and then its value.
  On a yes/no flag the CLI does not read the next word as a value, so a value that itself
  began with `--` was read as a brand-new flag, including flags the server deliberately
  refuses such as `--deliver`, which can send command output to a URL. Every argument that
  carries a value now travels glued to its flag as one word (`--flag=value`), so the CLI only
  ever reads it as the value of that one named flag, or rejects it, and it can never become a
  second flag. A plain yes/no flag is still a bare `--flag`, and an empty value is still left out.
  The recipe shortcut tools, which build their own command line, now also refuse a value
  that begins with `-` where a plain value is expected.
  Reported privately through SECURITY.md; the same fix is in the pending DataGate connector.

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
  It also dialled the shipped placeholder base URL (`https://{datacenter}.acronis.com`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `ACRONIS_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `ACRONIS_BASE_URL`, `ACRONIS_CLIENT_ID`, `ACRONIS_CLIENT_SECRET`.

### Changed
- Regenerated the vendored `acronis-cli` / `acronis-mcp` source from
  cli-printing-press 4.24.0 and re-rendered the templated skill surfaces. No
  changes to command names or flags; the local mirror, search, and cross-tenant
  rollups behave as before.

## [0.1.0]

### Added
- Initial msp-skills release: the `acronis-cli` CLI and `acronis-mcp` MCP server
  for Acronis Cyber Protect Cloud, with an offline SQLite mirror (`sync`) and
  full-text `search`.
- Cross-tenant backup rollups: `health`, `failures`, `freshness`, and
  `agents stale` answer "whose backups failed" and "which agents went offline"
  across every customer tenant in one table.
- Billing and posture views: `coverage --unprotected`, `reconcile usages`,
  `usages drift`, `agents compliance`, `tenants offering-items inventory`, and
  the `customer` 360 card.
- Tenant operations: `tenants` (list/get/create/update/delete/audit), `clients`,
  `agent-manager`, `task-manager`, plus full API coverage via `api`.
