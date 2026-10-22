# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.9] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.8] - 2026-10-15

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

## [0.1.7] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.6] - 2026-09-28

Reprinted on cli-printing-press **4.32.5** (from 4.24.0). The list below is what changed
for an operator or an agent; one command was removed and one renamed, both named here.

### Changed
- **Engine 4.32.5.** Runtime and MCP layers regenerated on the current press. The
  0.1.5 fix that joins every MCP tool-call value to its flag as one word (`--flag=value`)
  is now generated code (press #4647), and the MCP server refuses argument names carrying
  `=` and the local-store destination flags at the tool schema as well as at run time.
- **`auth set-token` is now `auth set-credentials <public_key> <secret_key>`.** The old
  command took one value and saved only the public key, half of the Basic credential the
  Datto BCDR API needs. The new one saves both, to `credentials.toml` in the data
  directory. The `DATTO_BCDR_PUBLIC_KEY` / `DATTO_BCDR_SECRET_KEY` environment variables
  work as before.
- **`import` is removed.** It POSTed each record to `/<resource>`, and the Datto BCDR API
  has no write endpoints, so it could only fail. Nothing else was removed.
- **`--agent` no longer implies `--yes`.** `--agent` still sets `--json --compact
  --no-input --no-color`; a command that asks for confirmation now needs an explicit
  `--yes`.
- **Local store schema 4 to 11, and one mirror per credential.** The SQLite mirror is
  migrated forward on first open; a 0.1.5 binary cannot read a mirror this release has
  migrated. On a machine with no mirror yet, `sync` with credentials writes
  `data-<hash>.db`, keyed to the credential, so two Datto accounts no longer share one
  mirror; an existing `data.db` keeps being used as it is.
- **New commands and MCP tools:** `export` (reads the API with GET only and writes
  JSONL or JSON; over MCP it prints to the tool result, and `--output` is refused), and the
  local learning loop (`teach`, `teach-lookup`, `teach-pattern`, `teach-playbook`,
  `recall`, `learnings ...`, `playbook list` / `playbook amend`), which reads and writes
  a local store only and never contacts the Datto BCDR API. The no-op `workflow` parent tool, which only returned its
  own help text, is gone; `workflow_archive` and `workflow_status` are unchanged.
- **MCP `--transport http` is authenticated and loopback by default.** It requires
  `DATTO_BCDR_MCP_HTTP_TOKEN` (callers send `Authorization: Bearer <token>`), binds
  `127.0.0.1:7777` by default, and refuses a plaintext non-loopback bind such as
  `--addr :7777` unless `--tls-cert` and `--tls-key` are given. The listener sets a
  request-header read deadline. `mcp-install.md`'s remote recipe is updated to match.
- **Three new optional install prompts** on every channel (`.mcpb`, MCP Registry,
  `mcp-install.md`): `DATTO_BCDR_MCP_HTTP_TOKEN`, `DATTO_BCDR_USER_AGENT` (User-Agent
  override) and `PRINTING_PRESS_CLIENT_PROFILE`.
- **Release artifacts:** the `.mcpb` bundle carries the companion `datto-bcdr-cli`, and
  `datto-bcdr-mcp` reports its real version (`0.1.6`) instead of `0.0.0-dev`.

### Fixed
- **MCP `search` and `sql` read the mirror `sync` wrote.** On 4.32.5 the generated MCP
  store path still pointed at `data.db` while `sync` wrote the credential-scoped file, so
  after a fresh sync both tools answered "No local data store found". They now use the
  same resolver as the CLI, pinned by a test and recorded in the hand-fix ledger.
- **`doctor` probes the credential on the default API URL.** 0.1.5 treated the shipped
  default `https://api.datto.com/v1`, which is the real Datto API, as an unset
  placeholder and refused to verify credentials against it. Template, `YOUR_` and
  reserved-domain placeholders are still refused.

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
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `DATTO_BCDR_BASE_URL`.

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
- Initial msp-skills release: Datto BCDR CLI + MCP server with a local SQLite mirror
  of every device, agent, share, and alert.
- Fleet-wide recovery-assurance commands the per-appliance Partner Portal can't answer:
  `screenshots` (backup-bootability audit), `recoverability` (fresh + screenshot-verified
  KPI), and `stale-backups` (local/offsite snapshot recency).
- Cross-client triage and reporting: `client-risk`, `alert-triage`, `storage-runway`,
  `forgotten-assets`, `agent-versions`, and the QBR-ready `client-report`.
- Offline mirror with full-text `search`, `analytics`, and resumable incremental `sync`;
  agent-native output via `--agent` / `--json` / `--select` and a `doctor` health check.
