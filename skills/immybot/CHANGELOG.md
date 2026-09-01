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
