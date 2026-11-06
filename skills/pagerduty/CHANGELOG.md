# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.8] - unreleased

### Security
- **The CLI and the MCP server run on the shared credential kit.** Any output, file, local-store write, MCP result, child process or request that would repeat a credential this program holds, in any encoding the kit reads, is replaced whole by `[credkit: withheld from <sink>: it carried a credential this program holds]`. That now covers a key nested in a compressed stream with bytes around it. A program that lost part of its standard output exits 74, and a key that also occurs in ordinary text is refused at startup with exit 78.

### Changed
- **Rebuilt on CLI Printing Press 4.34.2 (was 4.24.0).** See [GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c) for why the press was upgraded.
- **The MCP server has a token-protected HTTP mode for remote agents.** `pagerduty-mcp --transport http` refuses to start until `PAGERDUTY_MCP_HTTP_TOKEN` (32 characters or more) is set, rejects any request without that bearer token, serves `http://127.0.0.1:7777/mcp` by default and needs `--tls-cert` and `--tls-key` for any other address.
- **Every MCP tool that changes PagerDuty data previews first.** It returns what it would send and sends nothing until the call carries `confirm: true`.
- New install settings on every channel: `PAGERDUTY_MCP_READ_ONLY`, `PAGERDUTY_NO_LOCAL_DATA`, `PAGERDUTY_MCP_HTTP_TOKEN`, `PAGERDUTY_USER_AGENT` and `PRINTING_PRESS_CLIENT_PROFILE`.
- Six more commands for `external_data` cache variables on event orchestrations.

### Fixed
- **`sync` now fills the tables the offline commands read.** `oncall who`, `oncall hours`, `pulse`, `insights`, `audit` and `incidents timeline` answered "nothing synced" after a default sync, because no rows were stored for services, escalation policies, schedules, on-calls, incidents, log entries or change events.
- A complete `sync` removes on-call entries PagerDuty no longer returns for the current window, and keeps history and future shifts. A sync that was narrowed, paged by hand (`--param offset` or `limit`), resumed after a failure or cut short removes nothing.
- `oncall who` scopes resolve to IDs once: every scope flag narrows the result together, an ID wins over a name that reads like one, a name shared by several things answers for all of them, and every simultaneous next responder is listed.
- `oncall hours` counts each hour of a user's overlapping shifts once.
- `incidents timeline` shows what each entry says happened: the note, event details, the changes in a custom field update, the assignees and the notified user.
- `export` and `--all` read every page.
- A pasted `Token token=<key>` is sent as is instead of being wrapped twice.

## [0.1.7] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.6] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.5] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `pagerduty-cli business-services unsubscribe create` ran `pagerduty-cli business-services unsubscribe remove-business-service-notification-subscriber`, POST `/business_services/{id}/unsubscribe`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `pagerduty-cli business-services unsubscribe remove-business-service-notification-subscriber` (POST `/business_services/{id}/unsubscribe`)
  - `pagerduty-cli event-orchestrations integrations migrate-orchestration` (POST `/event_orchestrations/{id}/integrations/migration`)
  - `pagerduty-cli incident-workflows instances create-incident-workflow` (POST `/incident_workflows/{id}/instances`; starts an incident workflow, running its automated actions)

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
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `PAGERDUTY_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `pagerduty-cli` CLI + `pagerduty-mcp` MCP server
  for PagerDuty incident response and on-call management.
- Offline SQLite mirror (`sync`) with FTS5 full-text `search` across synced data.
- Live triage: `pulse` buckets open incidents by service and SLA risk.
- On-call intelligence: `oncall who` (now / next / handoff) and `oncall hours`.
- Post-incident analytics computed offline: `insights mttr` (MTTA/MTTR),
  `insights responders` (workload + off-hours load), `insights noisy`,
  `insights stale`.
- Coverage audits: `audit coverage` (broken escalation chains, single points of
  failure) and `audit schedule-gaps` (future windows with nobody on call).
- Incident forensics: `incidents timeline` and `incidents changes` (what shipped
  right before an incident broke).
- Full PagerDuty REST surface (incidents, services, schedules, escalation
  policies, event orchestrations, and more) with `--agent` JSON mode and
  `--dry-run` previews for writes.
