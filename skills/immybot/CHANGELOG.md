# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.11] - unreleased

### Security
- **The token request no longer follows a redirect to another server.** `auth login` and the automatic sign-in sent the token request, with your client secret, on to whatever host a redirect from the token URL named. A redirect off the token server now fails instead.

### Fixed
- **`--dry-run` sends nothing, not even a token request.** With no saved token, a dry run used to request one first, sending your client ID and secret to the token URL and saving the token, although a dry run promises that no request is sent. `auth login --dry-run` now also only shows the request it would make, and `auth set-token --dry-run` and `auth logout --dry-run` no longer save or clear your stored credentials.

## [0.1.10] - 2026-10-22

### Security
- The MCP server no longer opens a local file named by a binary multipart (upload) field and sends it to the vendor; run such an upload with the CLI.

## [0.1.9] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `immybot-cli access create` ran `immybot-cli access create-delete-azure-tenant-auth-details`, POST `/api/v1/auth/delete-azure-tenant-auth-details`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `immybot-cli access create-delete-azure-tenant-auth-details` (POST `/api/v1/auth/delete-azure-tenant-auth-details`)
  - `immybot-cli application-locks create-request-cancellation` (POST `/api/v1/application-locks/request-cancellation`)
  - `immybot-cli billing create-cancel-subscription` (POST `/api/v1/billing/cancel-subscription`)
  - `immybot-cli computers ephemeral-agent-circuit-breaker create-reset` (POST `/api/v1/computers/{computerId}/ephemeral-agent-circuit-breaker/reset`)
  - `immybot-cli maintenance-sessions actions create-by-id-rerun` (POST `/api/v1/maintenance-sessions/{sessionId}/actions/{actionId}/rerun`; re-runs a maintenance action (software install, patch or task) on the computer)
  - `immybot-cli maintenance-sessions create-cancel` (POST `/api/v1/maintenance-sessions/cancel`)
  - `immybot-cli metrics create-circuit-breakers-isolate` (POST `/api/v1/metrics/circuit-breakers/isolate`)
  - `immybot-cli plugins create-api-v1-by-provider-link-id-by-catch-all` (POST `/plugins/api/v1/{providerLinkId}/{catchAll}`; passes an arbitrary POST through to a provider plugin's API)
  - `immybot-cli provider-agents create-bulk-delete-pending` (POST `/api/v1/provider-agents/bulk-delete-pending`)
  - `immybot-cli provider-links cross-references create-by-external-link-id-disable-client-linking` (POST `/api/v1/provider-links/{id}/cross-references/{externalLinkId}/disable-client-linking`)
  - `immybot-cli scripts create-debug-cancel-by-cancellation-id` (POST `/api/v1/scripts/debug/cancel/{cancellationId}`)
  - `immybot-cli system create-disable-immy-support-access` (POST `/api/v1/system/disable-immy-support-access`)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.8] - 2026-10-15

### Fixed
- **Windows: the built-in Administrator account can save credentials.** On the built-in
  local Administrator (common on MSP servers and jump boxes), saving credentials failed
  with `token file DACL grants access to a non-owner principal (LA)`, and reads refused
  the saved file. The token-file permission check compared security-descriptor text
  instead of account SIDs; it now resolves each trustee to a SID first. The allowed
  principals are unchanged: your account, BUILTIN\Administrators and SYSTEM.
- **Windows: a token file with a NULL DACL is now refused.** A NULL DACL (no access
  control at all, so any account on the machine can read the file) written with its
  flags first, which is how Windows Server 2025 renders it, passed the check. The check
  now parses the DACL exactly and refuses anything it cannot parse.

## [0.1.7] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.6] - 2026-10-02

### Fixed

- Expose executor request schemas in MCP endpoint discovery, including required inputs, wire-key case, nested bodies, and known enum values. Search now finds request fields and exact short enum values (#351).
- Preserve separate path, query, header, and body inputs, including endpoints that reuse a name in more than one location. Existing CLI commands and custom workflows are retained.

## [0.1.5] - 2026-09-18

### Changed
### Fixed
- **`workflow archive` no longer reports success when nothing was archived.** It counted
  successes but exited 0 and printed the summary even when every resource failed
  (for example on an expired credential). Zero of N now exits non-zero with the true count;
  partial success still exits 0.
- The MCP server's config-resolution test that every other connector carries was missing;
  added, proving the server reads the same config file the CLI does.

### Changed
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but the `.mcpb` used to contain only the MCP server, so a one-click
  install on a machine without the CLI failed at the first tool call. The bundle now ships
  both binaries side by side for macOS (universal), Windows x64 and Linux x64. Older
  bundles are unchanged; this release's bundle is the first to include it.
- **The MCP server reports its real version.** `serverInfo.version` used to read
  `0.0.0-dev` (or a hard-coded literal) in every released MCP binary because the release
  build stamped the version into the CLI only. The release now stamps both and checks the
  built server's `initialize` reply against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256 sidecars
  before touching anything, and replace the old binaries in a single transaction that is
  undone if any step fails. (Served from the repository, so this applies to every install
  from now on, not only this version.)

## [0.1.4] - 2026-09-10

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

### Added
- **`IMMYBOT_NO_CONFIG_WRITE=1` keeps every credential off disk.** Set it and the CLI mints a token per
  invocation instead of caching one in `config.toml`. The MCP server honours the same switch,
  which it previously could not: it resolved its own config path and ignored `--config`
  entirely, so a Claude Desktop setup kept a plaintext token cache even when the CLI had been
  told not to write one. `auth logout` still clears a config that already exists - the switch
  suppresses credential writes, not the erase.

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
  It also dialled the shipped placeholder base URL (`https://{subdomain}.immy.bot`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `IMMYBOT_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `IMMYBOT_BASE_URL`, `IMMYBOT_TOKEN_URL`, `IMMYBOT_USER_AGENT`, `PRINTING_PRESS_CLIENT_PROFILE`.

## [0.1.0] - 2026-08-22

### Added
- Initial msp-skills release of the ImmyBot connector: `immybot-cli` and the
  `immybot-mcp` MCP server, covering the ImmyBot API (computers, tenants,
  software, target assignments, maintenance sessions and actions, scripts,
  schedules, provider and RMM links, persons, roles, and access).
- Offline SQLite mirror with `sync` and full-text `search`, so cross-tenant
  questions answer from local data instead of one API call per tenant.
- Cross-tenant views the console does not offer: `session-triage` (groups a
  maintenance window's failures by root cause), `version-spread` (one software
  title ranked across every tenant with a real semver comparator),
  `assignment-explain` (which deployment rule actually won on a given machine,
  and which are shadowed), `fleet-diff` (what changed between two syncs, from
  history the API does not retain), `deployment-health`, `onboarding-stalled`,
  `computer-dossier`, `drift`, `psa-reconcile`, and `script-blast-radius`.

### Fixed
- `auth login` no longer persists an env-supplied client secret to disk:
  `SaveTokens` keeps the ClientID/ClientSecret env-override markers, so the
  guards added in #268 actually hold.
- `sync` mirrors every computer. `GET /api/v1/computers` exposes no cursor,
  page, skip or offset, so `pageSize` is a ceiling rather than a page size and
  the profiler's spec default of 25 silently truncated the resource that the
  offline commands join on most.
- Three parameter-required lookup endpoints are out of the default sync walk
  (they returned HTTP 400 on every run) and remain reachable via `--resources`.
- Staleness reports `never_synced` separately instead of reading a zero
  watermark as a 292-year-old mirror.
- `doctor` reports the instance subdomain as its own check, and every auth
  hint - in `doctor`, `auth status`, `auth login --help`, the runtime error
  paths, and the MCP tool layer - names the four required `IMMYBOT_*`
  variables and points at `auth login` rather than an `auth setup` subcommand
  this CLI does not define.

Connector contributed by [@geekbrownbear](https://github.com/geekbrownbear)
(PR [#276](https://github.com/servosity/msp-skills/pull/276)), live-verified
against a production ImmyBot tenant.
