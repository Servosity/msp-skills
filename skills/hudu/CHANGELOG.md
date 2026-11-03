# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.17] - 2026-10-27

Rebuilt on CLI Printing Press 4.34.0 (the press upgrade that carries the fix for [GHSA-jh26-vhqh-692c](https://github.com/mvanhorn/cli-printing-press/security/advisories/GHSA-jh26-vhqh-692c)).

### Security
- **HTTP mode needs a token.** `hudu-mcp --transport http` refuses to start until `HUDU_MCP_HTTP_TOKEN` is set, every request must send it as `Authorization: Bearer <token>`, the server serves `http://127.0.0.1:7777/mcp` by default, and any other address also needs `--tls-cert` and `--tls-key`. The interim advice not to use HTTP mode is removed from the docs; remote agents (ChatGPT, Copilot Studio, the Gemini app) can be wired up again through a hosted HTTPS endpoint.
- **The MCP server is split into a read tool and an execute tool.** A state-changing call through `hudu_execute` sends nothing until the call also passes `confirm: true`.
- **`onboard` refuses to run from the MCP server.** It reads a local template file (`--template`) or writes one (`--init`), so a call through the MCP server returns a refusal and touches no file; run `hudu-cli onboard` from a terminal.
- **Password-vault secrets and password fields are withheld.** Every response from Hudu's password vault (`asset-passwords` list, get, create and update) keeps only a fixed set of metadata fields (name, username, URL, folder, dates); the password, the OTP secret and any other field are dropped. In asset responses, the value of every field whose asset layout type is Password, Confidential Text or not a known plain type is replaced by `[withheld: password field]` (an empty field stays empty, so audits still count filled fields). This is decided by the resource, including the address Hudu redirected to, before the CLI prints, the MCP server answers, `sync` stores or the response cache keeps anything. Asset field values are checked wherever they sit in the response, and each asset layout's field types are read again for every response, so a field changed to Password partway through a `sync` is withheld on the next page. The response cache never serves an entry written by an older version, and never keeps a vault or asset response, including one Hudu redirected to. Fetch a secret in the Hudu portal.
- **Error text from vault and asset requests is replaced.** When Hudu answers a password-vault or asset request with an error, or with any other status outside 2xx, the CLI and the MCP server show the HTTP status and a fixed note, never the body Hudu sent, whatever its format. Exit codes are unchanged.
- **An upgraded local store is scrubbed on first open.** The first time this version opens a store written by an older one (including one an older version left without a schema stamp), it removes stored vault passwords, OTP secrets and password field values from the mirror and its search index, in rows of every resource type (store schema version 14; older binaries then refuse the store, so upgrade rather than downgrade). A read-only open, as MCP `sql` and `search` use, that cannot confirm the scrub refuses the store instead of reading it; retry once the other process is done. No manual purge is needed.
- **The withheld placeholder is never sent back.** A request whose body contains `[withheld: password field]` is refused, so editing an asset you read with this CLI cannot overwrite the real value in Hudu.
- **New operator switches**, declared in the Claude Desktop bundle, the MCP Registry entry and every config in mcp-install.md: `HUDU_MCP_READ_ONLY=1` exposes only read-only tools and makes the execute tool refuse writes; `HUDU_NO_LOCAL_DATA=1` keeps no local copy of API responses; `HUDU_MCP_HTTP_TOKEN` (above).

### Changed
- **`--agent` no longer implies `--yes`.** A command that asks for confirmation now needs `--yes` passed on its own.

### Fixed
- Search finds short all-caps values such as `WEB`, `SQL` and `DNS` again ([#464](https://github.com/Servosity/msp-skills/issues/464)). The local search index rebuilds once, automatically, the first time the CLI or the MCP server opens the store after upgrading; nothing needs to be run by hand.
- **`onboard --apply` writes only to the company you name.** Template folders are always created under `--company`, even when the template file carries a different `company_id`.
- **`onboard --init --dry-run` writes nothing.** It reports where the example template would go.
- **`onboard --init` keeps an edited template.** When the template file already exists it stops with an error; pass `--yes` to replace it with the example.
- **`audit completeness`, `audit layout-drift` and `audit summary` read the asset values Hudu returns.** Hudu returns an asset's values under `fields`; the audits read only `custom_fields` (the write shape), so a filled asset scored 0%. They now read both.
- **`audit stale-passwords` no longer always says the store was never synced.** It looked up the wrong sync record name.
- **`sync` of `matchers` without an integration id** sends no request and reports a `missing_required_params` warning naming `integration_id`; pass `--resource-param matchers:integration_id=<id>` to sync it.
- **`sync` stops cleanly on a repeated page.** When Hudu returns the page it just returned (the `procedure-tasks` endpoint ignores `page` on some deployments), the walk ends complete; a repeat of an older page ends it as incomplete with reason `repeated_page`, so nothing is stored twice.

## [0.1.16] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.15] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.14] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.13] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.12] - 2026-10-02

### Fixed

- Expose executor request schemas in MCP endpoint discovery, including required inputs, wire-key case, nested bodies, and known enum values. Search now finds request fields and exact short enum values (#351).
- Preserve separate path, query, header, and body inputs, including endpoints that reuse a name in more than one location. Existing CLI commands and custom workflows are retained.

## [0.1.11] - 2026-09-28

### Changed
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but the `.mcpb` used to contain only the MCP server, so a one-click
  install on a machine without the CLI failed at the first tool call. The bundle now ships
  both binaries side by side for macOS (universal), Windows x64 and Linux x64. Older
  bundles are unchanged; this release's bundle is the first for this connector to include it.
- **The MCP server reports its real version.** `serverInfo.version` used to read
  `0.0.0-dev` (or a hard-coded literal) in every released MCP binary because the release
  build stamped the version into the CLI only. The release now stamps both and checks the
  built server's `initialize` reply against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256 sidecars
  before touching anything, and replace the old binaries in a single transaction that is
  undone if any step fails. (Served from the repository, so this applies to every install
  from now on, not only this version.)
- No connector code changed in this release; it exists so the published bundle and
  binaries pick up the release-pipeline fixes above.

## [0.1.10] - 2026-09-10

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

## [0.1.9] - 2026-09-01

### Added
- **`sync --phase-trace` breaks each resource's wall time into phases.** Emits a `sync_phases`
  event per resource splitting the time into queue wait, API fetch, rate-limit wait, retry
  wait, ID extraction and SQLite upsert, so a slow lightweight resource can be attributed
  rather than guessed at. Durations and counts only - no credentials, no response bodies.
  `queue_wait_ms` covers enqueue until a worker picks the resource up, which the previous
  per-resource duration started after and therefore could never measure.

### Changed
- Install and remote-agent documentation corrected against the shipped binaries. The remote
  section named `mcp-remote`, which bridges the opposite direction and cannot publish a local
  stdio server at all; connectors that parse `--transport http` now point at the native flag
  and the rest at `supergateway`. The HTTP endpoint is `/mcp`, not the bare root the docs gave.
  The Windows install path and a fallback paragraph describing an `npx` install that is not
  offered were both wrong and are gone. A new `check_install_docs` gate holds these claims
  against the binaries and installers so they cannot drift again.

## [0.1.8] - 2026-08-26

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

## [0.1.7] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  It also dialled the shipped placeholder base URL (`https://your-domain.huducloud.com/api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `HUDU_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

## [0.1.6] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.5] - 2026-07-17

### Changed

- fix(hudu): reconcile procedure task sync totals (#194)
- chore(fleet): bump Go toolchain to go1.26.5 across 54 connectors (GO-2026-5856) (#189)

## [0.1.4] - 2026-07-07

### Changed

- fix(hudu): relations page_size=25 + sync --exclude (#167, #168, #169) (#181)

## [0.1.3] - 2026-06-26

### Changed

- fix(hudu): IPAM endpoints are non-paginated, enable the page walk, skip matchers (#158, #159) (#161)

## [0.1.2] - 2026-06-22

### Changed

- fix(hudu): stop sending page_size to IPAM endpoints that reject it (#153) (#156)
- fleet: re-vendor 48 connectors to printing-press 4.24.0 engine (#96)
- chore(fleet): press-version provenance + back-fill hand-fix ledgers (#91)
- fix: drop false "with sound" demo claim, highlight first-party Servosity, point backup/DR skills at Servosity (#77)

## [0.1.0]

### Added
- Initial msp-skills release: `hudu-cli` + `hudu-mcp` (123 MCP tools) for the Hudu
  IT-documentation API.
- Offline SQLite mirror (`sync`) with FTS5 full-text `search` over every synced
  resource, plus `--data-source auto|live|local`.
- Documentation-hygiene audits over the mirror: `audit completeness`,
  `audit stale-passwords`, `audit expirations`, `audit stale-articles`,
  `audit layout-drift`, and a worst-first cross-tenant `audit summary`.
- `onboard` to scaffold a new client's asset layouts, folders, and procedures from
  a saved house template (preview by default, `--apply` to write).
- `resolve` a Hudu URL or exact name to its asset/company/layout/relations, and
  `reconcile` PSA/RMM integrator records against live Hudu assets.
- Agent-native output (`--agent`, `--json`, `--compact`, `--select`) for use from
  Claude Code, Codex, and any MCP-capable agent.
