# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.6.3] - 2026-09-01

### Fixed
- **`recall` and `playbook list` no longer claim to be read-only.** Both open the writable
  learn store and record a row, but were annotated `mcp:read-only=true` - and that annotation
  is what an MCP host reads to decide what to auto-approve without asking you. They are now
  `mcp:local-write`, a tier this engine already defines and already uses for `teach`: writes
  land only in the CLI's own local store, never in external state and never in a user-visible
  file. Measured at the live MCP server, both tools moved from `readOnlyHint=true,
  openWorldHint=true` to `readOnlyHint=false, destructiveHint=false, openWorldHint=false`.
  No behaviour changed - both commands write exactly what they wrote before. The promise was
  the defect, not the write.

### Changed
- Install and remote-agent documentation corrected against the shipped binaries. The remote
  section named `mcp-remote`, which bridges the opposite direction and cannot publish a local
  stdio server at all; connectors that parse `--transport http` now point at the native flag
  and the rest at `supergateway`. The HTTP endpoint is `/mcp`, not the bare root the docs gave.
  The Windows install path and a fallback paragraph describing an `npx` install that is not
  offered were both wrong and are gone. A new `check_install_docs` gate holds these claims
  against the binaries and installers so they cannot drift again.

## [0.6.2] - 2026-08-26

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

## [0.6.1] - 2026-08-26

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
  authenticate. Now declared on every install channel: `PRINTING_PRESS_CLIENT_PROFILE`, `SERVOSITY_MSP_BASE_URL`, `SERVOSITY_MSP_RESELLER_ID`, `SERVOSITY_MSP_USER_AGENT`.

## [0.6.0] - 2026-08-24

### Security

- Retrieving the current user's live API token now requires the explicit,
  human-only `--reveal-token` flag. The endpoint is no longer available through
  MCP discovery or execution, default sync, or `workflow archive`. Live token
  reads bypass both response caching and SQLite write-through caching.
- During writable migration, store schema v10 deletes API-token resource rows,
  related typed rows, sync state, and rebuilt full-text index entries. MCP
  search and SQL refuse stores that require this security migration. The store
  also rejects future attempts to persist this sensitive resource.
- Older binaries, SQLite free pages and WAL files, response caches, backups,
  and prior MCP transcripts may still contain the credential. Users who ran
  sync, archive, or the MCP endpoint should upgrade, remove obsolete local
  stores and caches where appropriate, and rotate the Servosity token.

### Fixed

- `servosity-cli api` now prints the shipped binary name instead of the
  internal `servosity-msp-cli` mint name (#254, reported by @DamienStevens).

## [0.5.1] - 2026-08-24

### Fixed

- The MCP `context` tool no longer advertises the admin-only `issues archived`
  and `issues ignored` endpoints that were removed from the reseller-scoped
  connector. Regression coverage and the hand-fix ledger now protect this
  generated surface from future reprints (#253, reported by @DamienStevens).

## [0.5.0] - 2026-08-17

Regenerated the vendored `servosity-cli` / `servosity-mcp` source from
cli-printing-press 4.24.0 to 4.30.3 (engine swap). This release carries five
breaking changes. Read the `### Changed` section before upgrading a script.

### Changed

- **BREAKING. Positional argument order changed on 15 commands.** 4.24 bound
  path parameters alphabetically; 4.30 binds them in URL path order, which is
  the correct order and what the API expects. Every affected command takes
  same-typed IDs on both sides of the swap, so an existing script does not
  error - it silently addresses a different resource. Five of the 15 are
  `DELETE`:

  | Command | Old order | New order |
  | --- | --- | --- |
  | `backup-job-report` | `<backup_destination_id> <backup_id> <backup_job_id> <backup_set_id>` | `<backup_id> <backup_set_id> <backup_job_id> <backup_destination_id>` |
  | `backup-job-report-summary` | same as above | same as above |
  | `companies c2c companies-delete` (DELETE) | `<backup_type> <id>` | `<id> <backup_type>` |
  | `companies restore-queues companies-jobs-delete` (DELETE) | `<company_pk> <id> <resticrestorequeue_pk>` | `<company_pk> <resticrestorequeue_pk> <id>` |
  | `companies restore-queues companies-jobs-order-first` | same as above | same as above |
  | `companies restore-queues companies-jobs-order-last` | same as above | same as above |
  | `companies restore-queues companies-jobs-partial-update` | same as above | same as above |
  | `companies restore-queues companies-jobs-update` | same as above | same as above |
  | `restic-backups backup-sets restic-backups-delete` (DELETE) | `<id> <resticbackup_pk>` | `<resticbackup_pk> <id>` |
  | `restic-backups backup-sets restic-backups-exclude-paths-delete` (DELETE) | `<id> <resticbackup_pk> <resticbackupset_pk>` | `<resticbackup_pk> <resticbackupset_pk> <id>` |
  | `restic-backups backup-sets restic-backups-exclude-paths-toggle-case-sensitive` | same as above | same as above |
  | `restic-backups backup-sets restic-backups-partial-update` | `<id> <resticbackup_pk>` | `<resticbackup_pk> <id>` |
  | `restic-backups backup-sets restic-backups-read` | same as above | same as above |
  | `restic-backups backup-sets restic-backups-source-paths-delete` (DELETE) | `<id> <resticbackup_pk> <resticbackupset_pk>` | `<resticbackup_pk> <resticbackupset_pk> <id>` |
  | `restic-backups backup-sets restic-backups-update` | `<id> <resticbackup_pk>` | `<resticbackup_pk> <id>` |

  Only `companies c2c companies-delete` pairs differently-typed arguments (a
  UUID and an enum string), so it is the one case likely to 404 instead of
  hitting the wrong record. Re-read `<command> --help` before re-running any
  stored invocation.

- **BREAKING. `--agent` no longer implies `--yes`.** In 4.24 the `--agent`
  bundle silently set `--yes`, so a confirmation-gated command run by an agent
  never prompted. In 4.30 `--agent` expands to `--json --compact --no-input
  --no-color` only, and `profile delete`, `client delete` and `client cache
  clear` refuse without an explicit `--yes`. This is the safer default but it
  changes the write-gating contract: any agent script relying on `--agent`
  alone now exits non-zero instead of proceeding. `SKILL.md`, `README.md` and
  `governance.md` still described the old behavior and are corrected here.

- **BREAKING. Two sync resources no longer resolve, and `reports` points
  somewhere new.** `sync --resources reports-account` and `sync --resources
  companies-fully-managed-ng` now fail with `unknown sync resource`. Their
  replacements are `reports` and `fully_managed_companies` respectively. The
  quiet half: `reports` used to sync `/reports/usage/` and now syncs
  `/reports/account/`, so it still resolves but mirrors different data. Usage
  reporting moved to the new `reports-usage` bucket. `contracts-get-by-token`
  is also new.

- **BREAKING. MCP `endpoint_id` `companies.fully-managed-ng` was renamed to
  `fully_managed_companies.companies-fully-managed-ng`.** Both point at
  `GET /companies/fully-managed-ng/`. Anything that hardcoded the old id in a
  `servosity-msp_search` / `servosity-msp_execute` call breaks.

- **BREAKING. 79 read commands lost their offline fallback.** They flipped
  `resolveRead` strategy from `auto` to `live`, so `--data-source local` now
  hard-errors with `no local data source for this command` (exit 5) instead of
  reading the mirror, and the default `auto` no longer falls back to local when
  the API is unreachable. All 79 are nested sub-resource reads under
  `companies` (30), `dr-backups` (14), `restic-backups` (13), `resellers` (11),
  `backups` (5) plus `backup-sets read`, `credentials versions credentials`,
  `issues events issues`, `backup-job-report`, `backup-job-report-summary` and
  `backup-job-status`. The 57 flat list and read commands keep `auto`.

- **The local store schema moved from 4 to 9, one way.** Once a 0.5.0 binary
  opens `data.db`, a 0.4.0 binary refuses it with `database schema version 9 is
  newer than supported version 4`. Keep `servosity-cli` and `servosity-mcp` on
  the same version; downgrading means deleting the store and resyncing.

- **`sync --full` now prunes.** A full resync reconciles deletions and removes
  local rows the API no longer returns for a fully enumerated partition. It was
  purely additive before. Pass `--no-prune` for the old behavior.

- **30 flags across 23 commands changed type from string to int**, including
  `agent-login create --company-id`, `companies create --reseller-id`, the
  `company-notes` and `credentials` create/update trio, and the
  `restic-backups backup-sets` id flags. Two consequences: a non-numeric value
  is now rejected by the flag parser, and the JSON request body carries a
  number where it carried a string (`"reseller_id": 42`, not `"reseller_id":
  "42"`).

- **Errors now print a JSON envelope on stdout under `--agent` / `--json`.**
  API and usage failures emit `{"code":N,"error":"..."}` to stdout in addition
  to the human message on stderr; stdout used to be empty on error. A script
  that treats non-empty stdout as success needs to check the exit code instead.

- **`meta.source` reports `dry-run` under `--dry-run`**, where it previously
  reported `live`.

### Added

- **18 new command paths (9 new top-level commands).** `export <resource>
  [id]` writes JSONL or JSON for backup, migration and analysis.
  `fully-managed-companies` is the fleet-wide fully-managed list. The remaining
  16 are the self-learning loop: `recall`, `teach`, `teach-pattern`,
  `teach-lookup`, `teach-playbook`, `playbook` (`list`, `amend`) and
  `learnings` (`list`, `candidates`, `stats`, `confirm`, `reject`, `forget`,
  `purge`).

- **`fully-managed-companies` recovers an endpoint that had no command.**
  `GET /companies/fully-managed-ng/` existed only in the MCP endpoint table and
  the sync map; the CLI could reach the single-company read but not the list.
  Press 4.30 added a declared-API-surface conformance test that fails the build
  when a declared path has no Cobra command, which is what surfaced it.

- **26 `--x-servosity-mfa` flags** on the MFA-gated endpoints: every
  destructive delete plus every encryption-key and API-token read or write,
  across `restic-backups` (6), `backups` (5), `dr-backups` (5), `current-user`
  (3), `companies` (3), `users` (2), `credentials` (1) and `resellers` (1). The
  flag sets the `X-Servosity-Mfa` request header. Additive: no existing
  invocation changes.

- **Six new global flags** - `--home`, `--client-profile`, `--no-learn`,
  `--receipt`, `--receipt-file`, `--audit-dir` - and the `SERVOSITY_MSP_NO_LEARN`
  environment variable. No global flag was removed.

- **The MCP server went from 37 to 155 tools.** Every non-endpoint CLI command
  now has a shell-out tool alongside the existing
  `servosity-msp_search` / `_execute`, plus a new `servosity-msp_get`. Additive,
  but it is a much larger tool list for a connecting agent to hold.

- `agent-context` schema_version moved from 3 to 4 and gained top-level `paths`
  and `learn_protocol` sections.

### Fixed

- **`auth logout` reported success while the token stayed live, and
  `auth set-token` silently kept using the old one.** The credentials layer was
  config-path-aware on read but not on write: `config.Load` prefers the
  credentials store colocated with an explicit `--config` /
  `SERVOSITY_MSP_CONFIG` file, but `SaveCredentials` and `RemoveCredentials`
  always resolved the default data-directory store. So `auth logout` printed
  `Logged out. Credentials cleared.` while the sibling token stayed on disk and
  kept reaching the wire, and `auth set-token` wrote the rotated token to the
  default store while the sibling kept serving the superseded one - a revoked or
  leaked credential kept being used with nothing to say so. Both paths now
  resolve the same store the read path prefers, `auth set-token` names the file
  it actually wrote, and `auth logout` clears the sibling and the default store
  because either can authenticate the same config (#248).

- **The MCP server let a tool caller choose filesystem paths.** The shell-out
  gate blocked four destination flags and missed `--db` (every local-store
  command), `--out` (`qbr`, `qbr-all`), `--input` (`import`), `--notes-file` /
  `--playbook-file` / `--playbook-notes-file` (`teach`) and `--reconcile`
  (`bill`): 28 of 155 tools forwarded one, so an MCP caller could direct a read
  or a write anywhere the server account could reach. The gate now refuses any
  flag whose usage text names a filesystem location, matched on the description
  rather than the name so the generated API body fields `--path`,
  `--seed-path`, `--source-paths`, `--exclude-paths`, `--filename` and
  `--working-dir` stay callable (#248).

- **A cross-host redirect stripped only `Authorization`.** With the new
  `X-Servosity-Mfa` header there is a second live credential on the wire, and
  `config.headers` lets an operator put an API key in any header name, all of
  which Go replayed to whatever host a 3xx named. Every credential-classed
  header is now deleted on a host change. Same-host redirects are unchanged and
  still re-derive `Authorization` (#248).

- `--dry-run` no longer prints the last four characters of the token. The
  Authorization preview is now a bare `****`.

- Documentation: `guide.md` and `SKILL.md` printed the old positional order for
  `backup-job-report` and `backup-job-report-summary`, and `SKILL.md`,
  `README.md` and `governance.md` claimed `--agent` implies `--yes`. All
  corrected against the binary.

### Removed

- The `reports-account` and `companies-fully-managed-ng` sync resources. See
  the `### Changed` entry above for the replacements.

### Security

- `golang.org/x/text` bumped to v0.39.0 (GO-2026-5970).

### Known limitations

- This release is not live-verified. The 2026-06-05 badge was earned against
  the 4.24.0 binary and the engine swap invalidated it, so the connector is
  back to `awaiting`. Nothing in this release has been exercised against a real
  Servosity tenant.

## [0.4.0] - 2026-07-06

### Removed
- `issues archived` (`GET /issues/archived/`) and `issues ignored`
  (`GET /issues/ignored/`) are removed from the partner surface. Both endpoints
  are admin-scoped and return HTTP 403 for the reseller-scoped
  `SERVOSITY_MSP_TOKEN` this connector ships, so they never worked here. The
  removal spans every surface: the two CLI subcommands, the MCP code-orchestration
  endpoint table (the `servosity-msp_search` / `servosity-msp_execute` tools),
  the `sync` resource map + store primary-key map, and the `guide.md` / `SKILL.md`
  docs. Admin functionality lives in Servosity's separate internal admin CLI
  (admin-token-scoped), not this partner connector. The
  reprint-durability requirement is documented in `docs/reprint-survival.md` and
  machine-pinned in `handfixes.json`. `issues attention`,
  `archive`/`ignore`/`reactivate` and the `issues` table's `ignored_until` field
  are partner-scoped and unaffected (#178).

## [0.3.1] - 2026-06-16

### Fixed
- Authentication: `servosity-cli` / `servosity-mcp` now send the partner token
  with the required `Token ` scheme on the `Authorization` header. The Servosity
  API authenticates `SERVOSITY_MSP_TOKEN` via Django REST Framework's
  `TokenAuthentication`, which rejects a bare token value with HTTP 403 on every
  data endpoint; the documented bare-token setup now works as written. The MSP
  token path normalizes to the `Token ` scheme, so a value that already carries
  a scheme prefix (the `SERVOSITY_MSP_TOKEN="Token <token>"` workaround) is not
  double-prefixed. Thanks to @sonofcar102 for the detailed report (#78).

### Changed
- Regenerated the vendored `servosity-cli` / `servosity-mcp` source from
  cli-printing-press 4.24.0 and re-rendered the templated skill surfaces. No
  changes to command names or flags; the fleet mirror, snapshot history, search,
  and the reporting/revenue commands (`qbr`, `fleet-health`, `email-draft`,
  `backup-facts`, `storage-trend`, `unprovisioned`, `triage`) behave as before.

## [0.2.0]

### Changed
- Engine upgrade from the 2026-06-05 reprint: the fleet CLI grows a reporting and
  revenue surface on top of the morning-sweep commands.

### Added
- `qbr` / `qbr-all`: the backup section of a client's Quarterly Business Review as
  Markdown, HTML, or PDF - one client or the whole book in one pass.
- `email-draft --stale`: ready-to-paste follow-up email bodies for every client
  with a stale backup, filled from the local store.
- `fleet-health`: one fleet-wide scorecard (24h job success rate, stale companies,
  open issues) with week-over-week deltas.
- `bill --reconcile`: line-by-line comparison of your monthly Servosity bill
  against a CSV of what you invoice your clients - surfaces over/under-charges.
- `unprovisioned`: agents installed at clients but not yet pulling backups,
  ranked by client - the lost-revenue surface.
- `storage-trend`: linear-regression forecast of when a client hits a storage
  capacity threshold, from locally captured measurements.
- `restore-queue watch`: one terminal pinned on every client's restore queue
  during a DR event, printing diffs per tick.
- `backup-facts`: one row-per-backup view across every backup type with
  freshness-derived health.

### Removed
- `clear`, `stale-issues`, `company show`, `find`, and `restore-queue list` from
  the previous engine. `triage` now carries the batch issue mutations
  (ignore/archive/reactivate/comment) with the opt-in `--dry-run` preview;
  `search` replaces `find`; `restore-queue watch --once` replaces
  `restore-queue list`.

## [0.1.1] - 2026-06-02

### Changed
- First marketplace-ready release: one-click `.mcpb` install, validated plugin
  manifest, and registry metadata aligned for submission. No CLI/behavior change
  from 0.1.0.

## [0.1.0] - 2026-05-26

### Added
- Initial msp-skills release: Servosity CLI (`servosity-cli`) + MCP server
  (`servosity-mcp`).
- Fleet-wide backup triage, stale-backup detection, and cross-engine analytics.
- Local fleet mirror with snapshot history so the partner portal's per-page
  views become one query.
- Cross-agent install: Claude Desktop `.mcpb`, Claude Code / Codex / Cowork,
  GitHub Copilot, Gemini CLI, ChatGPT (remote), Microsoft 365 Copilot (remote),
  Hermes, and OpenClaw.
