# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Fixed
- OAuth client credentials supplied through the environment (`PAX8_CLIENT_ID` and
  `PAX8_CLIENT_SECRET`) are no longer written to
  `~/.config/pax8-cli/config.toml`. `Load()` marks env-supplied values so
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
- Initial msp-skills release: `pax8-cli` and `pax8-mcp` for the Pax8 Partner API.
- Offline SQLite mirror with incremental `sync` and FTS5 `search`.
- Billing analytics the portal does not compose in one place: `reconcile`
  (billing leakage, with `--draft` pre-check), `mrr` (recurring revenue and
  margin by product), `overage` (metered usage before it invoices), `spend`
  (customers ranked by invoice total), and `since` (subscription change feed).
- `company show` customer-360 joining a company to its subscriptions, contacts,
  invoices, and usage.
- Full Partner API coverage (companies, contacts, subscriptions, orders,
  products, invoices, usage) via typed commands and the `api` browser.
- Agent-native output (`--agent`, `--select`, `--dry-run`), named profiles,
  output delivery sinks, and OAuth2 client-credentials auth.
