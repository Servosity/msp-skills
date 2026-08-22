# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.9] - unreleased

### Fixed
- `doctor`, the CLI's HTTP 400/401/403 error hints and the MCP server's tool-error
  hints no longer tell operators to run `ninjaone-cli auth setup`. This CLI's `auth`
  parent defines `login`, `status`, `set-token` and `logout`; it has never defined
  `setup`. Cobra does not reject an unknown subcommand of a non-root parent, and the
  generated parent returns `cmd.Help()` with a nil error, so `ninjaone-cli auth setup`
  printed the auth help and exited 0 - no "unknown command" error and no non-zero
  exit anywhere in the journey. An operator followed doctor's advice, saw a success
  code and was no closer to configured credentials; because these are agent-first
  CLIs with typed exit codes and an `--agent` mode, an agent performing setup read
  exit 0 as success and moved on unauthenticated. The hints fire exactly when they
  matter - fresh install, rotated secret, new machine, expired token - and this skill
  emits no `auth_instructions` field, so they were the only setup guidance it gave.
  `doctor` now names the variables `auth login` actually reads (`NINJAONE_CLIENT_ID` and `NINJAONE_CLIENT_SECRET`, plus `NINJAONE_OAUTH_SCOPE`
  when the instance needs a scope), and the
  CLI and MCP error paths point at `ninjaone-cli auth login` to re-authenticate. Seven
  occurrences across three files; the same defect affected appdirect, crowdstrike,
  halopsa and ninjaone, the four connectors that scaffold `auth login` rather than
  `auth setup`. Reported by @geekbrownbear (#277).

## [0.1.8] - 2026-08-19

### Fixed
- OAuth client credentials supplied through the environment (`NINJAONE_CLIENT_ID` and
  `NINJAONE_CLIENT_SECRET`) are no longer written to
  `~/.config/ninjaone-cli/config.toml`. `Load()` marks env-supplied values so
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

## [0.1.7] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.6] - 2026-07-10

### Fixed
- Device tagging now works through the MCP server. The code-orchestration executor substituted only numeric path parameters, leaving string-typed ones (e.g. `{assetType}`, `{entityType}`) as literal template tokens in the request URL - which broke `tag.set-for-asset`, `tag.batch-update`, and the `custom-fields` endpoints that carry a string path segment. String path parameters are now resolved and URL-escaped like numeric ones. Reported by @livewire-it (#186).

### Changed
- Bumped the Go toolchain to go1.26.5 (clears the `crypto/tls` standard-library advisory GO-2026-5856).

## [0.1.5] - 2026-07-07

### Changed

- fix(ninjaone): keep query object cursor paging advancing

## [0.1.4] - 2026-06-18

### Changed

- fix(ninjaone): store the queries-custom-fields* report variants (#137) (#142)

## [0.1.3]

### Fixed
- `sync` now stores correct rows for the device-scoped `/v2/queries/*` report
  endpoints. Every row in this family is keyed by its `deviceId` first, so rows no
  longer collapse. This fixes two distinct symptoms:
  - The endpoints the reporter hit - `queries-antivirus-status`,
    `queries-device-health`, `queries-network-interfaces`,
    `queries-logged-on-users`, `queries-policy-overrides`,
    `queries-raid-controllers`, `queries-raid-drives` - return objects with no
    top-level `id`, so they cached **zero** rows (`all_items_failed_id_extraction`).
  - A second, broader set silently dropped rows **across devices** because the
    generic key fell back to a patch `id` or a `name`: `queries-os-patches`,
    `queries-os-patch-installs`, `queries-software-patches`,
    `queries-software-patch-installs`, `queries-software`,
    `queries-operating-systems`, `queries-computer-systems`, `queries-processors`,
    `queries-volumes`, `queries-windows-services`, `queries-antivirus-threats`
    (e.g. every device running "Windows 11" collapsed to one stored row).
  Each row now keys on `deviceId` plus a per-row discriminator where the resource
  returns many rows per device (`productName`, `threatId`, `interfaceIndex`,
  `userName`, `controllerIndex`, `driveId`, the patch `id`, or `name`); single-row
  reports key on `deviceId` alone. Offline SQL, full-text `search`, and rollups
  like `av-sweep`, `software-audit`, `patch-compliance`, and `os-eol` now see the
  full per-device data. (The four `queries-custom-fields*` report variants are
  tracked separately - their row shape is not yet confirmed, see #137.)
- `sync` now follows the **object-valued** pagination cursor these same
  `/v2/queries/*` endpoints return (`"cursor": {"name": "<token>", …}`). The loop
  previously read only a string cursor, so it stopped after the first page and
  capped each resource at 100 rows (`pagination_cursor_missing`); it now follows
  the `cursor.name` token. String-cursor endpoints are unaffected.
  Thanks to @Xenith-B for the report (#136).

## [0.1.2]

### Fixed
- `sync` now fetches **every** page for NinjaOne's after-id (keyset) endpoints
  (`/v2/devices`, `/v2/organizations`, `/v2/locations`, …). These return a bare
  JSON array and paginate via `?after=<lastEntityId>` with no envelope cursor;
  the loop previously stopped at the first full page and reported a truncated
  dataset (e.g. 1,000 of 1,115 devices) as complete, and `--max-pages 0` had no
  effect. The loop now follows the after-id cursor to completion. Envelope-cursor
  endpoints (`/v2/queries/*`) are unaffected.
  Thanks to @AndrewITLive for the report (#88).

### Changed
- Regenerated on the printing-press 4.24.0 engine: robust numeric-ID handling
  and dependency security updates. Same commands and workflows, sturdier local
  mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `ninjaone-cli` CLI and `ninjaone-mcp` MCP server
  for the NinjaOne RMM public API, with OAuth2 client-credentials auth.
- Offline SQLite mirror (`sync`, incremental + resumable) with FTS5 full-text
  `search` so fleet-wide reads run with zero API calls.
- Fleet-wide rollups no single API call returns: `patch-compliance`,
  `backup-coverage`, `av-sweep`, `fleet-health`, `stale-devices`, `os-eol`,
  `software-audit`, and week-over-week `drift`.
- Agent-native output across every command: `--agent`/`--json`/`--select`/`--csv`,
  typed exit codes, and an `agent-context` introspection surface.
