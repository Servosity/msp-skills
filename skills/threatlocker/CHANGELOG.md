# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.3.13] - unreleased

### Security
- **Rebuilt on Printing Press 4.34.0** ([GHSA-jh26-vhqh-692c](https://github.com/Servosity/msp-skills/security/advisories/GHSA-jh26-vhqh-692c)). The MCP server splits reads from writes: `threatlocker_read` calls read-only endpoints (including ThreatLocker's POST searches), and `threatlocker_execute` returns a preview of a state-changing call until the agent passes `confirm: true`. `threatlocker-mcp --transport http` refuses to start without `THREATLOCKER_MCP_HTTP_TOKEN`, answers only requests that send `Authorization: Bearer <token>`, serves `http://127.0.0.1:7777/mcp` by default and needs `--tls-cert`/`--tls-key` for any other address. `THREATLOCKER_MCP_READ_ONLY=1` lists only read-only tools.
- `organizations auth-key` and `computers install-info` return a tenant's agent installation key, so they are now credential-tier: the CLI prints the key to the terminal that asked, never caches or stores it (the local store drops its key columns, and a store an earlier version wrote is cleaned before anything, the MCP `sql`, `search` and `export` tools included, reads it; if it cannot be cleaned, reading it is refused), and the MCP server never returns it (`threatlocker_read`, `threatlocker_execute` and `threatlocker_get` refuse both, and no mirrored tool is listed).
- With `THREATLOCKER_NO_LOCAL_DATA=1` the CLI and the MCP server save no API data locally: `sync` and `audit pull` are refused, the response cache is off, and reads go to the API.

### Changed
- **`approvals approve-batch` now needs `--yes` for a live run**, and a tenant: `--org`, `THREATLOCKER_ORG_ID`, or `--all-tenants`. Without them it refuses and sends nothing; `--dry-run` shows the (tenant, request) pairs it would approve. Scripts that ran it without `--yes` must add the flag. Before each approval the live run re-checks the request with ThreatLocker and approves only requests still pending for that file, so a request someone approved or ignored since the last `sync` is skipped. Over MCP the tool can only preview. A `--json` run exits non-zero when any approval in the batch fails.
- `audit retention-check` counts only complete, gap-free `audit pull` windows as coverage, and reports `unverified` for a tenant whose rows came from no complete pull.

### Added
- `audit pull --since <window>`: pages the Unified Audit (ActionLog) deny events (`--action permit` or `any-deny` for others) and the System Audit for one time window into the local store, keyed so a re-pull never duplicates rows. `audit export`, `audit drift`, `audit retention-check` and `organizations posture` read what it saves; before it, nothing filled those tables.
- `maintenance open`: endpoints whose maintenance-mode window is open right now, across tenants, longest-open first, flagged overdue past `--older-than`.
- `organizations posture`: one row per tenant (endpoints, online, isolated, pending approvals, open maintenance windows, newest audit row), sorted by what needs a look first.
- `local-data purge` (from the 4.34.0 engine).
- `THREATLOCKER_MCP_READ_ONLY`, `THREATLOCKER_NO_LOCAL_DATA` and `THREATLOCKER_MCP_HTTP_TOKEN` are declared on every install channel (Claude Desktop bundle, MCP Registry, manual config).

### Fixed
- `applications hunt --org <tenant>` lists only that tenant's approval requests, like every other local read. Application rules are not stored per tenant, so they are listed either way.
- `sync` stores application files in the table `applications hunt` reads, and pages online devices, application files and maintenance history instead of stopping after the first page.
- `doctor` probes the credential with a command that needs no input, so a healthy install is no longer reported as having the wrong base URL, and it treats the shipped base URL as a real host.
- The help for `devices health`, `applications hunt` and the audit commands names the command that fills their table (`sync` or `audit pull`), and an endpoint counts as online only when its online-device row is as fresh as its computer row.
- `maintenance open` and `organizations posture` count each maintenance window once, using its newest version.

## [0.3.12] - 2026-10-23

### Changed
- `cli/NOTICE` now names the product `threatlocker-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

## [0.3.11] - 2026-10-22

### Security
- The MCP server drops the flags whose values name a file or directory on the machine that runs it (for example --db, --output) from its tool schemas and refuses them by name, instead of advertising them and ignoring them silently. The command line keeps every flag.

## [0.3.10] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.3.9] - 2026-10-15

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

## [0.3.8] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.3.7] - 2026-10-02

### Fixed

- Expose executor request schemas in MCP endpoint discovery, including required inputs, wire-key case, nested bodies, and known enum values. Search now finds request fields and exact short enum values (#351).
- Preserve separate path, query, header, and body inputs, including endpoints that reuse a name in more than one location. Existing CLI commands and custom workflows are retained.

## [0.3.6] - 2026-09-28

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

## [0.3.5] - 2026-09-10

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

## [0.3.4] - 2026-09-06

### Fixed
- **`recall` and `playbook list` told your MCP host they were read-only tools. They were not.**
  Both were annotated `mcp:read-only=true` while both open the *writable* learn store on every
  call: `recall` inserts a learn event, `playbook list` appends an audit record. The annotation
  is what an MCP host reads to decide what it may auto-approve without asking you, so the false
  claim was the defect - a host was waving through a write it had been told could not happen.
  Both now carry `mcp:local-write=true`, the tier this connector already used for `teach` and
  `playbook amend`: the write lands only in this CLI's own local store, never in external state
  and never in a file you would go looking at. Measured at the running MCP server, before and
  after: `recall` and `playbook_list` went from `readOnlyHint=true, openWorldHint=true` to
  `readOnlyHint=false, destructiveHint=false, openWorldHint=false`.

  **The writes themselves are unchanged.** This is a truth fix, not a behaviour fix - both
  commands still write exactly what they wrote before, and nothing you can call does anything
  new. If you had approved these tools on the strength of the read-only annotation, that is the
  decision worth revisiting. See issue #275 finding 4.

- **`learnings list`, `learnings candidates` and `learnings stats` keep `mcp:read-only`, and
  `AGENTS.md` now says what that does and does not promise.** Each still opens the learn store
  read-write and creates the database file when it is absent, but none of them records a
  learning row, so the annotation stands. It is narrower than it sounds, and the prose no longer
  implies otherwise.

### Changed
- **Install and remote-MCP instructions now describe the artifacts this repository actually
  ships.** The guide routed installs through the printing-press library (`npx -y
  @mvanhorn/printing-press-library install threatlocker`, and a `go install` from that repo's
  module path) - neither publishes these binaries. It now points at `skills/threatlocker/
  install.sh` / `install.ps1`, and says plainly that the installer downloads binaries and does
  *not* register the skill with your agent or write any MCP client config; `mcp-install.md`
  covers that separately.
- **The ChatGPT answer no longer sends you to a bridge package.** `threatlocker-mcp` speaks HTTP
  natively: `threatlocker-mcp --transport http --addr :7777` behind an HTTPS tunnel or your own
  reverse proxy. The old text named `mcp-remote`, which bridges the other direction.
- **`doctor` exits 0 even when the credential is rejected**, so the guide now tells scripts to
  add `--fail-on error` instead of testing the exit code alone.
- **Interim `.mcpb` note.** The Claude Desktop bundle's `manifest.json` launches a binary name
  the archive may not contain (issue #287). The download section now tells you to check with
  `unzip -l <file>.mcpb | grep bin/` and to fall back to the shell installer if it does not
  match.

### Documentation
- **The upstream citations on this connector's two hand-fixes now record what upstream actually
  did, instead of pointing at a dead issue.** They cited `mvanhorn/cli-printing-press#4165`,
  which had been closed as completed with both of these findings explicitly out of scope - so
  the code sent readers to an issue that would never move. They were re-pointed at `#4482`,
  which upstream then closed on 2026-09-01 by merging `#4489` (first shipped in press v4.31.5),
  fixing both: `doctor` now skips read commands that need input, and the sync profiler binds a
  child collection keyed by a required query parameter. Both citations say so, and neither
  claims a tracker is still open.

  Nothing in the shipped binary changes. This connector is still generated on press 4.30.2, so
  both hand-fixes are still doing their work; what changed is the instruction left for the next
  reprint, which now says *check* rather than *assume* - and says what to check. Comments,
  `handfixes.json` follow-ups and `reprint-patches.py` only.

## [0.3.3] - 2026-08-26

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

## [0.3.2] - 2026-08-26

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
  authenticate. Now declared on every install channel: `THREATLOCKER_API_KEY`, `THREATLOCKER_BASE_URL`, `THREATLOCKER_ORG_ID`, `THREATLOCKER_USER_AGENT`.

## [0.3.1] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

### Fixed

- **MCP tools are no longer default-denied.** Every tool that is not a Cobra
  mirror - `search`, `sql`, `context`, and the `threatlocker_search` / `threatlocker_get` /
  `threatlocker_execute` code-orchestration trio - returned
  `MCP tenant gate is not configured` instead of running. The generated tenant
  gate treated "no platform source registered" as a failure rather than as
  "nothing to gate", and no connector registers one. The previously released
  binary had 6 dead tools. See issue #249.

## [0.3.0] - 2026-08-14

### Changed

- fix(threatlocker): sync the last four resources, and stop doctor recommending a command that cannot run (#227)

## [0.2.0]

### Fixed
- `sync` stored zero rows for every resource while still reporting success and exiting 0,
  which left the local mirror empty and every offline-backed feature with nothing to read
  (`devices health`, `search`, `analytics`, and any `--data-source local` command).
  Two compounding causes: `sync` was pointed at the ThreatLocker portal's dropdown helper
  endpoints instead of the real list endpoints, and no primary-key field was configured, so
  every row failed id extraction. `sync` now walks the real paginated list endpoints and
  keys rows on their actual id fields. Reported with live-tenant measurements by
  @geekbrownbear in #208.
- `doctor` reported `Cache: stale` with 0 rows for all nine resources as a consequence of
  the above; a completed sync now reports a populated, fresh cache.
- Responses were cached without regard to which customer tenant they belonged to. One
  API token addresses many organizations and the tenant is chosen by a request header,
  so two `--org` runs against the same command could share a cached response and return
  the wrong organization's data. Cached responses are now partitioned by tenant.
- The MCP server never applied tenant scoping at all: it built its API client on a
  different path from the CLI and so ignored `THREATLOCKER_ORG_ID`. Both surfaces now
  share one implementation.
- `sync` could still report success while storing nothing useful. A resource that
  returned rows but stored none, a run that ended on an unreadable response, and rows
  that reached the shared table but failed their per-resource table all counted as
  clean successes. Each is now reported as a warning, so an empty local mirror can no
  longer look like a healthy sync.
- `sync` no longer runs resources that cannot be listed: a duplicate copy of computer
  groups, and scheduled actions, which the API rejects without a parameter sync does
  not send. `applications` and `approvals` now sync from their real endpoints, so
  `applications hunt` and `approvals triage` have data to read.

### Changed
- Regenerated on the printing-press 4.30.2 engine (was 4.24.0). The sync fix above comes
  out of the generator and the spec rather than being hand-maintained: the engine now
  supports POST-with-body list endpoints for sync and resolves per-entity primary keys.
  Also brings corrected pagination across large result sets, robust numeric-ID handling,
  the self-learning `teach`/`recall` surface, and dependency security updates.
- `computers list`, `organizations list`, `applications search` and `approvals list` now
  cover the authenticating organization's whole managed tree by default. One request with the child-organizations flag set returns
  the full tree, so this is the correct default for an MSP and costs no extra API calls.
  Pass `--child-orgs=false` (computers) or `--all-children=false` (organizations) for the
  authenticating organization alone.
- Go toolchain moved to go1.26.6, clearing GO-2026-6218 (see #210).

### Known limitations
- Four resources still store no rows and are tracked separately: `computer-groups`
  (its sync endpoint returns a different shape from the identically-pathed list command),
  `reports` (a grouped wrapper with no row-level key), `online-devices` (returns no rows),
  and `scheduled-actions` (HTTP 417; needs a parameter sync does not send).

## [0.1.0]

### Added
- Initial msp-skills release: the `threatlocker-cli` CLI and `threatlocker-mcp` MCP
  server for the ThreatLocker Portal API, with a cross-tenant offline SQLite mirror.
- Cross-tenant approval triage (`approvals triage`) and one-command approve-across-tenants
  (`approvals approve-batch`), deduping requests by file hash.
- Audit evidence past the 31-day retention cliff: `audit export` (JSONL/CSV, per-tenant
  or all-tenants) and `audit retention-check`, plus `audit drift` for security-relevant changes.
- Fleet health: `devices health` classifies every endpoint online / offline / stale /
  isolated, and `applications hunt` locates a file by hash, certificate, or path across
  every tenant and endpoint.
- Full ThreatLocker write surface (applications, policies, computer maintenance and
  protection) with `--dry-run` previews and `--agent` JSON output for AI agents.
