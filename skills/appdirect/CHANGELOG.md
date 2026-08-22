# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Fixed
- `doctor`, the CLI's HTTP 400/401/403 error hints and the MCP server's tool-error
  hints no longer tell operators to run `appdirect-cli auth setup`. This CLI's `auth`
  parent defines `login`, `status`, `set-token` and `logout`; it has never defined
  `setup`. Cobra does not reject an unknown subcommand of a non-root parent, and the
  generated parent returns `cmd.Help()` with a nil error, so `appdirect-cli auth setup`
  printed the auth help and exited 0 - no "unknown command" error and no non-zero
  exit anywhere in the journey. An operator followed doctor's advice, saw a success
  code and was no closer to configured credentials; because these are agent-first
  CLIs with typed exit codes and an `--agent` mode, an agent performing setup read
  exit 0 as success and moved on unauthenticated. The hints fire exactly when they
  matter - fresh install, rotated secret, new machine, expired token - and this skill
  emits no `auth_instructions` field, so they were the only setup guidance it gave.
  `doctor` now names the variables `auth login` actually reads (`APPDIRECT_CLIENT_ID` and `APPDIRECT_CLIENT_SECRET`, plus
  `APPDIRECT_OAUTH_SCOPE` when the account needs a non-default scope), and the
  CLI and MCP error paths point at `appdirect-cli auth login` to re-authenticate. Seven
  occurrences across three files; the same defect affected appdirect, crowdstrike,
  halopsa and ninjaone, the four connectors that scaffold `auth login` rather than
  `auth setup`. Reported by @geekbrownbear (#278).
- OAuth client credentials supplied through the environment (`APPDIRECT_CLIENT_ID` and
  `APPDIRECT_CLIENT_SECRET`) are no longer written to
  `~/.config/appdirect-cli/config.toml`. `Load()` marks env-supplied values so
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
- Initial msp-skills release: `appdirect-cli` and `appdirect-mcp`, covering the
  documented AppDirect marketplace REST surface - companies, users, memberships,
  subscriptions, billing, assisted sales, catalog, and checkout.
- Offline SQLite mirror (`sync`) with full-text `search` and `analytics`.
- Cross-company billing views: `reconcile` (active-but-unbilled, overdue,
  failed-payment), `payments unpaid`, and `subs changed`.
- Single-customer rollup `company show` and assisted-sales `pipeline` /
  `pipeline stale`.
- OAuth2 client_credentials auth with an invisible token mint/refresh and a
  white-label `APPDIRECT_BASE_URL` override.
