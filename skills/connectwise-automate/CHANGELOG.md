# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.8] - unreleased

### Security
- **Rebuilt on press 4.35.1.** The MCP server's HTTP mode (`--transport http`, for remote agents) now serves `127.0.0.1:7777` by default, refuses to start until `CONNECTWISE_AUTOMATE_MCP_HTTP_TOKEN` is set (at least 32 characters), rejects any request without that bearer token, and needs TLS for any address other than loopback. [mcp-install.md](./mcp-install.md) documents it for reference; web-only AI apps are not supported yet.
- **MCP tools that change your Automate server ask first.** Running a command on a computer, deploying or re-attempting patches, and `import` return a preview and run only when called with `confirm: true`. `apitoken_mint` and `apitoken_refresh` send nothing over MCP and name the CLI command instead: minting a token is human-in-the-loop only.
- **One credential layer, shared across the fleet.** The CLI and the MCP server now run on the shared credential kit. Your token, clientId, the MCP HTTP token and every credential in config.toml or the credential store are held from start-up, and any output that would carry one (terminal, `--json`, MCP results, logs, the local store, the learn journal, files the program writes) is withheld whole. A credential goes only to the configured server over https, never across a redirect to another host. This replaces the connector's own masking layer.
- **`apitoken mint` and `apitoken refresh` save the new token instead of printing it.** It goes to the credential store `auth set-token` uses, later commands use it (an exported `CONNECTWISE_AUTOMATE_TOKEN` still wins), and the output and the local store show where it was saved.
- **`CONNECTWISE_AUTOMATE_CLIENT_ID` stays in your environment.** `auth set-token` no longer copies it into config.toml.
- **The configured server is the only host that gets your credentials.** The client keeps the base URL it resolved at start-up and refuses a request aimed anywhere else, and `sync --path-context` can no longer set the server.

### Fixed
- **Fleet reports count the fleet the API last returned in full.** `sync` now records which records each complete, unfiltered sync returned. `fleet-health`, `client-rollup`, `alert-triage` and the other reports read only those, so a sync resumed after `--max-pages` keeps its first page, a record fetched by a later `get` is not counted as current, and a sync filtered with `--param` or `--resource-param` is never taken for the whole fleet. A report over a mirror no complete sync proves fails with the command that finishes it (or reports anyway with `--allow-partial-failure`).
- **Two clients with the same name stay two clients.** Every report groups by the client's Id and shows `Name (#Id)` when names repeat, including when computers were synced without their clients. A Closed alert is not counted as open.
- Windows 10 editions other than LTSC are flagged end-of-life in `os-inventory`.

### Added
- `agent-versions`: computers grouped by Automate agent version, newest first, with per-client counts; `--behind-only` lists the agents left behind.

### Changed
- `server info` is now `server get` (`server info` still works). The MCP tool is `server_get`.
- The local mirror's `monitors` resource now holds `/MonitorStatistics`, and sensor checks moved to `monitors-sensor-checks`. Rows a 0.1.7 sync stored under `monitors` are sensor checks until you run `sync --full --resources monitors`.

## [0.1.7] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.
- **Credentials no longer follow a redirect to another server.** If the API answered with a redirect to a different host or port, or from https down to http, the connector forwarded the ClientId header to it. It now removes every credential header on such a redirect, and refuses to resend a request body (which can carry a secret) anywhere but the server you configured.

### Changed
- `cli/NOTICE` now names the product `connectwise-automate-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

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
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  It also dialled the shipped placeholder base URL (`https://{server}/cwa/api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `CONNECTWISE_AUTOMATE_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `CONNECTWISE_AUTOMATE_BASE_URL`.

## [0.1.0]

### Added
- Initial msp-skills release: ConnectWise Automate CLI + MCP server with a local
  SQLite mirror for offline, cross-client queries.
- Cross-client roll-ups the per-server console can't do: `fleet-health`,
  `stale-agents`, `patch-compliance`, `client-rollup`.
- Triage and inventory: `alert-triage`, `os-inventory` (EOL flagging), and `since`
  for overnight drift.
- Full Automate API coverage: computers, clients, locations, alerts, patching,
  scripts, monitors, groups, contacts, and network devices.
- Endpoint and fleet actions (`computers command-execute`, `patching deploy-*`) and
  token minting (`apitoken`), gated as human-in-the-loop in governance.md.
