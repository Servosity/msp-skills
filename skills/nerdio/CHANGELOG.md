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
  It also dialled the shipped placeholder base URL (`https://nmm.example.com`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `NERDIO_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

### Fixed
- OAuth client credentials supplied through the environment (`NERDIO_CLIENT_ID` and
  `NERDIO_CLIENT_SECRET`) are no longer written to
  `~/.config/nerdio-cli/config.toml`. `Load()` marks env-supplied values so
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
- Initial msp-skills release: the `nerdio-cli` CLI and `nerdio-mcp` MCP server for
  the Nerdio Manager for MSP (NMM) Partner REST API.
- Cross-account fleet commands: `fleet autoscale-audit` (pools with autoscale off
  or drifting), `fleet host-estate` (every session host and its power state), and
  `fleet billing-rollup` (per-customer billed/unpaid/usage rollup).
- `usages drift` for month-over-month consumption comparison across accounts.
- `job wait` - poll any async NMM job to a terminal state with a typed exit code,
  plus `scripted-actions fan-run` to execute one action across many accounts and
  wait for all of them.
- Coverage for host pools, session hosts, desktop images, Intune devices, backup
  and recovery vaults, reservations, networks, scripted actions, and secure
  variables.
- Offline SQLite mirror (`sync`) with full-text `search`, OAuth2 client-credentials
  auth against each MSP's own NMM instance, and `--agent` JSON output mode.
