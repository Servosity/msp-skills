---
name: betterstack
description: "Every Better Stack Uptime feature, plus an offline SQLite mirror and cross-resource fleet analytics  -  what's down and who's paged, coverage gaps, MTTA/MTTR, flapping, on-call gaps, and status-page drift  -  that the API alone can't answer. Trigger phrases: `which monitors are down`, `find unprotected monitors`, `incident MTTR report`, `who is on call`, `noisy monitors`, `use betterstack`, `run betterstack-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Better Stack"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - betterstack-cli
    install:
      - kind: script
        bins: [betterstack-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.ps1
---

# Better Stack - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `betterstack-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.ps1 | iex
   ```
3. Verify: `betterstack-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `betterstack-cli` and `betterstack-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

betterstack-cli mirrors your whole Better Stack Uptime account (monitors, heartbeats, incidents, on-call, escalation policies, status pages) into a local SQLite store, then answers operational questions no single API call can: `coverage` finds monitors that won't page anyone, `mttr` rolls up incident response times, `flapping` ranks your noisiest monitors, and `fleet` shows the whole account on one screen. Covers the official Terraform provider's resource surface (full create/update/delete on monitors and heartbeats; create/delete on groups, policies, and status pages), with agent-native output and typed exit codes throughout.

## When to Use This CLI

Use this CLI when an agent or operator needs to query, audit, or provision a Better Stack Uptime account from the terminal  -  especially for cross-resource questions (which monitors are unprotected, how fast incidents resolve, which monitors flap, who is on call) that the one-resource-at-a-time API can't answer directly. Prefer it over raw API calls whenever the answer spans more than one resource type or needs to work offline.

## Unique Capabilities

### Cross-resource analytics the API can't answer
- **`fleet`**  -  One-screen health of the whole Better Stack account: monitors up/down/paused, heartbeats, open incidents, and who's on call now.

  _Reach for this first when an agent needs the operational state of the entire account in one call instead of five._

  ```bash
  betterstack-cli fleet --agent
  ```
- **`coverage`**  -  Find monitors with no escalation policy or no alert channel  -  the ones that will go down silently and page no one.

  _Use before an incident to prove every monitor actually escalates to a human._

  ```bash
  betterstack-cli coverage --agent
  ```
- **`down`**  -  Focused triage list of monitors currently down or degraded, joined to their open incidents and whether anyone is actually paged for each.

  _Reach for this at the start of a shift or during an outage to see what is down and who is (or is not) being paged, in one call._

  ```bash
  betterstack-cli down --agent
  ```
- **`triage`**  -  Open incidents ranked by age and acknowledgement state  -  never-acknowledged first  -  joined to the affected monitor.

  _Use this to prioritize incident response: it surfaces the open incidents nobody has acknowledged yet, oldest first._

  ```bash
  betterstack-cli triage --agent
  ```
- **`statuspage-audit`**  -  Flags status pages showing operational while a backing monitor has an open incident, and status-page resources pointing at missing or paused monitors.

  _Use this before or during an incident to catch public status pages that are silently out of sync with reality._

  ```bash
  betterstack-cli statuspage-audit --agent
  ```
- **`group-health`**  -  Per-group health rollup: monitor and heartbeat up/down counts plus open incidents for every monitor group and heartbeat group.

  _For MSP-style accounts where one group is one client, this answers per-client health in a single call._

  ```bash
  betterstack-cli group-health --agent
  ```

### Incident intelligence
- **`mttr`**  -  Mean time to acknowledge and resolve, computed across incidents over a window and broken down by monitor.

  _Use for on-call retros and SLA reporting without exporting to a spreadsheet._

  ```bash
  betterstack-cli mttr --days 30 --agent
  ```
- **`flapping`**  -  Rank monitors by how many incidents they generated in a window to surface the noisy, flapping, or misconfigured ones.

  _Use to find alert fatigue sources before tuning thresholds._

  ```bash
  betterstack-cli flapping --days 7 --top 10 --agent
  ```

### On-call and resilience
- **`oncall-gaps`**  -  Detect on-call calendars with nobody currently on call.

  _Use to confirm someone is actually reachable on every rotation before relying on paging._

  ```bash
  betterstack-cli oncall-gaps --agent
  ```
- **`heartbeat-risk`**  -  Rank heartbeats by risk: tight period+grace windows, paused-but-expected, and non-up status.

  _Use to catch fragile cron/scheduled-task check-ins before they false-alarm or silently miss._

  ```bash
  betterstack-cli heartbeat-risk --agent
  ```

## Command Reference

**heartbeat-groups**  -  Heartbeat groups

- `betterstack-cli heartbeat-groups create`  -  Create a heartbeat group
- `betterstack-cli heartbeat-groups delete`  -  Delete a heartbeat group
- `betterstack-cli heartbeat-groups get`  -  Get a heartbeat group by ID
- `betterstack-cli heartbeat-groups list`  -  List heartbeat groups

**heartbeats**  -  Heartbeats  -  cron/scheduled-task check-ins

- `betterstack-cli heartbeats create`  -  Create a heartbeat
- `betterstack-cli heartbeats delete`  -  Delete a heartbeat
- `betterstack-cli heartbeats get`  -  Get a heartbeat by ID
- `betterstack-cli heartbeats list`  -  List heartbeats
- `betterstack-cli heartbeats update`  -  Update a heartbeat (sends PATCH)

**incidents**  -  Incidents  -  outages and alerts across monitors and heartbeats

- `betterstack-cli incidents acknowledge`  -  Acknowledge an incident
- `betterstack-cli incidents delete`  -  Delete an incident
- `betterstack-cli incidents get`  -  Get an incident by ID
- `betterstack-cli incidents list`  -  List incidents
- `betterstack-cli incidents resolve`  -  Resolve an incident

**monitor-groups**  -  Monitor groups

- `betterstack-cli monitor-groups create`  -  Create a monitor group
- `betterstack-cli monitor-groups delete`  -  Delete a monitor group
- `betterstack-cli monitor-groups get`  -  Get a monitor group by ID
- `betterstack-cli monitor-groups list`  -  List monitor groups

**monitors**  -  Uptime monitors (HTTP, keyword, ping, TCP, heartbeat-backed)

- `betterstack-cli monitors create`  -  Create a monitor
- `betterstack-cli monitors delete`  -  Delete a monitor
- `betterstack-cli monitors get`  -  Get a monitor by ID
- `betterstack-cli monitors list`  -  List all monitors
- `betterstack-cli monitors update`  -  Update a monitor (sends PATCH)

**on-calls**  -  On-call calendars and current on-call shifts

- `betterstack-cli on-calls get`  -  Get an on-call calendar by ID (includes current on-call users)
- `betterstack-cli on-calls list`  -  List on-call calendars

**policies**  -  Escalation policies

- `betterstack-cli policies create`  -  Create an escalation policy
- `betterstack-cli policies delete`  -  Delete an escalation policy
- `betterstack-cli policies get`  -  Get an escalation policy by ID
- `betterstack-cli policies list`  -  List escalation policies

**status-page-resources**  -  Resources (monitored components) shown on a status page

- `betterstack-cli status-page-resources delete`  -  Remove a resource from a status page
- `betterstack-cli status-page-resources get`  -  Get a status page resource
- `betterstack-cli status-page-resources list`  -  List resources on a status page

**status-page-sections**  -  Sections within a status page

- `betterstack-cli status-page-sections create`  -  Create a status page section
- `betterstack-cli status-page-sections delete`  -  Delete a status page section
- `betterstack-cli status-page-sections get`  -  Get a status page section
- `betterstack-cli status-page-sections list`  -  List sections of a status page

**status-pages**  -  Public status pages

- `betterstack-cli status-pages create`  -  Create a status page
- `betterstack-cli status-pages delete`  -  Delete a status page
- `betterstack-cli status-pages get`  -  Get a status page by ID
- `betterstack-cli status-pages list`  -  List status pages


**coverage**


- `betterstack-cli coverage`  -  Find monitors with no escalation policy or no alert channel  -  the ones that will go down silently and page no one.


**down**


- `betterstack-cli down`  -  Focused triage list of monitors currently down or degraded, joined to their open incidents and whether anyone is actually paged for each.


**flapping**


- `betterstack-cli flapping`  -  Rank monitors by how many incidents they generated in a window to surface the noisy, flapping, or misconfigured ones.


**fleet**


- `betterstack-cli fleet`  -  One-screen health of the whole Better Stack account: monitors up/down/paused, heartbeats, open incidents, and who's on call now.


**group-health**


- `betterstack-cli group-health`  -  Per-group health rollup: monitor and heartbeat up/down counts plus open incidents for every monitor group and heartbeat group.


**heartbeat-risk**


- `betterstack-cli heartbeat-risk`  -  Rank heartbeats by risk: tight period+grace windows, paused-but-expected, and non-up status.


**mttr**


- `betterstack-cli mttr`  -  Mean time to acknowledge and resolve, computed across incidents over a window and broken down by monitor.


**oncall-gaps**


- `betterstack-cli oncall-gaps`  -  Detect on-call calendars with nobody currently on call.


**statuspage-audit**


- `betterstack-cli statuspage-audit`  -  Flags status pages showing operational while a backing monitor has an open incident, and status-page resources pointing at missing or paused monitors.


**triage**


- `betterstack-cli triage`  -  Open incidents ranked by age and acknowledgement state  -  never-acknowledged first  -  joined to the affected monitor.




## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `BETTERSTACK_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

- `betterstack-cli coverage`
- `betterstack-cli down`
- `betterstack-cli flapping`
- `betterstack-cli fleet`
- `betterstack-cli group-health`
- `betterstack-cli heartbeat-groups`
- `betterstack-cli heartbeat-groups get`
- `betterstack-cli heartbeat-groups list`
- `betterstack-cli heartbeat-risk`
- `betterstack-cli heartbeats`
- `betterstack-cli heartbeats get`
- `betterstack-cli heartbeats list`
- `betterstack-cli incidents`
- `betterstack-cli incidents get`
- `betterstack-cli incidents list`
- `betterstack-cli monitor-groups`
- `betterstack-cli monitor-groups get`
- `betterstack-cli monitor-groups list`
- `betterstack-cli monitors`
- `betterstack-cli monitors get`
- `betterstack-cli monitors list`
- `betterstack-cli mttr`
- `betterstack-cli on-calls`
- `betterstack-cli on-calls get`
- `betterstack-cli on-calls list`
- `betterstack-cli oncall-gaps`
- `betterstack-cli policies`
- `betterstack-cli policies get`
- `betterstack-cli policies list`
- `betterstack-cli status-pages`
- `betterstack-cli status-pages get`
- `betterstack-cli status-pages list`
- `betterstack-cli statuspage-audit`
- `betterstack-cli triage`

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
betterstack-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Find unprotected monitors

```bash
betterstack-cli coverage --agent
```

Lists monitors with no escalation policy or alert channel so you can fix paging gaps before an outage.

### 30-day incident response report

```bash
betterstack-cli mttr --days 30 --agent
```

Computes MTTA and MTTR across the window from the local mirror  -  no spreadsheet export needed.

### Narrow a verbose monitor payload

```bash
betterstack-cli monitors list --per-page 250 --agent --select data.id,data.attributes.pronounceable_name,data.attributes.status
```

Uses --select dotted paths to pull only id, name, and status from the JSON:API envelope so agents don't burn context on full monitor objects.

### Acknowledge an incident safely

```bash
betterstack-cli incidents acknowledge 12345 --by oncall@example.com --dry-run
```

Shows the exact request without sending; drop --dry-run to actually acknowledge.

### Rank the noisiest monitors

```bash
betterstack-cli flapping --days 7 --top 10 --agent
```

Surfaces the monitors generating the most incidents so you can tune alert thresholds.

## Auth Setup

Auth is a Better Stack Uptime API token (Authorization: Bearer). Create one under Better Stack → Settings → API tokens, then set BETTERSTACK_API_TOKEN. Run `betterstack-cli doctor` to confirm the token and reachability.

## Credential safety

- The bearer token (`Authorization: Bearer <token>`) and every API request go only to the scheme, host and port of the configured base URL (`https://uptime.betterstack.com/api/v2`, or `BETTERSTACK_BASE_URL`). A redirect to any other host is refused, so the token never reaches it.
- Any output, file, local store write or MCP result that would carry `BETTERSTACK_API_TOKEN`, the stored token, or a credential from a response (a monitor's `auth_username`, `auth_password` and `request_headers`, an escalation policy's `incident_token`), in any encoding, is withheld whole: the program prints "[credkit: withheld from stdout: it carried a credential this program holds]" (naming the sink) in its place. Leave those fields out with `--select` to print the rest. Responses are never cached on disk, and credential-named fields reach the local store as "[redacted]".
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, a tool that changes something in Better Stack (every create, update and delete tool, `incidents_acknowledge`, `incidents_resolve`, `import`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  betterstack-cli heartbeat-groups list --agent --select id,name,sort_index
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `BETTERSTACK_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `BETTERSTACK_CONFIG_DIR`, `BETTERSTACK_DATA_DIR`, `BETTERSTACK_STATE_DIR`, `BETTERSTACK_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `BETTERSTACK_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `betterstack-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "betterstack": {
        "command": "betterstack-mcp",
        "env": {
          "BETTERSTACK_HOME": "/srv/betterstack"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `BETTERSTACK_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `BETTERSTACK_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `BETTERSTACK_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `BETTERSTACK_DATA_SOURCE` sets the `--data-source` default.

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
betterstack-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "betterstack-cli learnings confirm 12"] }
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
       materially more, record the divergence via `betterstack-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `betterstack-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `betterstack-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
betterstack-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
betterstack-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
betterstack-cli teach-playbook \
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
betterstack-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`betterstack-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `BETTERSTACK_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
betterstack-cli feedback "the --since flag is inclusive but docs say exclusive"
betterstack-cli feedback --stdin < notes.txt
betterstack-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `BETTERSTACK_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `BETTERSTACK_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
betterstack-cli profile save briefing --json
betterstack-cli --profile briefing heartbeat-groups list
betterstack-cli profile list --json
betterstack-cli profile show briefing
betterstack-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `betterstack-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add betterstack-mcp -- betterstack-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `betterstack-mcp --transport http` requires `BETTERSTACK_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `BETTERSTACK_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `BETTERSTACK_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which betterstack-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   betterstack-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `betterstack-cli <command> --help`.
