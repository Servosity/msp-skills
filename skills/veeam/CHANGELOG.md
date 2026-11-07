# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.8] - 2026-11-07

### Security
- **Rebuilt on press 4.35.1.** The MCP server's HTTP mode (`--transport http`, for remote agents) now serves `127.0.0.1:7777` by default, refuses to start until `VEEAM_MCP_HTTP_TOKEN` is set (at least 32 characters), rejects any request without that bearer token, refuses browser origins other than loopback, and needs TLS for any address other than loopback. stdio stays the default, and msp-skills does not support a web-only AI app on HTTP mode yet.
- **Credentials are held by the shared msp-skills credential kit.** `VEEAM_TOKEN`, a token in `config.toml` or the credential store, `SPACEFAST_TOKEN`, `VEEAM_MCP_HTTP_TOKEN`, a credential saved in a run profile and the password fields of a `--stdin` body or a JSON-valued flag are held from startup. Any output that would carry one, whole, in pieces or encoded (terminal, MCP results, files, the local store, logs, the learn journal and the feedback log), is withheld, and a held credential is never sent to a host other than the configured Veeam Service Provider Console or over plain http.
- **Secrets Veeam returns stay out of output and the local store.** The credential and password fields of reads such as `infrastructure get-*-credentials-by-server`, `get-*-encryption-passwords-by-server`, `authentication generate-totp-secret` and the guest OS credential, SQL account and user login lists are held, so the output that carries one is replaced by a withheld notice and `sync` reports those rows as withheld instead of storing them.
- `VEEAM_MCP_READ_ONLY`, `VEEAM_NO_LOCAL_DATA` and `VEEAM_MCP_HTTP_TOKEN` can now be set from every install channel (Claude Desktop extension, MCP registry, manual config).

### Fixed
- **The cross-tenant reports count what Veeam returned in full.** `fleet-health`, `stale-backups`, `at-risk`, `alarms-triage`, `license-usage`, `company-overview` and `since` now read only the records the last complete, unfiltered `sync` returned. A sync narrowed with `--param`, `--resource-param` or `--global-param`, a record fetched by a live read, and a job or alarm Veeam no longer returns are not counted. A report over a list no complete sync proves fails and names the `sync` command that fixes it.
- **The reports read Veeam's documented fields.** Active alarms come from the active-alarm list, not the alarm templates; an agent counts as online only when its status is `Active`; per-OS agent lists no longer count each agent twice; license points add up `usedUnits` where the unit is `Points` (the reports showed 0 before); a backup job belongs to the company it is assigned to, not the server's owner; Resolved alarms are not active; and two alarms on different objects that share a name stay two problems.
- Two jobs or agents with the same name in different companies stay separate records in the local mirror.

### Changed
- On 16 create commands (backup policies, discovery rules, backup agent job configurations, companies, resellers, SAML identity providers, Microsoft 365 organizations and subscription plans), deeply nested settings are now one JSON flag instead of many long flags, for example `--deployment-settings-backup-agent-settings '{"disableScheduledBackups":true}'`. The request sent to Veeam is unchanged.
- A plain `sync` now also fetches active alarms, license usage per organization, restore points and discovery rules, so it makes more calls than before.
- `alarms-triage` shows the latest activation and repeat count; it no longer claims a first-seen time.
- After upgrading, run `veeam-cli sync` once before the reports: a mirror synced by an older version is not proven whole and the reports refuse it.

## [0.1.7] - 2026-10-23

### Changed
- `cli/NOTICE` now names the product `veeam-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.6] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.
- The MCP server no longer opens a local file named by a binary multipart (upload) field and sends it to the vendor; run such an upload with the CLI.

## [0.1.5] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `veeam-cli infrastructure create` ran `veeam-cli infrastructure abort-backup-server-multipart-patch`, POST `/infrastructure/backupServers/{backupServerUid}/patch/upload/multipart/abort`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `veeam-cli infrastructure abort-backup-server-multipart-patch` (POST `/infrastructure/backupServers/{backupServerUid}/patch/upload/multipart/abort`)
  - `veeam-cli licensing finalize-reports` (POST `/licensing/reports/finalization`)
  - `veeam-cli plugins enabled enable-disable-plugin` (POST `/plugins/{pluginId}/enabled`)

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
  It also dialled the shipped placeholder base URL (`https://your-vspc-host:1280/api/v3`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `VEEAM_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

## [0.1.0]

### Added
- Initial msp-skills release: Veeam Service Provider Console (VSPC) v3 CLI + MCP
  server with a local multi-tenant SQLite mirror for offline, cross-company queries.
- Cross-tenant rollups: `fleet-health`, `stale-backups`, `company-overview`,
  `at-risk` (RPO), `alarms-triage`, `license-usage`, and `since` (fleet drift).
- Full VSPC v3 surface (~1000 commands): companies, backup servers, jobs, agents,
  protected workloads, alarms, discovery, infrastructure, licensing, and billing.
- Per-instance `VEEAM_BASE_URL` + bearer `VEEAM_TOKEN` auth for appliance-hosted
  consoles; write/infrastructure/destructive/credential tiers gated in governance.md.
