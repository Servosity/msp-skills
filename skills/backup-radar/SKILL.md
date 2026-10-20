---
name: backup-radar
description: "Every Backup Radar v3 and v2 endpoint plus a local store that finds the backups nobody is watching. Trigger phrases: `which backups is nobody watching`, `morning backup check`, `backup compliance for a client`, `retire stale backups`, `backup radar failures today`, `use backup-radar`, `run backup-radar`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Backup Radar"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - backup-radar-cli
    install:
      - kind: script
        bins: [backup-radar-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.ps1
---

# Backup Radar CLI and MCP server

## Prerequisites: Install the CLI

This skill drives the `backup-radar-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.ps1 | iex
   ```
3. Verify: `backup-radar-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `backup-radar-cli` and `backup-radar-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Covers the ScalePad Backup Radar API v3 (client health, devices, ingest, retire) and the read-only v2 API (backups, results, inactive, retired, overview) in one binary. Sync both into SQLite, then run check for the morning triage, unwatched for blind spots, coverage diff against your backup vendor's device list, and scorecard for client QBRs.

## When to Use This CLI

Use this CLI for MSP backup monitoring questions answered by Backup Radar: daily triage of failed or missing backup results, client compliance trends, finding backups that are inactive, unmapped or unticketed, reconciling a backup vendor's device list against what Backup Radar monitors, retiring stale backups, and pushing custom backup results.

## Anti-triggers

Do not use this CLI for:
- Running, restoring or configuring backups: use the backup vendor's own CLI (datto-bcdr, veeam, cove, axcient, acronis)
- Managing PSA tickets: Backup Radar reports ticket counts, and backups retire --close-psa-tickets only closes tickets tied to backups it retires
- ScalePad Lifecycle Manager, Core assets or ControlMap data: those are other ScalePad APIs

## Unique Capabilities

### Daily triage and reporting
- **`check`**  -  One ranked list of today's failing, silent and at-risk backups, each marked new or ongoing with days in status and open ticket count; uses whichever keys you synced.

  _Reach for this first in a daily backup review instead of paging raw device or backup lists._

  ```bash
  backup-radar-cli check --include-at-risk --agent
  ```
- **`scorecard`**  -  In-compliance rate, weekly trend and worst devices for one client, or every client ranked, from up to a year of client health (v3 key); worst devices use synced device history (each sync fetches 90 days; stored days accumulate up to a year).

  _Use it for QBR backup slides and client health rankings._

  ```bash
  backup-radar-cli scorecard "Acme Dental" --days 90 --agent
  ```
- **`timeline`**  -  One device's day-by-day backup story merged from both API generations, with tickets, tags and notes; --live adds each day's v2 results (v2 key).

  _Use it to explain a single device's failures before a client call._

  ```bash
  backup-radar-cli timeline "DC01" --job "Nightly Image" --days 30 --agent
  ```

### Find what nobody is watching
- **`unwatched`**  -  Every backup Backup Radar receives but nobody acts on: inactive, unmapped to ScalePad, failing with no ticket, no PSA company, or warnings counted as success by policy. Unmapped needs the v3 key; the other reasons need the v2 key.

  _Use it to answer which backups have no human or ticket behind them before a client finds out._

  ```bash
  backup-radar-cli unwatched --reason inactive,ticketless --agent
  ```
- **`coverage diff`**  -  Compare the devices a backup vendor protects against what Backup Radar monitors and list the gaps in both directions.

  _Use it when a vendor console and Backup Radar disagree about what is protected and monitored._

  ```bash
  backup-radar-cli coverage diff --devices vendor-devices.json --name-field hostname --agent
  ```
- **`stale`**  -  Backups with no result in N days, grouped by client, with retire request bodies ready to preview; works from either key's synced data.

  _Use it to plan a cleanup before calling backups retire, never instead of the server-side dry run._

  ```bash
  backup-radar-cli stale --days 30 --agent
  ```

### Custom ingest
- **`push`**  -  Push flat CSV or JSONL backup results into Backup Radar (v3 key, live write) with local validation and automatic batching under the 10,000-result limit.

  _Use it to report results from backup tools Backup Radar cannot parse, from any script or scheduler._

  ```bash
  backup-radar-cli push --from results.csv --dry-run
  ```

## Command Reference

**backups**  -  Ingest backup results and retire unused backups (v3 key); list, inspect, filter and count backups (v2 key).

- `backup-radar-cli backups filters`  -  Returns the available filters, such as tags, companies, statuses, and more (v2 key).
- `backup-radar-cli backups get`  -  Returns the backup data for a specific day (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).
- `backup-radar-cli backups inactive`  -  Returns inactive backups that are ready to be activated (v2 key).
- `backup-radar-cli backups ingest`  -  Accepts a root JSON array of backup entries (v3 key).
- `backup-radar-cli backups list`  -  Returns scheduled backups that match the specified filters and are paginated accordingly (v2 key).
- `backup-radar-cli backups overview`  -  Returns the counts of policies, backups, servers, and workstations (v2 key).
- `backup-radar-cli backups results`  -  Returns all backup results for the given date (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).
- `backup-radar-cli backups retire`  -  Retires backups that match the supplied filters (v3 key). Filters are exact (case-insensitive) and combined with AND.
- `backup-radar-cli backups retired`  -  Returns backups that have been retired (v2 key). Uses the Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY).

**devices**  -  Get the client's backup devices.

- `backup-radar-cli devices`  -  Returns a cursor-paginated list of backup devices whose names match the optional filters (v3 key).

**health**  -  Get the client's backup health.

- `backup-radar-cli health get`  -  Returns backup health for one client (v3 key).
- `backup-radar-cli health list`  -  Returns a cursor-paginated list of clients whose backup health matches the optional filters (v3 key).


**check**


- `backup-radar-cli check`  -  One ranked list of today's failing, silent and at-risk backups, each marked new or ongoing with days in status and open ticket count; uses whichever keys you synced.


**coverage**


- `backup-radar-cli coverage diff`  -  Compare the devices a backup vendor protects against what Backup Radar monitors and list the gaps in both directions.


**push**


- `backup-radar-cli push`  -  Push flat CSV or JSONL backup results into Backup Radar (v3 key, live write) with local validation and automatic batching under the 10,000-result limit.


**scorecard**


- `backup-radar-cli scorecard`  -  In-compliance rate, weekly trend and worst devices for one client, or every client ranked, from up to a year of client health (v3 key); worst devices use synced device history (each sync fetches 90 days; stored days accumulate up to a year).


**stale**


- `backup-radar-cli stale`  -  Backups with no result in N days, grouped by client, with retire request bodies ready to preview; works from either key's synced data.


**timeline**


- `backup-radar-cli timeline`  -  One device's day-by-day backup story merged from both API generations, with tickets, tags and notes; --live adds each day's v2 results (v2 key).


**unwatched**


- `backup-radar-cli unwatched`  -  Every backup Backup Radar receives but nobody acts on: inactive, unmapped to ScalePad, failing with no ticket, no PSA company, or warnings counted as success by policy. Unmapped needs the v3 key; the other reasons need the v2 key.




## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `BACKUP_RADAR_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

- `backup-radar-cli backups list`
- `backup-radar-cli backups get`
- `backup-radar-cli backups filters`
- `backup-radar-cli backups inactive`
- `backup-radar-cli backups retired`
- `backup-radar-cli devices`
- `backup-radar-cli health list`
- `backup-radar-cli health get`

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
backup-radar-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning triage for agents

```bash
backup-radar-cli check --include-at-risk --agent
```

Ranked failing, silent and at-risk backups with new or ongoing state, ready for an agent to open tickets.

### Narrow the device list

```bash
backup-radar-cli devices --all --agent --select br_device_name,client.br_company_name,br_job_name,last_result
```

Device rows carry long history arrays; select only the fields you need (the list wrapper is already removed).

### Find blind spots

```bash
backup-radar-cli unwatched --reason inactive,unmapped,ticketless --agent
```

One row per backup and reason, plus counts and any reasons skipped because their collection was never synced.

### Plan a cleanup

```bash
backup-radar-cli stale --days 45 --agent
```

Silent backups grouped by client. Add --bodies to print only the retire bodies, then preview one with: stale --days 45 --bodies | head -n 1 | backups retire --stdin (PowerShell: stale --days 45 --bodies | Select-Object -First 1 | backups retire --stdin). Every body has dry_run true.

### Vendor coverage gap

```bash
backup-radar-cli coverage diff --devices vendor-devices.json --name-field hostname --agent
```

Devices the vendor protects that Backup Radar does not monitor, and the reverse.

## Auth Setup

Backup Radar has two API generations with two different keys. The v3 commands (health, devices, backups ingest, backups retire, push) use a ScalePad API key from the ScalePad API tab: set BACKUP_RADAR_API_KEY (`export` on macOS / Linux, `$env:BACKUP_RADAR_API_KEY = "..."` on Windows) or store it with auth set-token. The v2 commands (backups list, get, results, filters, inactive, retired, overview, and timeline --live) use a separate Backup Radar API key that is read only from the environment: set BACKUP_RADAR_V2_API_KEY the same way. Either key alone works for its own commands, and the local reports use whatever data you synced. doctor probes each configured key against its API (GET /clients/health with the v3 key, GET /backups/overview with the v2 key) and reports valid, rejected or not verified per key. EU tenants set BACKUP_RADAR_BASE_URL=https://eu.api.scalepad.com/backup-radar/v3; that override applies to v3 commands only, because v2 always uses https://api.backupradar.com.

Run `backup-radar-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

The analysis reports (`check`, `unwatched`, `stale`, `scorecard`, `timeline`, `coverage diff`) carry several lists plus a not-synced note, so they take `--json`, `--agent` or the human table and refuse `--csv`, `--plain` and `--quiet` with a usage error.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  backup-radar-cli backups list --agent --select backupId,backupType,companyName
  ```
- **Previewable**  -  `--dry-run` shows the request without sending; `backups retire --dry-run` is the exception: it is the API's own preview flag, so the request is sent with `dry_run: true` (and with `--stdin` it sets `dry_run` in the body)
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Live writes**  -  `backups ingest`, `backups retire` (without `--dry-run`) and `push` are live writes that run immediately and never prompt; preview with `push --dry-run` or `backups retire --dry-run` first

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. The analysis reports above return their view bare under `--json` (top-level `rows`, `sources`, and so on) and wrapped in this envelope under `--agent`. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `BACKUP_RADAR_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `BACKUP_RADAR_CONFIG_DIR`, `BACKUP_RADAR_DATA_DIR`, `BACKUP_RADAR_STATE_DIR`, `BACKUP_RADAR_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `BACKUP_RADAR_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `backup-radar-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "backup-radar": {
        "command": "backup-radar-mcp",
        "env": {
          "BACKUP_RADAR_HOME": "/srv/backup-radar"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `BACKUP_RADAR_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `BACKUP_RADAR_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
backup-radar-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "backup-radar-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `backup-radar-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `backup-radar-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "DC" → DC01 + DC02). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `backup-radar-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
backup-radar-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "not reporting") satisfies future queries under another alias (e.g., "no results") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
backup-radar-cli teach \
  --query "$QUERY" \
  --resource-type <type> \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
backup-radar-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
backup-radar-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between API versions, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-client / per-device / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`backup-radar-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `BACKUP_RADAR_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
backup-radar-cli feedback "the --since flag is inclusive but docs say exclusive"
backup-radar-cli feedback --stdin < notes.txt
Get-Content notes.txt | backup-radar-cli feedback --stdin   # PowerShell
backup-radar-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `BACKUP_RADAR_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `BACKUP_RADAR_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command except `completion` accepts `--deliver <sink>` (redirect a completion script to a file instead; the `--help` and `--version` flags print to stdout only, while the `help` and `version` commands deliver). `profile save` validates a `--deliver` sink and records it for later `--profile` runs rather than delivering its own confirmation. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
backup-radar-cli profile save "briefing" --json
backup-radar-cli --profile "briefing" backups list
backup-radar-cli profile list --json
backup-radar-cli profile show "briefing"
backup-radar-cli profile delete "briefing" --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Other error |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `backup-radar-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add backup-radar-mcp --env BACKUP_RADAR_API_KEY=<scalepad-key> --env BACKUP_RADAR_V2_API_KEY=<v2-key> -- backup-radar-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which backup-radar-cli` (macOS / Linux) or `Get-Command backup-radar-cli` (Windows PowerShell)
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   backup-radar-cli check --include-at-risk --agent
   ```
4. If ambiguous, drill into subcommand help, e.g. `backup-radar-cli backups --help`.
