# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.10] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.9] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.8] - 2026-11-05

### Security
- **Rebuilt on CLI Printing Press 4.34.2** (the press upgrade behind [GHSA-jh26-vhqh-692c](https://github.com/mvanhorn/cli-printing-press/security/advisories/GHSA-jh26-vhqh-692c)). HTTP mode refuses to start without `LIONGARD_MCP_HTTP_TOKEN`, checks it on every request, and needs `--tls-cert` and `--tls-key` for any address other than loopback; every MCP write previews and sends only with `confirm: true`.
- **One shared credential filter.** The CLI and the MCP server now use the fleet's shared credential kit: every credential the program holds (environment, config, credential store, saved profiles, flags, piped `--stdin` bodies, credentials Liongard returns) is withheld from every output, file, local database and MCP result in any encoding, and is sent only to your Liongard instance over https. A run with no instance set (`LIONGARD_INSTANCE` still the `example` placeholder) sends no credential at all.
- The learn journal no longer stores the `LIONGARD_LEARN_SESSION` value or flag names the command does not declare.

### Changed
- **`metrics` and `inspectors` sync again**, so `metrics pivot` and `inspectors coverage` answer from the local copy. `metrics breach` refuses until every covered system has an evaluated value stored, because synced metric definitions carry no values (Liongard evaluates them per system: `liongard-cli metrics evaluation`).
- **Commands that read the local copy refuse an incomplete one** (a sync that never finished, or one narrowed by `--param`, `--resource-param`, `--since` or `--max-pages`) and name the exact `sync --full --resources <name>` command to run, instead of answering from partial data.

## [0.1.7] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.6] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.5] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given this command the hidden alias `create`, so typing `create` in its place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `liongard-cli agents flush create` ran `liongard-cli agents flush job-queue`, POST `/agents/{AgentID}/flush`. The alias is removed; the command still runs under its own name. If a script reached it through `create`, call it by the name below:
  - `liongard-cli agents flush job-queue` (POST `/agents/{AgentID}/flush`)

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

- **`LIONGARD_ACCESS_KEY_ID` and `LIONGARD_ACCESS_KEY_SECRET` were ignored, so every call went out
  unauthenticated.** Liongard hands you an Access Key ID and an Access Key Secret and never shows
  the encoded form the API actually wants, and the README, the SKILL and the guide all tell you to
  export those two variables. The config loader read neither: it only ever looked for a
  pre-encoded `LIONGARD_API_KEY`, so an operator who followed the shipped instructions got an empty
  `X-ROAR-API-KEY` header and a 401 on every command, with nothing in `doctor` naming the cause.
  The loader composes the pair into base64 of `accessKeyId:accessKeySecret` again, as it did when
  the connector first shipped; a pre-encoded `LIONGARD_API_KEY` still wins when both are set.
  Both variables are now declared on all three install channels, so Claude Desktop asks for them
  and the copy-paste MCP config blocks carry them. `LIONGARD_API_KEY` is no longer marked required
  in `manifest.json` or `server.json` either: while it was, the MCPB bundle and the registry
  install both forced a value into it, and because the pre-encoded key wins the ID/secret pair
  stayed dead through every official install no matter what the docs said. All three fields now
  state the one-of contract - set the key or set the pair, never both - and the install pages show
  the two setups separately instead of one line that sets all three at once.

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
  It also dialled the shipped placeholder base URL (`https://{instance}.app.liongard.com/api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `LIONGARD_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `LIONGARD_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `liongard-cli` and the `liongard-mcp` MCP server,
  covering the full Liongard API surface (environments, systems, launchpoints,
  agents, inspectors, metrics, detections, timeline, users, access keys).
- Offline local SQLite mirror via `sync`, with FTS5 full-text `search` and
  `analytics` over your whole estate.
- Cross-estate rollups that no single API call returns: `drift` (change feed
  joined to environment and system), `health` (one estate scorecard with a typed
  exit code), `launchpoints stale`, `agents offline`, `detections failures`,
  `coverage`, and `inspectors coverage`.
- Reporting helpers: `metrics pivot` (one metric across every system, CSV-ready)
  and `metrics breach` (systems crossing a numeric threshold).
- Per-environment `environments overview` and per-system `systems history` views.
- `--agent` JSON mode on every command, plus `doctor` for auth/connectivity
  checks and `tail` for live change polling.
