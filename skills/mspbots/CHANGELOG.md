# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.8] - unreleased

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.7] - 2026-11-05

Reprinted on cli-printing-press **4.34.2** (from 4.24.0). Every command, flag and MCP tool that shipped in 0.1.6 is still here; the list below is what changed for an operator or an agent. The press upgrade follows advisory GHSA-jh26-vhqh-692c.

### Security
- **HTTP mode for remote agents is available again.** `mspbots-mcp --transport http` refuses to start until `MSPBOTS_MCP_HTTP_TOKEN` is set, answers 401 to any request without that token as a bearer header, serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert` and `--tls-key` for any address other than loopback. The install guide documents this mode for reference; msp-skills does not support a web-only AI app on it yet.
- **Every output is checked for credentials.** Stdout, stderr, files the CLI writes (an `export --out` file included), its local database, MCP tool results and child processes all pass one shared check: text that repeats a credential the program holds, whole, as a piece of 8 or more characters, or in a decoded form, is withheld whole and replaced by a notice that says so. The check can withhold ordinary text it cannot finish reading; it says so the same way, so rephrase and retry.
- **A credential is held wherever it comes from.** Before the command line is parsed, the CLI and the MCP server read every credential source they can reach: `MSPBOTS_API_KEY`, a user name, password or query value written into `MSPBOTS_BASE_URL`, every config file `--config`, `--home` or `MSPBOTS_CONFIG` names (a file that does not parse is read token by token), saved profiles and the saved credentials. A value is withheld however short it is.
- **A credential is only sent to your configured MSPbots API, over https.** A request that carries a credential is refused if it goes to any other host (a redirect included) or over plain `http://`. A `--deliver` webhook whose URL carries a `token=` query value is refused unless it points at the API base URL.
- **Errors printed before a command runs pass the check too,** such as a bad flag value or an unknown command.
- Updated `golang.org/x/text` to v0.42.0 and `mark3labs/mcp-go` to v0.57.0.

### Added
- `MSPBOTS_MCP_READ_ONLY=1` exposes only read-only MCP tools and refuses any state-changing call; `MSPBOTS_NO_LOCAL_DATA=1` keeps no local copy of API responses and refuses `snapshot`. Both are optional and declared in the Claude Desktop extension, the MCP Registry entry and the install guide, beside `MSPBOTS_MCP_HTTP_TOKEN`, `MSPBOTS_USER_AGENT` and `PRINTING_PRESS_CLIENT_PROFILE`.

### Fixed
- **`doctor` checks the API key against the real MSPbots API** (`https://api.mspbots.ai`) instead of reporting the default host as a placeholder, and says so when no key is configured.
- Commands that take an alias (`pull`, `export`, `snapshot`, `describe`, `diff`, `trend`) say "not found" and name what is missing when the alias is not registered or has too few snapshots.

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
  authenticate. Now declared on every install channel: `MSPBOTS_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `mspbots-cli` + `mspbots-mcp` covering the MSPbots
  Public API (dataset and widget reads) as typed subcommands and MCP tools - the
  first published tool we could find for this API (June 2026).
- Local alias registry (`registry add` / `list` / `rm`): name a 19-digit resource
  ID once, use the alias in every other command - the discovery surface the API
  itself doesn't ship.
- Readable filter compiler: `pull --where "Update Date >= 2026-06-01"` compiles
  human-readable predicates into the API's comma-encoded operator DSL (ranges,
  on-or-after, contains, is-empty).
- KPI history the platform doesn't keep: `snapshot` captures timestamped copies
  into local SQLite; `trend` aggregates a numeric column across snapshots
  (sum/avg/count/min/max with deltas); `diff` reports row-level
  added/removed/changed between two snapshots.
- `describe`: samples live rows and infers column names, types, null rates, and
  example values - the metadata endpoint the API lacks.
- Full-table `export` to CSV or JSONL with automatic pagination and an honest
  partial-dump flag when `--max-pages` is hit.
- Agent ergonomics: `--agent` mode (JSON, non-interactive), `--select`/`--compact`
  field control, `profile` saved flag sets, `doctor` health check, offline `sync`
  store with `--data-source` control.
