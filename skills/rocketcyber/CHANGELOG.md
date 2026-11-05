# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.9] - unreleased

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.8] - 2026-11-05

### Security
- **Rebuilt on CLI Printing Press 4.34.2** (the security update in [GHSA-jh26-vhqh-692c](https://github.com/mvanhorn/cli-printing-press/security/advisories/GHSA-jh26-vhqh-692c)). HTTP mode (`rocketcyber-mcp --transport http`) refuses to start until `ROCKETCYBER_MCP_HTTP_TOKEN` is set to at least 32 characters, every request must send it as `Authorization: Bearer <token>`, it serves `http://127.0.0.1:7777/mcp` by default, any other address also needs `--tls-cert` and `--tls-key`, and a request from a web page on another origin is refused. The "do not use HTTP mode" note is removed.
- **Two new operator switches, declared on every install channel.** `ROCKETCYBER_MCP_READ_ONLY=1` (preset in the Claude Desktop bundle) keeps the MCP server to read-only tools; `ROCKETCYBER_NO_LOCAL_DATA=1` keeps no local copy of API responses.
- **Credentials are held by the shared credential kit** (`docs/credential-leak-contract.md`) from the first instruction of each binary: the API token, the MCP HTTP token and every value in the config and credentials files never reach standard output or error, an MCP result, a file, the local store or a request to another host, in any of the encodings the kit reads: what would carry one is replaced whole by `[credkit: withheld from <sink>: it carried a credential this program holds]`. The token is sent only to the configured RocketCyber API host over HTTPS.
- **New exit codes.** A command whose standard output was partly lost exits 74 instead of 0; a token that also occurs in ordinary text (for example one repeated letter) is refused at startup with exit 78. `rocketcyber-mcp` refuses `--config` and `--home`; set `ROCKETCYBER_CONFIG` or `ROCKETCYBER_HOME` in its environment instead.

### Removed
- **`import`.** The RocketCyber Customer API is read-only, so the command could only fail; no command writes to RocketCyber.

### Fixed
- **`export` writes every page**, to a file or to standard output. It wrote the first 100 agents, firewalls or incidents as if that were the complete backup.
- **`reports` prints the CSV report RocketCyber returns** instead of refusing it as not JSON.
- **A default `sync` no longer walks every detection app's event history.** It syncs agents, apps, firewalls and incidents; `sync --resources suppression` stores one row per suppression rule.
- **`doctor` checks the real RocketCyber host and the token** instead of calling every default install an unconfigured placeholder.
- **Novel commands never read a failed or missing answer as nothing found.** `triage` reports a failed source as a fetch failure and fails when every source fails; `defender riskiest`, `office trend` and `suppression audit` refuse an answer missing the data they rank; `triage` and `suppression audit` say when the API counted more rows than it answered; `agents stale` and `incidents mttr` read every synced row.

## [0.1.7] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

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
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `ROCKETCYBER_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `rocketcyber-cli` + `rocketcyber-mcp` for the
  RocketCyber Customer API v3 (incidents, agents, detection events, Defender and
  Microsoft 365 posture, firewalls, suppression rules, CSV report export).
- Local SQLite mirror with full-text `search` and incremental `sync` for offline,
  multi-account rollups.
- SOC analytics the console doesn't compute: `triage` (cross-account board),
  `agents stale`, `incidents mttr`, `defender riskiest`, `office trend`, and
  `suppression audit`.
- `--agent` JSON output for AI-agent use; `import` with `--dry-run` for the only
  write path.
