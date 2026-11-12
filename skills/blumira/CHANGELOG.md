# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.11] - 2026-11-11

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.1.10] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.9] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.8] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.7] - 2026-11-07

### Security
- **Rebuilt on Printing Press 4.35.1 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)).** The MCP server now has a native HTTP mode: it listens on `127.0.0.1:7777` by default, refuses to start until `BLUMIRA_MCP_HTTP_TOKEN` is set (at least 32 characters), requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert` and `--tls-key` for any address other than loopback. Remote agents (ChatGPT, Copilot Studio, the Gemini app) are supported again through a hosted HTTPS endpoint; see [mcp-install.md](./mcp-install.md).
- **Every MCP write previews first.** The six Blumira write tools (resolve a finding, set owners, add a comment and the rest) return a preview and send nothing until they are called again with `confirm` set to boolean true.
- `auth login` sends the client secret only to the token endpoint: it refuses every redirect and refuses a plaintext `--auth-url` other than loopback.

### Added
- `BLUMIRA_MCP_READ_ONLY` (preset to `1` in the Claude Desktop bundle and the install configs) makes the MCP server refuse every write. `BLUMIRA_NO_LOCAL_DATA=1` keeps no local copy of API responses. Both are declared on every install channel.

### Fixed
- **`sync` reads every page.** Blumira returns 50 rows a page unless asked for more, and `sync` treated that first page as the whole list, so a partition over 50 rows (findings, devices, rules, accounts) was cut at 50 and recorded as complete. `sync` now asks for 200 rows a page, follows the next-page link and the total counts Blumira returns, and records a walk as complete only when it reached the end.
- **Over-time reports see every sync.** A sync that returned no findings was not recorded, so `drift` missed the first finding that appeared after it; `sync` also records a history point for every findings walk, so `drift`, `velocity`, `audit` and `recurring` no longer depend on an analytics command running between syncs.
- **Device reports cover MSP sub-accounts.** `sync` now reads each MSP sub-account's device list, and `exposure`, `dc-roster` and `overview` name every account whose devices could not be read completely instead of reporting it as healthy.
- **Owners are read from Blumira's role-keyed `owners` object**, unknown ownership is no longer reported as unassigned, and two owners who share a name stay two people in `workload` and `reconcile`.
- **`evidence-search` reads every evidence page**, matches the search term literally (a `%` or `_` is not a wildcard), and applies `--limit` to findings after merging evidence and finding matches, so a finding found in both places no longer hides another match.
- `velocity` reads a finding's latest state, so a resolution that was later reopened no longer counts as current.
- `doctor` probes the real Blumira host and checks the credential instead of reporting every install as an unset placeholder.

## [0.1.6] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.5] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.4] - 2026-10-15

### Changed
- **Rebuilt with Go 1.26.9 (was 1.26.6).** Go 1.26.9 includes upstream Go security
  fixes, among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and
  GO-2026-6612. The connector's own code is unchanged.
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but this connector's previous `.mcpb` contained only the MCP
  server, so a one-click install on a machine without the CLI failed at the first tool
  call. This release's bundle ships both binaries side by side for macOS (universal),
  Windows x64 and Linux x64.
- **The MCP server reports its real version.** `serverInfo.version` read `0.0.0-dev` in
  the previous release because the release build stamped the version into the CLI
  only. The release now stamps both and checks the built server's `initialize` reply
  against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256
  sidecars before touching anything, and replace the old binaries in a single
  transaction that is undone if any step fails. (Served from the repository, so this
  already applies to every install, not only this version.)

## [0.1.3] - 2026-09-10

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
