# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.7] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.6] - 2026-09-28

Reprinted on cli-printing-press **4.32.5** (from 4.24.0). Every command, flag and
MCP tool that shipped in 0.1.5 is still here except the two named below; the list is
what changed for an operator or an agent.

### Upgrade warning
- **The local store upgrades one way.** The first run of 0.1.6 migrates the local
  SQLite store from schema 4 to schema 11. After that, a 0.1.5 or older binary refuses
  to open it ("database schema version 11 is newer than supported version 4"). If you
  may need to roll back, copy the data directory's `data.db` aside before the first run,
  or re-run `sync` after downgrading.

### Fixed
- **`doctor` never checked your credentials on a default install.** It treated the real
  Huntress API root (`https://api.huntress.io`, the only host there is) as an unset
  placeholder, so an operator who had not set `HUNTRESS_BASE_URL` was told the base URL
  was still the shipped placeholder and the credential was never probed. `doctor` now
  probes the default host, and the credential check bypasses the response cache, so a
  key revoked on the Huntress side can no longer read as valid from a cached answer.
- **MCP `search` and `sql` now find the data `sync` wrote on a fresh install.** 4.32.5
  names a new local store after a hash of the configured credential, but the generated
  MCP tools still looked for the old `data.db`. On a new install they reported no data
  even after a successful sync. They now resolve the store exactly as the CLI does; an
  existing `data.db` is still used as before.

### Changed
- **Engine 4.32.5.** Runtime and MCP layers regenerated on the current press. The 0.1.5
  security fix (every MCP tool-call value joined to its flag as one word, `--flag=value`)
  is now generated code rather than a hand-fix that had to survive a reprint.
- **`auth set-token` is replaced by `auth set-credentials <api_key> <api_secret>`**, which
  saves both halves of the Basic credential to an owner-only (0600) credentials file in
  the CLI's data directory. Environment variables keep working unchanged.
- **`--agent` no longer implies `--yes`.** It still sets `--json --compact --no-input
  --no-color`; a command that asks for confirmation now needs an explicit `--yes`.
- **New global flags:** `--receipt` / `--receipt-file` / `--audit-dir` write a private run
  receipt, `--client-profile` selects a tenant-gated client profile, `--home` relocates
  config and data, `--no-learn` turns off the local learning journal, and `--rate-limit`
  now defaults to `auto` (paces to the server's rate-limit headers).
- **New commands:** `export`, `recall`, `teach` / `teach-lookup` / `teach-pattern` /
  `teach-playbook`, `learnings` and `playbook`. The API resource groups (`agents`,
  `organizations`, `incident-reports`, `signals`, ...) are now listed in `--help`; they
  existed before but were hidden.
- **MCP tool surface:** 26 -> 40 tools. Added `export`, `huntress_get`, `recall`, the
  `teach*`, `learnings_*` and `playbook_*` tools. The `workflow` parent tool, which only
  returned its own help text, is gone; the CLI `workflow` command is unchanged.
- **Remote MCP recipe:** `huntress-mcp --transport http` now binds `127.0.0.1:7777` by
  default and refuses a non-loopback `--addr` without `--tls-cert`/`--tls-key`, so the
  documented launch line drops `--addr :7777`; put the loopback listener behind your
  HTTPS tunnel as before.
- **Two new optional install prompts** on every channel (`.mcpb`, MCP Registry,
  `mcp-install.md`): `HUNTRESS_MCP_HTTP_TOKEN` (bearer token, only for `--transport http`)
  and `PRINTING_PRESS_CLIENT_PROFILE`, plus `HUNTRESS_USER_AGENT`. The key, secret and base
  URL prompts are unchanged.
- **Release artifacts:** first huntress release cut after the 2026-09-18 pipeline change,
  so the `.mcpb` bundle carries the companion `huntress-cli` and `huntress-mcp` reports its
  real version (`0.1.6`) instead of `0.0.0-dev`.

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
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `HUNTRESS_BASE_URL`.

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
- Initial msp-skills release: `huntress-cli` and the `huntress-mcp` MCP server,
  covering the full Huntress API - accounts, organizations, agents, incident
  reports, remediations, signals, escalations, identities, external recon,
  reports, invoices, reseller subscriptions, and SIEM ES|QL.
- Offline SQLite mirror with `sync` and FTS5 `search` for instant, repeatable
  queries that cost zero API calls.
- Cross-tenant rollups the per-org API and portal can't return: `fleet-incidents`,
  `fleet-summary`, `coverage-gaps`, `blast-radius`, `billing-reconcile`,
  `triage-age`, `org-scorecard`, and `reseller-rollup`.
- History the point-in-time API throws away: `drift`, `mttr`, `handoff`, and
  `canary-watch`.
- Agent-native output everywhere: `--json`, `--select`, `--agent`, `--dry-run`,
  and typed exit codes.
