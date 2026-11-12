# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [Unreleased]

### Changed
- **A second `sync` reads only what changed.** HubSpot's CRM object lists (contacts, companies, deals, tickets, calls, emails, meetings, notes, tasks, leads, line items, products, quotes) have no "modified since" filter, so every `sync` used to read every record again. Once a resource has synced completely, the next plain `sync` asks HubSpot's search for the records modified since the last one (with a 5 minute overlap, because search takes a few moments to see an update) and stores only those. The sync event stream shows `sync_delta` with the number of changed rows. Owners, pipelines, lists and imports are still read in full. `sync --full`, `--since`, `--with-history`, a `--param` filter, and a change to the properties sync collects still read the whole list; after a `--param` filter or a property change, so does the next plain `sync`. With `--max-pages`, an incremental `sync` stops after that many pages and the next one continues where it stopped. An interrupted full read of these objects now starts again from the first page, so no changed record is skipped. The first `sync` after updating reads everything once ([#695](https://github.com/Servosity/msp-skills/issues/695)).
- **Records deleted in HubSpot now clear on the weekly `sync --full`, not on every `sync`.** An incremental `sync` only sees records that still exist, so it never removes anything from your local copy. Run `sync` as often as hourly and `sync --full` once a week: the full read walks every record and removes the ones HubSpot no longer has. Until then a deleted record can stay in local reports for up to a week.

## [0.2.10] - 2026-11-11

### Fixed
- **`sync` no longer calls HubSpot's list ID lookup as if it were a list.** The `hubspot-lists-crm` sync resource failed with HTTP 400 on every run, because `GET /crm/v3/lists/idmapping` translates one legacy list ID. It is gone from `sync`, and its unfinished checkpoint is removed from existing local stores. `sync` stores no HubSpot lists, because HubSpot only returns them all through `POST /crm/v3/lists/search`: call `hubspot-cli hubspot-lists-crm post-v3-lists-search-v3-lists-search` to read them ([#693](https://github.com/Servosity/msp-skills/issues/693)).
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).
- **`sync` reads HubSpot's CRM objects 100 rows a page instead of 10.** HubSpot's published default is 10, but its CRM object lists return 100 rows a page when asked, so a full sync now makes ten times fewer requests for the same rows ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.2.9] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.2.8] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` memory no longer grows with the number of rows.** HubSpot's edge sets a new bot-protection cookie (`__cf_bm`) on every request that sends none, and the credential check holds every cookie a response sets until the program exits. The CLI kept no cookies, so it held one more per page: a 31,883-company sync held about 3,400, grew to 1 GB and was stopped after 5 hours. The CLI now sends the cookie back and receives a new one only when it expires. A 30,000-company first sync peaks at 135 MiB, against 128 MiB for 5,000.
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.2.7] - 2026-11-08

### Fixed
- The first sync finishes in a 1 GB container; the store-write credential check no longer holds memory near its decode budget ([#612](https://github.com/Servosity/msp-skills/issues/612)).
- A record the credential check withholds from the local database no longer stops the sync of its resource: the record is skipped, every other record is stored, and the sync names the skipped records and exits non-zero so the next run fetches them again ([#612](https://github.com/Servosity/msp-skills/issues/612)).

## [0.2.6] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.2.5] - 2026-11-04

### Changed
- **Rebuilt on cli-printing-press 4.34.0.** The MCP server's `--transport http` mode refuses to start without `HUBSPOT_MCP_HTTP_TOKEN`, needs TLS off loopback, and refuses a request body over 4 MiB or one that takes longer than 30 seconds to arrive. `HUBSPOT_MCP_READ_ONLY=1` (preset in every install channel) drops the write tools; `HUBSPOT_NO_LOCAL_DATA=1` keeps no local copy of API responses.
- **`contacts bulk-update` needs `--yes` for every live write.** Batches of 100 rows or fewer used to send without confirmation; now they refuse and send nothing unless you pass `--yes` (or preview with `--dry-run`). Batches over 100 rows keep the digest confirm. Scripts that run a small live batch must add `--yes`.

### Fixed
- **`contacts bulk-update` reports only what HubSpot confirmed.** A row counts as updated only when the batch finished and a result names that row by its record id, or under `--id-property` by that property's value (never by the row's own `id` handle). Rows sharing a key in one batch, rows HubSpot did not confirm, and rows rejected while reading the input all fail the run (exit 6) unless you pass `--allow-partial-failure`.
- **Timestamps compare by time, not as text.** `since`, `engagements of`, `notes signals`, `nurture-mine`, `meetings history`, `meetings ever-had`, `meetings status-report` and `deals velocity` order and filter by the actual time, so a timestamp with milliseconds no longer sorts before the same second without them, and a `--since` cutoff with milliseconds is kept. A cutoff is read as UTC, and one that is not a duration, an RFC3339 timestamp or YYYY-MM-DD is refused.
- **A property cleared in HubSpot now clears in the local copy.** Before, `sync` (with or without `--with-history`) and the cache written after a live read or update kept the old value, so a deal whose owner was removed still showed the old owner and `deals unowned` left it out.
- **`sync --with-history` keeps the full snapshot.** It no longer narrows the properties HubSpot returns to the history properties, and history rows record HubSpot's `sourceType`.
- **Deal commands agree on which deals are open.** `pipeline-health`, `owner-load`, `deals top`, `deals forecast`, `deals unowned`, `deals velocity`, `stale deals`, `nurture-mine` and `contacts win-back` read the pipeline the same way and treat a deal as closed when HubSpot marks it closed or its stage is a closed stage, including custom closed stages.
- **`sync` collects the deal pipelines and each deal's `hs_is_closed`.** The default sync now stores the deal pipelines (resource `hubspot-pipelines-crm`) and requests `hs_is_closed` and `hs_is_closed_won` with HubSpot's default deal properties, in the store the command reads (`--db` included), so the reports below work on a fresh install after `hubspot-cli sync`, and over MCP after the sync tool. Every report reads the stored pipelines the same way, including pipelines a live pipelines read stored.
- **Deal reports read only deal pipelines.** HubSpot keeps separate pipelines for deals, tickets and other objects, and the local copy stores them together. Each stored pipeline now records the object type it was fetched for, and the deal reports read only deal pipelines, so a ticket pipeline (from `hubspot-pipelines-crm get-v3-pipelines-object-type-get-all tickets`, or a pipeline create) is never reported as a deal pipeline. A store from an earlier version has no record of which pipelines are deal pipelines, so the deal reports refuse until you run `hubspot-cli sync --resources hubspot-pipelines-crm,hubspot-deals-crm` once.
- **Reports never answer from data the local copy does not have.** `sync` does not collect associations. `deals forecast`, `deals top` and `nurture queue` now fail on a deal whose stage has no known HubSpot probability instead of weighing it 0, and `pipeline-health`, `owner-load`, `deals unowned`, `deals velocity`, `stale deals` and `nurture-mine` fail on a deal whose open or closed state is unknown; each error names the remedy: `hubspot-cli sync --resources hubspot-pipelines-crm,hubspot-deals-crm`, or the MCP sync tool with the same resources. `contacts win-back`, `notes signals --pipeline` and `engagements of --data-source local` need associations and now refuse instead of returning an empty list; `deals top`, `nurture-mine`, `nurture queue` and `notes signals` still answer but list what they could not compute under `not_computed`.
- **`pipeline-health` never counts a closed deal as open money at risk.** It adds `open_count` and `open_amount`, labels closed stages, shows deals in a stage the stored pipeline does not list in their own row, picks HubSpot's own `default` pipeline for `default`, and prints an unknown stage probability as unknown instead of 0.
- **`deals velocity` only dates a deal from history that describes its current stage.** A deal whose stage changed after the last `--with-history` sync, or that has no history, is listed under `skipped` instead of getting an inflated dwell time.
- **`sync` and `--all` read every page.** HubSpot lists put the next page under `paging.next.after`; sync and live `--all` reads now follow it, so a list longer than one page is no longer cut off after the first page.
- **`sync` collects every property the reports read.** HubSpot returns only the properties a read asks for. `sync` now asks for each object's default properties plus the ones the reports use (owner, lifecycle stage, last contact and last activity, and each engagement's title and outcome), and a `--param properties=` you pass adds to that list instead of replacing it. `engagements of` asks for the same list when it reads live. A report that finds a stored record synced without one of those properties refuses and names `hubspot-cli sync --resources <resource> --full` (or the MCP sync tool with the same resource and full), instead of treating it as empty. A `--filter` on a property the stored records lack refuses the same way and names the `--param properties=<name>` that adds it. The check covers HubSpot's default properties too: a live read that named only some properties (`--properties`) stores the record without the rest, and a report such as `owner-load` now refuses instead of counting a missing amount as $0. A store from an earlier version needs that full sync once.
- **`deals velocity` also skips history captured before the deal last changed.** A deal that left its stage and came back after the last `--with-history` sync, or that has no `hs_lastmodifieddate`, is listed under `skipped` as `stale_history`. The check uses the time HubSpot reported for the record the history came with, not the time the row was written, so a stage change while a sync was running is caught. History stored by an earlier version counts as stale until the next `--with-history` sync.
- **Every report checks the deals it reads.** `deals forecast` and `deals velocity`, and `contacts win-back`, `nurture-mine` and `notes signals` for the deals they join, now refuse a stored deal synced without a property sync collects (for example after a live read with `--properties`) and name `hubspot-cli sync --resources hubspot-deals-crm --full`, instead of counting a missing amount or close date as none.
- **A deal pipeline deleted in HubSpot stops answering the deal reports.** The next pipelines sync, or a live read of the deal pipelines, retires every stored pipeline HubSpot no longer lists, so `pipeline-health <label>` never picks a deleted pipeline over a new one with the same label.
- **The guide says what `HUBSPOT_MCP_READ_ONLY=1` drops:** the local-write tools (`teach`, `teach-playbook` and the rest) are dropped too.
- **`engagements of` reads every page of associations** and fails instead of returning a partial timeline when any page or record cannot be read.
- **`meetings ever-had --value` matches in any case**, so `--value Scheduled` finds HubSpot's stored `SCHEDULED`.
- **`--csv` output from the reports neutralizes spreadsheet formulas**, like the endpoint commands' CSV.
- **`stale` applies `--filter` before `--limit`, and `nurture queue` scores every candidate before `--top`,** so a filtered run no longer returns too few rows.
- **A local read error fails the command** instead of returning a partial answer that reads as complete. `engagements of --data-source local` never goes to the API.

### Security
- **Commands that change your portal now need `--yes` or `--dry-run`.** Every create, update, upsert, merge, archive, delete and association command, plus `import`, now refuses and sends nothing unless you pass `--yes` (send it) or `--dry-run` (preview it), exiting 2 with `"confirmation_required": true` in the `--json`/`--agent` envelope. A run profile now saves and applies only output, format and filter flags, so no saved profile can carry `--yes`, `--confirm`, `--digest` or any other approval flag (this also covers `contacts bulk-update` over 100 rows and `local-data purge`). `profile save` names what it left out; a profile file that carries any other flag gets a warning and the value is ignored. A profile saved only to preview with `--dry-run` is no longer accepted. POSTs that only read (search, batch read) are unchanged. Scripts that run a write must add `--yes`.
- **`import` creates records again.** For most object types it sent each record to the batch archive endpoint instead of the create endpoint. It now posts each record to that object's create endpoint, and refuses `hubspot-lists-crm` and `hubspot-imports-crm`, which have no single-record create endpoint.
- **An MCP tool call can no longer make `contacts bulk-update --from-csv` read a file on the host.** The flag is not available over MCP; use the CLI for a CSV import.
- **Credentials are guarded by the shared credential kit.** The CLI and the MCP server hold every credential before either prints anything: the access token from the environment, the config file or the credentials file, every `[headers]` value, `HUBSPOT_MCP_HTTP_TOKEN`, a minted bearer and the MCP TLS key. Any line of output, file write, local database statement, MCP message, child process or outbound request that carries a held value, as written, encoded or as any 8-character piece, is withheld as a whole unit and replaced by a `[credkit: withheld ...]` notice.
- **A credential only goes to HubSpot, over TLS.** A request that carries one over plain `http`, to another host, or across a redirect to another host is refused, and so is a `--deliver` webhook or `feedback --send` that carries one. A `HUBSPOT_BASE_URL` on plain `http` works only for requests that carry no credential.
- **A run profile no longer prints a credential.** `profile show` and `profile use` replace the output with the kit's notice when a profile holds a value under a credential's name or under base-url. A plain profile prints as before.
- **A credential spelled `true`, `false` or `null` refuses to start.** It cannot be told from ordinary output, so the program exits 78 and sends nothing.
- **The learn journal and other local files never keep a credential.** The `HUBSPOT_LEARN_SESSION` value is stored hashed, a flag name the command does not declare is recorded only as a placeholder, `teach.log` records argument counts instead of arguments, and a journal record, `teach`, `recall`, `profile save` or `feedback` text that carries a held value is withheld rather than written.
- **No MCP tool uploads a file.** `hubspot_execute` refuses HubSpot's CRM file import endpoint before any preview or request, and an MCP value is never read as a path on the host; run a file import with `hubspot-cli`.

## [0.2.4] - 2026-10-22

### Security
- The MCP server no longer opens a local file named by a binary multipart (upload) field and sends it to the vendor; run such an upload with the CLI.

## [0.2.3] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

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
