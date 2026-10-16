# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.2] - 2026-10-15

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

## [0.2.1] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.2.0] - 2026-09-28

Reprinted on cli-printing-press **4.32.5** (from 4.24.0). Every command, flag and
MCP tool that shipped in 0.1.6 is still here except one no-op parent tool (below);
the list is what changed for an operator or an agent.

### Fixed
- **`doctor` told every HubSpot operator their install was broken.** It treated
  `https://api.hubapi.com`, HubSpot's one real API root, as an unset placeholder and
  answered `FAIL API: base_url is still the shipped placeholder; set HUBSPOT_BASE_URL`
  without ever checking the token. It now dials the real root and verifies the token
  against a live read (`OK Credentials: valid (verified with GET /crm/v3/objects/calls)`),
  and a rejected token reports the HTTP 401. `HUBSPOT_BASE_URL` stays optional.

### Changed
- **Engine 4.32.5.** Runtime and MCP layers regenerated on the current press. The
  0.1.6 security fix (every MCP tool-call value joined to its flag as one word,
  `--flag=value`, press #4647) is now generated code, and the MCP server refuses the
  local-store destination flags (`--db`, `--output`, `--audit-dir`, ...) at the tool
  schema as well as at run time.
- **`sync --full` now prunes** local rows HubSpot no longer returns, after a complete
  walk (`--no-prune` turns it off). A walk narrowed by `--param`, `--resource-param` or
  `--global-param` never prunes, so a filtered sync cannot delete rows outside its filter.
- **Local store schema 4 -> 11.** The first run of 0.2.0 migrates the local database in
  place; a 0.1.x binary cannot open the migrated file afterwards (it reports the schema
  is newer than it supports). Re-run `sync` if you ever go back to 0.1.x.
- **`--agent` no longer implies `--yes`.** `--agent` still sets `--json --compact
  --no-input --no-color`; a command that asks for confirmation now needs an explicit
  `--yes`. Scripts that relied on `--agent` to auto-confirm a write must add `--yes`.
- **New commands:** `teach`, `teach-lookup`, `teach-pattern`, `teach-playbook`, `recall`,
  `learnings` (`list`, `candidates`, `confirm`, `reject`, `forget`, `purge`, `stats`),
  `playbook` (`list`, `amend`) and `export`, the press's local learning loop; all but
  `learnings purge` are also MCP tools, plus a generic `hubspot_get` read tool. `--no-learn` turns the loop off.
- **New global flags:** `--receipt` / `--receipt-file` / `--audit-dir` write a private run
  receipt; `--client-profile` selects a client profile; `--home` relocates config, data,
  state; `--rate-limit` now defaults to `auto` (paces to HubSpot's rate-limit headers).
- **MCP over HTTP:** `hubspot-mcp --transport http` serves Streamable HTTP on
  `127.0.0.1:7777` and requires `Authorization: Bearer <HUBSPOT_MCP_HTTP_TOKEN>`. The
  no-op `workflow` parent tool, which only returned its own help text, is gone;
  `workflow_archive` and `workflow_status` are unchanged.
- **Three new optional install prompts** on every channel (`.mcpb`, MCP Registry,
  `mcp-install.md`): `HUBSPOT_MCP_HTTP_TOKEN` (only for `--transport http`),
  `HUBSPOT_USER_AGENT` (overrides the request User-Agent) and
  `PRINTING_PRESS_CLIENT_PROFILE`. `HUBSPOT_ACCESS_TOKEN`, `HUBSPOT_BASE_URL` and
  `HUBSPOT_OWNER_EMAIL` are unchanged.
- **Release artifacts:** first hubspot release cut after the 2026-09-18 pipeline change,
  so the `.mcpb` bundle carries the companion `hubspot-cli` and `hubspot-mcp` reports its
  real version (`0.2.0`) instead of `0.0.0-dev`.

## [0.1.6] - 2026-09-10

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

## [0.1.5] - 2026-08-26

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

## [0.1.4] - 2026-08-26

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
  authenticate. Now declared on every install channel: `HUBSPOT_BASE_URL`, `HUBSPOT_OWNER_EMAIL`.

## [0.1.3] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.2] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.1] - 2026-06-06

### Changed

- skill: hubspot - UPDATE to 4.22.0 reprint (zero-review pipeline) (#37)
- fix(install): honor GITHUB_TOKEN/GH_TOKEN in fetch_stdout across all skills (#31)
- feat(surfaces): generate every skill-enumerating surface; media on GitHub; Servosity live-verified (#21)

## [0.1.0]

### Added
- Initial msp-skills release: the HubSpot CLI and MCP server for the terminal and
  any MCP-capable agent.
- Offline SQLite mirror with full-text search - sync your CRM once, then run
  reads against local data with zero API calls.
- Pipeline analytics: `pipeline-health` (per-stage count, dollars, and $ at
  risk), `owner-load` (open-deal load per rep per stage), and `deals top`
  (composite-ranked top-N deals).
- Stale-detection and nurture queues: `stale deals` / `stale contacts`,
  `nurture queue`, and `nurture-mine` for the daily who-to-call list.
- Cross-object engagement timelines via `engagements of` (calls, emails,
  meetings, notes, and tasks for any contact, deal, or company).
- Property-history reporting: `sync --with-history` snapshots, plus
  `meetings ever-had` and `meetings status-report` for "was ever in state X"
  monthly reports.
- Agent output modes: `--agent`, `--json`, `--compact`, `--csv`, and `--dry-run`
  for safe, scriptable, low-token automation.
