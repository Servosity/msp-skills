# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

## [0.1.2] - 2026-08-26

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
  authenticate. Now declared on every install channel: `CROWDSTRIKE_BASE_URL`, `CROWDSTRIKE_TOKEN_URL`.

## [0.1.1] - 2026-08-22

### Fixed
- `doctor`, the CLI's HTTP 400/401/403 error hints and the MCP server's tool-error
  hints no longer tell operators to run `crowdstrike-cli auth setup`. This CLI's `auth`
  parent defines `login`, `status`, `set-token` and `logout`; it has never defined
  `setup`. Cobra does not reject an unknown subcommand of a non-root parent, and the
  generated parent returns `cmd.Help()` with a nil error, so `crowdstrike-cli auth setup`
  printed the auth help and exited 0 - no "unknown command" error and no non-zero
  exit anywhere in the journey. An operator followed doctor's advice, saw a success
  code and was no closer to configured credentials; because these are agent-first
  CLIs with typed exit codes and an `--agent` mode, an agent performing setup read
  exit 0 as success and moved on unauthenticated. The hints fire exactly when they
  matter - fresh install, rotated secret, new machine, expired token - and this skill
  emits no `auth_instructions` field, so they were the only setup guidance it gave.
  `doctor` now names the variables `auth login` actually reads (`FALCON_CLIENT_ID` and `FALCON_CLIENT_SECRET`, plus
  `CROWDSTRIKE_OAUTH_SCOPE` when the instance needs a scope), and the
  CLI and MCP error paths point at `crowdstrike-cli auth login` to re-authenticate. Seven
  occurrences across three files; the same defect affected appdirect, crowdstrike,
  halopsa and ninjaone, the four connectors that scaffold `auth login` rather than
  `auth setup`. Reported by @geekbrownbear (#278).
- OAuth client credentials supplied through the environment (`FALCON_CLIENT_ID` and
  `FALCON_CLIENT_SECRET`) are no longer written to
  `~/.config/crowdstrike-cli/config.toml`. `Load()` marks env-supplied values so
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
- Initial msp-skills release: the `crowdstrike-cli` CLI and `crowdstrike-mcp` MCP server.
- Cross-tenant Flight Control fleet rollups over a CID-keyed local SQLite store:
  `fleet sync`, `fleet scorecard`, `fleet alerts`, `fleet vulns`, `fleet stale`,
  `fleet policy-drift`, `fleet remediate`, `fleet trend`, `fleet tenants`, `fleet search`.
- Per-CID Falcon coverage: alerts (modern Alerts API), devices/hosts, Spotlight
  vulnerabilities, prevention policies, and MSSP CID/user-group management.
- Offline sync to local SQLite with full-text `search` and `analytics`, plus
  agent-native JSON (`--agent`) and `--dry-run` preview on every command.
