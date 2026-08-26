# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `SKYKICK_BASE_URL`, `SKYKICK_TOKEN_URL`.

### Fixed
- OAuth client credentials supplied through the environment (`SKYKICK_CLIENT_ID` and
  `SKYKICK_CLIENT_SECRET`) are no longer written to
  `~/.config/skykick-cli/config.toml`. `Load()` marks env-supplied values so
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
- Initial msp-skills release: `skykick-cli` plus the `skykick-mcp` MCP server
  (20 tools), targeting the current `apis.cloudservices.connectwise.com` host.
- `fleet-sync` - one command pulls every subscription plus per-tenant settings,
  retention, autodiscover, snapshot stats, mailboxes, sites, and alerts into a
  run-versioned local SQLite store (last 5 runs kept so `drift` can diff).
- Cross-tenant posture views the per-subscription API can't produce:
  `fleet-health`, `stale-snapshots`, `coverage-gaps`, `retention-audit`,
  `autodiscover-audit`, `partner-rollup`.
- `drift` - diffs the two most recent fleet-syncs for protection-state changes.
- `alert-sweep` - fleet-wide ranked alert triage with optional bulk mark-complete
  (`--complete <ids> --apply`); `watch-operation` polls async discovery to a
  terminal state.
- Offline SQLite mirror with full-text search, `--agent` JSON mode, and
  `--data-source auto|live|local` on every read.

