# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.5] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.2.4] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `sentinelone-cli accounts expire-now create` ran `sentinelone-cli accounts expire-now expire-an-account`, POST `/accounts/{account_id}/expire-now`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `sentinelone-cli accounts expire-now expire-an-account` (POST `/accounts/{account_id}/expire-now`)
  - `sentinelone-cli agents abort-scan` (POST `/agents/actions/abort-scan`)
  - `sentinelone-cli dv cancel-running-query` (POST `/dv/cancel-query`)
  - `sentinelone-cli firewall-control item-move-rules by-category` (POST `/firewall-control/{firewall_rule_category}/move-rules`)
  - `sentinelone-cli firewall-control remove-tags remove-rule-tags-by-category` (POST `/firewall-control/{firewall_rule_category}/remove-tags`)
  - `sentinelone-cli remote-scripts run` (POST `/remote-scripts/execute`)
  - `sentinelone-cli reports delete` (POST `/reports/delete-reports`)
  - `sentinelone-cli settings clear-pending-emails` (POST `/settings/notifications/cancel-pending-emails`)
  - `sentinelone-cli singularity-marketplace enable-or-disable-application` (POST `/singularity-marketplace/applications/{mode}`)
  - `sentinelone-cli sites expire-now expire-site` (POST `/sites/{site_id}/expire-now`)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.2.3] - 2026-10-15

### Fixed
- **Windows: the built-in Administrator account can save credentials.** On the built-in
  local Administrator (common on MSP servers and jump boxes), saving credentials failed
  with `token file DACL grants access to a non-owner principal (LA)`, and reads refused
  the saved file. The token-file permission check compared security-descriptor text
  instead of account SIDs; it now resolves each trustee to a SID first. The allowed
  principals are unchanged: your account, BUILTIN\Administrators and SYSTEM.
- **Windows: a token file with a NULL DACL is now refused.** A NULL DACL (no access
  control at all, so any account on the machine can read the file) written with its
  flags first, which is how Windows Server 2025 renders it, passed the check. The check
  now parses the DACL exactly and refuses anything it cannot parse.

## [0.2.2] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.2.1] - 2026-09-28

0.2.0 was tagged but never published: the release probe refused its MCP binary, which reported `serverInfo.version` 0.0.0-dev because the reprint dropped `var version` from the MCP entry point. That is restored (and recorded in `handfixes.json`), and 0.2.1 is the first release of this reprint.

### Changed
- **Reprinted on cli-printing-press 4.32.5** (was 4.24.0). Every generated file under `cli/` was regenerated on the current engine and the connector's recorded hand-fixes were re-applied on top (`handfixes.json`; all entries pass). The SentinelOne analysis commands (`threats triage`, `threats mttr`, `threats verdicts`, `threats recurrence`, `threats blast-radius`, `fleet-health`, `coverage-gaps`, `exclusions-audit`, `posture`, `ranger-exposure`, `sites-risk`, `versions rollout`, `whatchanged`, `agents dossier`) are unchanged.
- **Breaking: three firewall-control commands moved.** On the old engine `firewall-control copy-rules`, `move-rules` and `set-location` were groups whose only subcommand was `by-category`, and the plain endpoints could not be reached. Now `firewall-control copy-rules` / `move-rules` / `set-location` call the plain endpoints, and the by-category operations are `firewall-control item-copy-rules by-category`, `item-move-rules by-category` and `item-set-location by-category`. The old spelling is refused with a message naming the new command, and nothing is sent. MCP tool names are unchanged.
- **Two fleet security fixes are now engine-native instead of hand-carried.** MCP tool-call values travel glued to their flag (`--flag=value`, 0.1.5) and the MCP server refuses filesystem-destination arguments (`--db`, `--output`, ..., 0.1.4); both are emitted by the 4.32.5 engine. The connector still carries the wider destination floor, the usage-text rule that fails the build when a regeneration grows a new local-path flag, and its tests.
- **Local store schema bump (4 to 11).** The first command that opens the local SQLite mirror migrates it in place: new tables for the learning loop and playbooks, a sync-completion marker, and a rebuilt search index (substring search now also matches CJK text). The migration is one-way: once upgraded, a 0.1.x binary refuses the file ("database schema version 11 is newer than supported version 4"). To go back to 0.1.x, delete the mirror and run `sync` again.
- **`--agent` no longer implies `--yes`.** It still sets `--json --compact --no-input --no-color`; pass `--yes` explicitly when a command asks for confirmation.
- **Companion CLI in the bundle and a real MCP version.** The `.mcpb` ships `sentinelone-cli` next to `sentinelone-mcp`, and `initialize` reports this release's version.

### Added
- **Remote transport.** `sentinelone-mcp --transport http` serves MCP over streamable HTTP behind `Authorization: Bearer <SENTINELONE_MCP_HTTP_TOKEN>`, binds loopback (`127.0.0.1:7777`) by default and refuses a plaintext bind on a non-loopback address unless `--tls-cert` / `--tls-key` are given. mcp-install.md describes the tunnel setup.
- **Self-learning loop** (engine feature): `recall`, `teach`, `teach-pattern`, `teach-lookup`, `teach-playbook`, `learnings` (`list`, `candidates`, `confirm`, `reject`, `forget`, `purge`, `stats`) and `playbook` (`list`, `amend`). Everything it records stays in the local SQLite store; nothing is sent to SentinelOne or anywhere else. Opt out with `--no-learn` or `SENTINELONE_NO_LEARN=true`.
- **Install prompts** now also declare the three optional variables the 4.32.5 binaries read: `SENTINELONE_MCP_HTTP_TOKEN` (bearer token for `--transport http`), `SENTINELONE_USER_AGENT` (User-Agent override; leave blank) and `PRINTING_PRESS_CLIENT_PROFILE` (only relevant to a tenant-gated profile; leave blank).

### Fixed
- **MCP HTTP listener sets a header-read deadline** (`ReadHeaderTimeout` 10s), so a client that opens a connection and never finishes its headers cannot pin the `--transport http` listener. Recorded in `handfixes.json`.
- **`doctor` info rows.** The engine now skips informational keys natively when evaluating `--fail-on`; the connector's placeholder refusal and real credential probe are re-applied and gated by `check_doctor_truth.py`.
- **`users login-by-token --token` help** shows `<your-user-token>` again instead of the vendor spec's token-shaped sample value, which the reprint had restored.
- **Auth header.** SentinelOne requires `Authorization: ApiToken <token>`; the engine still emits the bare token, so the prefix is re-applied (and a token pasted with the prefix is not doubled). This was a silent hand-fix since the first print and is now recorded in `handfixes.json`.

## [0.1.5] - 2026-09-10

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

## [0.1.4] - 2026-08-26

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

## [0.1.3] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  It also dialled the shipped placeholder base URL (`https://your-console.sentinelone.net/web/api/v2.1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `SENTINELONE_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `SENTINELONE_BASE_URL`.

## [0.1.2] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: the `sentinelone-cli` CLI and `sentinelone-mcp`
  MCP server for the SentinelOne v2.1 Management API.
- Offline SQLite mirror with full-text `search`, incremental `sync`, and a
  per-sync history snapshot that powers the time-aware analytics.
- Cross-site threat analytics: `threats triage` (ranked worklist),
  `threats blast-radius` (endpoint-joined containment), `threats recurrence`
  (unkilled root causes), `threats mttr` (SLA breaches), and
  `threats verdicts --changed` (verdict/incident flips between syncs).
- Fleet and coverage views: `fleet-health summary` / `fleet-health stale`,
  `coverage gaps`, `versions rollout`, `ranger exposure`, `agents dossier`,
  `exclusions audit`, `sites risk`, and the per-tenant `posture` scorecard.
- `whatchanged` overnight drift report diffing the fleet against an earlier
  snapshot (new threats, agents offline, version and protection-mode changes).
- Agent-first ergonomics: `--agent` JSON mode, `--dry-run` previews,
  `--data-source` (auto/live/local), `--rate-limit`, profiles, and `doctor`.
