# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
  authenticate. Now declared on every install channel: `SHERWEB_BASE_URL`, `SHERWEB_TOKEN_URL`.

### Fixed
- OAuth client credentials supplied through the environment (`SHERWEB_CLIENT_ID` and
  `SHERWEB_CLIENT_SECRET`) are no longer written to
  `~/.config/sherweb-cli/config.toml`. `Load()` marks env-supplied values so
  `configForSave()` can keep them off disk, but `SaveTokens()` cleared those
  markers before saving, so every token mint - including the automatic one on an ordinary
  authenticated command, with no `auth login` step involved - rewrote the live client ID and secret into
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
- Initial msp-skills release: the `sherweb-cli` CLI and `sherweb-mcp` MCP server
  for the Sherweb Partner API (Distributor billing + Service Provider customers,
  subscriptions, platforms, catalog, and orders).
- Composed authentication: OAuth2 client-credentials bearer token plus the APIM
  gateway subscription-key header sent on every call.
- Offline SQLite mirror via `sync` + `deep-sync`, with resumable incremental
  sync and full-text search.
- Cross-entity analytics that join payable and receivable data: `margin`,
  `margin-trend`, `orphans`, `usage-leak`, `right-size`, `drift`, `sub-changes`,
  `fleet-subs`, and read-only `amend-preview`.
- Agent-friendly output (`--agent`, `--json`, `--compact`, `--select`), a
  natural-language `which` command resolver, and a `doctor` health check.
