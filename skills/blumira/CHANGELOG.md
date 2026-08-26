# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

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
  authenticate. Now declared on every install channel: `BLUMIRA_BASE_URL`.

### Fixed
- OAuth client credentials supplied through the environment (`BLUMIRA_CLIENT_ID` and
  `BLUMIRA_CLIENT_SECRET`) are no longer written to
  `~/.config/blumira-cli/config.toml`. `Load()` marks env-supplied values so
  `configForSave()` can keep them off disk, but `SaveTokens()` cleared those
  markers before saving, so `auth login` rewrote the live client ID and secret into
  the config file in cleartext. That defeats the setups this is meant to
  support, where the secret lives only in the OS credential store (Keychain,
  Windows Credential Manager) and reaches the process as an environment
  variable at launch. The minted access token is still cached there, and an
  explicit `auth login --client-id <id> --client-secret <secret>` still stores
  what you passed it. **Deliberate behaviour change:** credentials that came
  from the environment are no longer persisted by `auth login` either, so a
  shell that later drops those variables must set them again or pass the flags.
  Thanks to @Xenith-B for the report (#266).

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `blumira-cli` CLI and `blumira-mcp` MCP server,
  vendored source-only under `cli/` (Apache-2.0).
- Offline SQLite mirror via `sync`, with full-text `search` and `evidence-search`
  over synced findings, evidence, agents, and detection rules.
- Cross-account MSP analytics no single API call composes: `triage` (one ranked
  open-findings queue across every sub-account), `drift`, `velocity` (MTTR /
  open-rate), `sla` (time-to-breach watchlist), `coverage` (detection drift vs the
  basis ruleset), `exposure` and `dc-roster` (stale / unprotected agents and
  domain controllers), `audit` (resolved-then-refired), `recurring`, `overview`,
  `reconcile`, and `workload`.
- Self-minting OAuth2 auth: `auth login` mints and caches a Blumira JWT from a
  Client ID + Secret, or set `BLUMIRA_API_TOKEN` directly.
- Agent-ready: `--agent` mode for JSON output, `--dry-run` previews for writes,
  and `agent-context` for capability discovery.
