# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.12] - unreleased

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.1.11] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.10] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.9] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.8] - 2026-11-07

### Security
- **Rebuilt on Printing Press 4.35.1 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)).** The MCP server's HTTP mode now listens on `127.0.0.1:7777` by default, refuses to start until `TACTICAL_RMM_MCP_HTTP_TOKEN` is set (at least 32 characters), requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert` and `--tls-key` for any address other than loopback. HTTP mode is documented in [mcp-install.md](./mcp-install.md) for reference; web-only AI apps (ChatGPT on the web, Copilot Studio, the Gemini app) are not supported by msp-skills yet.
- **One credential guard for every output.** The CLI and the MCP server now run on the shared msp-skills credential kit. Your Tactical RMM API key (from `TRMM_API_KEY`, the config file or the saved credential store) is known before anything prints, and any output, file, local database write or MCP result that would carry it, in any common encoding, is replaced by a short notice instead. The key is sent only to the Tactical RMM server you configured, never over plain `http`, and never to another site (a `--deliver` webhook URL that carries it is refused). This replaces the earlier error-only filter from 0.1.7.
- **Agent actions now ask first.** The Tactical RMM endpoints that act on agents rather than read them (bulk agent recovery, clearing the server cache, and the ping, process, event log, registry and service lookups that send a command to the endpoint) are treated as actions: `sync`, `export`, `tail` and other local reads never call them, and over MCP they run only with `confirm: true`. The MCP tools that mirror `agents bulk-run` and `maintenance set` preview first and send only with `confirm: true`.

### Added
- `TACTICAL_RMM_MCP_READ_ONLY=1` exposes only read tools and refuses state-changing calls; `TACTICAL_RMM_NO_LOCAL_DATA=1` keeps no local copy of API responses. Both are optional and declared on every install channel, as is `TACTICAL_RMM_USER_AGENT`.
- `--all` on `agents bulk-run` and `maintenance set`: with no `--filter`, the command now needs `--all` to target every synced agent.

### Fixed
- **Alert reports read your alerts, not your alert templates.** `sync` now stores the Tactical RMM alert list, and fleet health, the alerts digest, the client scorecard and `since` count active alerts (not resolved, not snoozed). In fleet health JSON, `alerts.by_severity` now counts active alerts only.
- **Cohort filters fail closed.** `agents bulk-run` and `maintenance set` refuse an unknown or malformed `--filter` entry instead of ignoring it and targeting a wider cohort.
- **Client inventory is complete.** The client scorecard lists every synced client, and fleet health counts sites from the synced site list.
- **`checks worst` reports the checks that are failing.** Tactical RMM keeps a check's status on each agent, not in the check list `sync` stores, so `checks worst` always printed an empty list. It now reads each failing agent's checks live (`GET /agents/{agent_id}/checks/`, capped by `--max-scan-agents`) and prints an object: `items` (each check with `agents_failing` and the `agents`), plus `scanned_agents` and `fetch_failures`. `total_checks` is gone. `checks flapping --snapshot` reads the same live status, so flips are now detected.
- **Sites are told apart by client.** Site names repeat across clients. `patch posture --by site` now keeps same-named sites in different clients as separate rows with a `client` field, and `agents bulk-run` and `maintenance set` refuse a `--filter site=` name used by more than one client unless `client=` picks one.
- **A saved profile never shows a password in its base URL.** `profile show` printed a password or query token saved inside a profile's `base-url`; that value is now withheld like any other credential.
- **A new Spacefast page key is never printed.** `--deliver spacefast:` printed the private page link, which carries the page key; the output now names the private delivery record that keeps the link.

## [0.1.7] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.6] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.5] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `tactical-rmm-cli agents reboot create` ran `tactical-rmm-cli agents reboot agents-agents`, POST `/agents/{agent_id}/reboot/`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `tactical-rmm-cli agents meshcentral agents-agents` (POST `/agents/{agent_id}/meshcentral/`; recovers (reinstalls) the MeshCentral agent on the endpoint)
  - `tactical-rmm-cli agents reboot agents-agents` (POST `/agents/{agent_id}/reboot/`)
  - `tactical-rmm-cli agents shutdown agents` (POST `/agents/{agent_id}/shutdown/`)
  - `tactical-rmm-cli alerts alerts-bulk` (POST `/alerts/bulk/`; bulk_action resolve or snooze on a list of alerts (Tactical RMM alerts/views.py BulkAlerts))
  - `tactical-rmm-cli autotasks run autotasks` (POST `/autotasks/{pk}/run/`; runs an automated task on its agents now)
  - `tactical-rmm-cli checks reset checks` (POST `/checks/{pk}/reset/`)
  - `tactical-rmm-cli checks resetall checks` (POST `/checks/{agent_id}/resetall/`)
  - `tactical-rmm-cli scripts test scripts` (POST `/scripts/{agent_id}/test/`; runs a script on an agent)
  - `tactical-rmm-cli services services-agentid-2` (POST `/services/{agent_id}/{svcname}/`; starts, stops or restarts a Windows service on an agent)
  - `tactical-rmm-cli software software-agentid-2` (POST `/software/{agent_id}/`; installs software on an agent)
  - `tactical-rmm-cli software uninstall software` (POST `/software/{agent_id}/uninstall/`)
  - `tactical-rmm-cli winupdate install winupdate` (POST `/winupdate/{agent_id}/install/`)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

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
  It also dialled the shipped placeholder base URL (`https://api.example.com`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `TACTICAL_RMM_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `TACTICAL_RMM_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `tactical-rmm-cli` + `tactical-rmm-mcp` for self-hosted
  Tactical RMM, with an offline SQLite mirror and full-text search.
- Cross-entity fleet views the web UI can't assemble: `fleet health`, `triage`,
  `patch posture`, `clients scorecard`, `coverage`, `since`, `checks worst`/`flapping`,
  `alerts digest`, `services down`, `software find`.
- Preview-first cohort actions: `agents bulk-run` and `maintenance set` resolve and
  print the target cohort and run only with `--execute`.
- Typed commands across the full Tactical RMM API (agents, clients, sites, checks,
  alerts, scripts, automation, autotasks, software, services, winupdate, accounts,
  core) with `--agent` JSON output, `--dry-run`, profiles, and `--deliver` sinks.
