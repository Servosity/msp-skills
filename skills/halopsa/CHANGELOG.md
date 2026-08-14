# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.5] - 2026-08-14

### Fixed
- Every SLA view reported zero risk and zero attainment on every tenant. They
  read `targetdate`, a legacy column HaloPSA parks at `1900-01-01`, rather than
  the `fixbydate` the portal actually shows. Because a date window can never
  match `1900-01-01`, the failure was a silent zero rather than an error:
  `sla breaching` listed no tickets at risk whatever was due, `triage` reported
  `breach_count` 0 for every agent, `sla scorecard` counted every closed ticket
  as carrying a target nothing could satisfy and reported 0 percent met, and
  `client card` showed `1900-01-01` as each ticket's target while its
  `sla_at_risk` overlay metric stayed permanently 0. On the reporting tenant
  scorecard attainment goes from 0 percent to 97-99 percent, which is the real
  figure. `targetdate` is kept as a fallback for tenants that populate it.
  Thanks to @geekbrownbear for the portal-verified diagnosis (#213).

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
