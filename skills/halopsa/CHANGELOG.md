# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.15] - 2026-09-01

### Added
- **`HALOPSA_NO_CONFIG_WRITE=1` keeps every credential off disk.** Set it and the CLI mints a token per
  invocation instead of caching one in `config.toml`. The MCP server honours the same switch,
  which it previously could not: it resolved its own config path and ignored `--config`
  entirely, so a Claude Desktop setup kept a plaintext token cache even when the CLI had been
  told not to write one. `auth logout` still clears a config that already exists - the switch
  suppresses credential writes, not the erase.

### Changed
- Install and remote-agent documentation corrected against the shipped binaries. The remote
  section named `mcp-remote`, which bridges the opposite direction and cannot publish a local
  stdio server at all; connectors that parse `--transport http` now point at the native flag
  and the rest at `supergateway`. The HTTP endpoint is `/mcp`, not the bare root the docs gave.
  The Windows install path and a fallback paragraph describing an `npx` install that is not
  offered were both wrong and are gone. A new `check_install_docs` gate holds these claims
  against the binaries and installers so they cannot drift again.

## [0.2.14] - 2026-08-26

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

## [0.2.13] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  It also dialled the shipped placeholder base URL (`https://{tenant}.{domain}/api`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `HALOPSA_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `HALOPSA_BASE_URL`, `HALOPSA_TOKEN_URL`.

## [0.2.12] - 2026-08-22

### Fixed
- `doctor`, the CLI's HTTP 400/401/403 error hints and the MCP server's tool-error
  hints no longer tell operators to run `halopsa-cli auth setup`. This CLI's `auth`
  parent defines `login`, `status`, `set-token` and `logout`; it has never defined
  `setup`. Cobra does not reject an unknown subcommand of a non-root parent, and the
  generated parent returns `cmd.Help()` with a nil error, so `halopsa-cli auth setup`
  printed the auth help and exited 0 - no "unknown command" error and no non-zero
  exit anywhere in the journey. An operator followed doctor's advice, saw a success
  code and was no closer to configured credentials; because these are agent-first
  CLIs with typed exit codes and an `--agent` mode, an agent performing setup read
  exit 0 as success and moved on unauthenticated. The hints fire exactly when they
  matter - fresh install, rotated secret, new machine, expired token - and this skill
  emits no `auth_instructions` field, so they were the only setup guidance it gave.
  `doctor` now names the variables `auth login` actually reads (`HALOPSA_TENANT`, `HALOPSA_CLIENT_ID` and `HALOPSA_CLIENT_SECRET`, plus
  `HALOPSA_DOMAIN` when self-hosted), and the
  CLI and MCP error paths point at `halopsa-cli auth login` to re-authenticate. Seven
  occurrences across three files; the same defect affected appdirect, crowdstrike,
  halopsa and ninjaone, the four connectors that scaffold `auth login` rather than
  `auth setup`. Reported by @geekbrownbear (#277).

## [0.2.11] - 2026-08-20

### Fixed
- `sync` no longer page-walks `feed`. HaloPSA's `/Feed` is a time-ordered
  activity stream whose own parameters are `count` for the window size and
  `newer_than_id`/`older_than_id` for position; it has no page number to
  honour. The connector sent it the paging trio every other list endpoint
  gets (`pageinate=true`, `page_size`, an incrementing `page_no`), which the
  endpoint ignores, so each request returned a window overlapping the last and
  the walk re-read rows it already had. That is what #264 recorded for `feed`
  as 100 rows fetched and 62 stored. Nothing was being dropped: `/Feed`
  identifies its rows with a globally unique id, so the repeats were one row
  fetched twice and the local mirror collapsed them correctly. `feed` now asks
  for a single window of `count` rows per sync, which is what an activity-feed
  mirror wants, and a `--param count=<n>` override still wins. Reported by
  @Xenith-B (#273).
- The same walk could fail to terminate. Because the cursor it advances is
  computed client-side, the sticky-cursor guard never fires, and a full window
  returned to every request keeps the more-pages heuristic true, so under the
  default `--max-pages 0` the loop had no way to stop. A tenant whose feed
  always filled the window would have kept requesting pages until the sync was
  interrupted. `feed` no longer enters that loop at all.

## [0.2.10] - 2026-08-20

### Fixed
- `sync` no longer silently drops rows from `team-tree`,
  `timesheet-forecasting` and `integration-runbook-variable-group`. These are
  the last three of the nine resources reported in #264; the other five were
  fixed in 0.2.8, and these needed sample records to confirm their key before
  it was safe to change. `team-tree` numbers its nodes within a node type, so
  two nodes both carry id 16 and 45 fetched rows stored 44; every row also
  carries a `guid`, which is unique by construction and is now the key.
  `timesheet-forecasting` has the same shape as `timesheet`, returning
  `"id": 0` on every record, so all 132 fetched rows collapsed onto one; it is
  keyed by the agent and the day. `integration-runbook-variable-group` carries
  no id-shaped field at all, only a `label` and a `value`, so all 39 rows
  failed extraction and the resource stored nothing; `value` is the stable key
  and is now used. Thanks to @Xenith-B for the sanitized sample records that
  made all three confirmable (#264).
- Rows cached under the old keys are cleared automatically on first open, so a
  resync repopulates them under the corrected keys rather than leaving stale
  rows alongside the new ones.
- `timesheet` and `timesheet-forecasting` rows no longer risk accumulating a
  duplicate on every sync. Both are keyed by the agent and the day because
  their records carry no usable id, and HaloPSA declares `date` as a DATETIME,
  returning values such as `2026-10-25T19:13:41.4892555+00:00`: a forecast day
  carrying a meaningless time. Had that time component been recomputed per
  request rather than stored, the key would have changed on every sync while
  the row it identifies did not, inserting a fresh copy each time. The key now
  depends only on the calendar day, which is the contract these rows actually
  have. `timesheet` is purged along with the others so any row 0.2.8 or 0.2.9
  stored under a full-timestamp key is repopulated cleanly.

### Notes
- `feed` was reported in #264 as part of the same family and is deliberately
  unchanged. Its 100 fetched to 62 stored is not row loss: `/Feed` identifies
  its rows with a globally unique `id` (its own `newer_than_id`/`older_than_id`
  paging contract depends on that), so two rows sharing an id are one row
  fetched twice, and collapsing them is correct. Walking a live activity feed
  by page number re-reads rows as new events shift the window, which is what
  the duplicate count reflects. That is a separate defect with a separate fix
  and is tracked on its own.

## [0.2.9] - 2026-08-19

### Fixed
- OAuth client credentials supplied through the environment (`HALOPSA_CLIENT_ID` and
  `HALOPSA_CLIENT_SECRET`) are no longer written to
  `~/.config/halopsa-cli/config.toml`. `Load()` marks env-supplied values so
  `configForSave()` can keep them off disk, but `SaveTokens()` cleared those
  markers before saving, so every token mint - including the automatic one on an
  ordinary authenticated command, with no `auth login` step involved - rewrote the
  live client ID and secret into the config file in cleartext. That defeats the
  setups this is meant to support, where the secret lives only in the OS credential
  store (Keychain, Windows Credential Manager) and reaches the process as an
  environment variable at launch. The minted access token is still cached there, and
  an explicit `auth login --client-id <id> --client-secret <secret>` still stores
  what you passed it. **Deliberate behaviour change:** credentials that came from
  the environment are no longer persisted by `auth login` either, so a shell that
  later drops those variables must set them again or pass the flags.
  Thanks to @Xenith-B for the report (#266).

## [0.2.8] - 2026-08-19

### Fixed
- `sync` no longer fails the whole `attachment` resource with
  `missing id for attachment`. HaloPSA answers a list endpoint with a
  `<Entity>_View` object that carries the rows next to sidecar arrays, and the
  connector picked the row array by looking for the only populated one. That
  worked by luck: Halo leaves the sidecars empty unless they are asked for.
  `/Attachment` is the endpoint that always returns its `folders` sidecar, so
  two arrays arrived, the scan could not tell rows from sidecar, gave up, and
  the entire envelope fell through to the single-object path, which failed the
  resource and stored nothing. The empty envelope an unscoped `/Attachment`
  returns on a tenant with no attachments failed the same way, which is why it
  reproduced regardless of content. The row array is now read from the key the
  spec declares for each of the 23 list resources. `tickets`, `clients`,
  `asset`, `actions` and `users` were one tenant configuration away from
  breaking identically and are covered by the same table. Thanks to @Xenith-B
  for the live-tenant report and the envelope diagnosis (#265).
- `sync` no longer silently drops rows from `lookup`, `asset-type-info`,
  `workflowstep` and `timesheet`. Each numbers its rows within a parent, so the
  bare id collides across parents and the last writer wins: `lookup` restarts
  its id per `lookupid` and stored 269 of 1020 fetched rows, `asset-type-info`
  uses a field-row index scoped to `typeinfo_id` and stored 18 of 129,
  `workflowstep` restarts `step_id` per flow and stored 144 of 211, and
  `timesheet` returns a literal `"id": 0` on every record so all 108 rows
  collapsed onto one. Rows are now keyed by their parent as well, and
  `timesheet` by the agent and the day, which is the only thing identifying it.
  This is the same defect the `actions` fix in 0.2.3 addressed, found because
  that release also started reporting the loss as a `sync_anomaly` instead of
  hiding it (#264).
- `workflowstep` and `online-status` resolve the id fields they actually carry.
  `workflowstep` has no `id`, so id resolution reached its display `name` and
  merged steps that share a name across flows; it now reads `step_id`.
  `online-status` names its identifier `techID`, which no spelling of `id`
  reaches, so every record failed extraction and the resource stored nothing;
  it now reads `techID` (#264).
- Rows cached under the old keys are cleared automatically on first open, so a
  resync repopulates them under the corrected keys instead of leaving stale
  collapsed rows alongside the new ones.

### Known limitations
- Four further resources @Xenith-B observed losing rows are not addressed in
  this release: `feed`, `team-tree`, `integration-runbook-variable-group` and
  `timesheet-forecasting`. The spec declares no response schema for any of
  them, so their key could not be confirmed, and keying rows on a guessed
  column risks a different silent loss. Tracked in #264.

## [0.2.7] - 2026-08-14

### Fixed
- `--since` now accepts the RFC3339 timestamps its own help documents. The
  parser folded its input to lowercase before trying any format, and because
  `time.Parse` matches layout characters literally, the uppercase `T` separator
  and `Z` designator no longer matched, so every timestamp was rejected as an
  unrecognized time. The fold is still applied to keywords and durations, which
  need it: `time.ParseDuration` rejects uppercase unit letters, so `24H` and
  `7D` only work because of it. Affects `standup`, `agent workload`,
  `sla scorecard`, `tickets age-out` and `tickets reopens`, all of which
  document RFC3339 and all of which rejected it. Thanks to @geekbrownbear for
  the report and the fix (#219).

## [0.2.6] - 2026-08-14

### Fixed
- `tickets reopens` now reports reopened tickets instead of saying detection is
  unavailable, and `standup`'s `reopened` column is no longer permanently zero.
  Both read a ticket-level marker HaloPSA never writes; the ticket fields record
  only the final close, while the action trail keeps the whole history. Reopens
  are now derived from it, unioning explicit Re-Open outcomes with tickets
  resolved more than once. Neither signal alone is enough: a reopen driven by a
  customer email has no Re-Open action, and a reopen not yet re-resolved has only
  one Resolved action. Because outcome names can be tenant-configured, the
  repeated-resolution signal is deliberately configuration-independent and
  carries the feature on its own where those names differ. The honest "cannot
  tell" message is kept, but now fires on an unsynced actions table, which is the
  real unknown. Thanks to @geekbrownbear for the operator-confirmed trail (#218).

## [0.2.5] - 2026-08-14

### Fixed
- Every SLA view reported zero risk and zero attainment wherever HaloPSA leaves
  `targetdate` at its `1900-01-01` sentinel, which was the case on 1264 of 1265
  tickets on the tenant this was reported against. They read that legacy column
  rather than the `fixbydate` the portal actually shows. Because a date window
  can never match `1900-01-01`, the failure was a silent zero rather than an
  error: `sla breaching` listed no tickets at risk whatever was due, `triage`
  reported `breach_count` 0 for every agent, `sla scorecard` counted those
  tickets as carrying a target nothing could satisfy and reported 0 percent met,
  and `client card` showed `1900-01-01` as their target while its `sla_at_risk`
  overlay metric stayed at 0. On the reporting tenant scorecard attainment goes
  from 0 percent to 97-99 percent, which is the real figure. `targetdate` is
  kept as a fallback, so a tenant that does populate it is unaffected either
  way. Thanks to @geekbrownbear for the portal-verified diagnosis (#213).

## [0.2.4] - 2026-08-13

### Fixed
- Eight more local views reported `(unassigned)` agents and zero-day ticket ages
  for the same reason `triage` did in 0.2.3: they read `agent_name` and
  `datecreated`, columns HaloPSA never populates. `standup`, `agent workload`,
  `sla breaching`, `sla scorecard`, `tickets age-out`, `tickets reopens`,
  `asset history`, and `client card` now resolve agent names from the synced
  agent records and fall ticket age through to `dateoccurred` (#211).
- `standup` and `agent workload` additionally collapsed every agent into a
  single row. Both grouped on a name that collides with a real column on the
  ticket table, which SQLite resolves ahead of the output alias. This is the
  same defect `triage` had, and it survived the agent-name fix on its own.

## [0.2.3] - 2026-08-13

### Fixed
- `sync` now stores every page of every paginated resource. HaloPSA honours
  paging only when `pageinate=true` and an explicit 1-based `page_no` are both
  sent, and it numbers pages rather than row offsets; the connector sent
  neither the enable flag nor a first-page cursor and advanced by row offset,
  so a full sync of a 1254-ticket instance stored 50 tickets and still reported
  success. Endpoints that ignore `page_size` and always return a fixed page
  (`/Asset` serves 50 rows however large the page size) are now walked using
  the total the API reports rather than the page length, so they no longer stop
  after one page. On the reporting instance tickets went from 50 to 1254,
  assets from 50 to 144, and users from 50 to 122. Thanks to @geekbrownbear for
  the live-tenant report and the diagnosis (#203).
- `sync` no longer discards ticket actions. HaloPSA numbers actions within
  their ticket, so every ticket has an action 1; the local mirror keyed rows on
  the bare action id, so each ticket's action 1 overwrote the previous one's and
  8185 fetched actions collapsed into 149 stored rows. Actions are now keyed by
  ticket as well as action id. `contracts burn`, `time leaks`, `standup`,
  `agent workload`, and `time gaps` all read this table and had been computing
  against a fraction of it (#203).
- `sync` completion counts now report distinct rows actually stored rather than
  rows fetched and upserted, and emit a `sync_anomaly` event when the two
  disagree, so a future key collision is visible instead of silent (#203).
- `triage` now reports a row per agent instead of a single `(unassigned)` line
  with `oldest_days: 0`. Agent names are resolved from the synced agent records
  because HaloPSA's ticket payload carries `agent_id` rather than a name, ticket
  age falls through to `dateoccurred` because `datecreated` is never populated,
  and the aggregate runs over a subquery so its grouping key can no longer
  collide with the ticket table's own `who` column (#203).
- `sync --max-pages` no longer discards its resume cursor when it caps a walk
  over an endpoint that returns a fixed short page. The cap classified such a
  walk as naturally complete, cleared the cursor, and the next sync restarted at
  page one, so a capped sync could never advance through those resources.
- Supplying the paging cursor by hand (`sync --param page_no=1`) no longer makes
  an unlimited sync re-fetch that one page forever. The override pinned every
  request to the same page while the walk advanced its own cursor, so nothing
  detected the loop. Sync now recognises a pinned cursor as a request for that
  single page, fetches it, and says so.
- Ticket actions cached by an earlier version are cleared automatically on first
  open. Those rows were written under the old un-qualified key, so without the
  purge a resync would leave one stale duplicate per action number sitting
  alongside the corrected rows.

### Changed
- The Go toolchain moves to go1.26.6, picking up the standard-library fix for
  GO-2026-6218 (quadratic complexity in `net/url`), which is reachable from the
  connector's HTTP path.

## [0.2.2] - 2026-06-20

### Fixed
- OAuth2 authentication now resolves the `{tenant}` and `{domain}` endpoint
  placeholders in the token URL, so `auth login`, automatic token minting, and
  refresh no longer fail with `invalid character "{" in host name`, on Windows
  or any platform. The token-mint paths previously POSTed the literal
  `https://{tenant}.{domain}/auth/token` because they bypassed the URL
  substitution used for ordinary requests. Thanks to @Xenith-B for the report
  (#147).

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.2.0] - 2026-06-06

### Changed
- Maintenance and packaging updates.

## [0.1.1] - 2026-06-02

### Fixed
- OAuth2 client-credentials tokens now send the `scope` parameter (defaulting to
  `all`), fixing HTTP 401 on every authenticated HaloPSA API call (refs #7).

### Changed
- First marketplace-ready release: one-click `.mcpb` install, validated plugin
  manifest, and registry metadata aligned for submission.

## [0.1.0] - 2026-05-26

### Added
- Initial msp-skills release: HaloPSA CLI (`halopsa-cli`) + MCP server
  (`halopsa-mcp`).
- Ticket triage, SLA-breach pre-emption, and cross-client analytics.
- Local SQLite mirror with full-text search for fast, offline cross-entity
  queries the live API can't return in one shot.
- Cross-agent install: Claude Desktop `.mcpb`, Claude Code / Codex / Cowork,
  GitHub Copilot, Gemini CLI, ChatGPT (remote), Microsoft 365 Copilot (remote),
  Hermes, and OpenClaw.
