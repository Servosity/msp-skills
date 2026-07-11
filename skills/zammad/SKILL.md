---
name: zammad
description: "Every Zammad ticket, article, and Knowledge Base operation as one CLI and MCP server  -  plus a team-management layer (agent load, customer health, aging backlog, escalation triage, churn risk, feedback mining) the Zammad API can't answer in a single call. Trigger phrases: `who is overloaded in support`, `which customers are at risk`, `what tickets are open too long`, `who should we escalate`, `what feedback are we getting`, `search zammad tickets`, `use zammad`, `run zammad`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Zammad"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - zammad-cli
    install:
      - kind: go
        bins: [zammad-cli]
        module: github.com/mvanhorn/printing-press-library/library/project-management/zammad/cmd/zammad-cli
---

# Zammad  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `zammad-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install zammad --cli-only
   ```
2. Verify: `zammad-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.5 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/project-management/zammad/cmd/zammad-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A single Go binary for the whole Zammad REST + Knowledge Base surface with an offline SQLite store, structured `--json`/`--select` output, and `--dry-run` on every write. On top of parity it adds a team-management layer computed from your synced tickets: `agent-load` and `agent-trend` for workload, `customer-health` and `churn-risk` for account risk, `overdue` for aging backlog, `escalate` for sentiment triage, and `feedback` for mining what customers ask for. Works against any Zammad instance  -  set your instance URL and token.

## When to Use This CLI

Use this CLI when an agent needs to read, search, or triage Zammad tickets and Knowledge Base content, or to answer team-management questions  -  who is overloaded, which customers are at risk, what is aging, who sounds like they might churn, and what feedback themes are recurring. It is ideal for support-ops automation, standups, and account-health reviews where the answer requires aggregating across many tickets.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to send customer-facing marketing email  -  it manages ticket articles, not campaigns.
- Do not use it to configure Zammad triggers, automations, SLAs, or channels  -  those are admin-console features not exposed here.
- Do not rely on escalate/churn/feedback as authoritative sentiment analysis  -  they are heuristic keyword triage that surface signals for a human or agent to judge.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Team management
- **`agent-load`**  -  See each agent's current ticket load broken out by state (open/pending/backlog) so you can balance work before it piles up.

  _Reach for this when asked who is overloaded or how work is distributed across the support team right now._

  ```bash
  zammad-cli agent-load --json
  ```
- **`agent-trend`**  -  Show whether each agent's queue is growing or shrinking across the last N weeks, with opened-vs-closed deltas.

  _Reach for this when asked if the team is keeping up or falling behind over time._

  ```bash
  zammad-cli agent-trend --weeks 2 --json
  ```
- **`customer-health`**  -  Rank organizations by a health signal built from open count, oldest-open age, reopen rate, velocity, and last activity.

  _Reach for this to spot which customers are struggling before they complain._

  ```bash
  zammad-cli customer-health --at-risk --json
  ```
- **`overdue`**  -  Find aging tickets still in new/open/pending past a threshold, weighted by priority so the worst rise to the top.

  _Reach for this to catch tickets about to breach SLA or go stale._

  ```bash
  zammad-cli overdue --days 3 --json
  ```
- **`escalate`**  -  Surface active tickets whose recent customer messages read as upset (heuristic negative-signal scan), ranked by hits, age, and priority  -  with the matched snippets shown.

  _Reach for this to find customers who sound angry and should be escalated; it is keyword triage with snippets, so you make the final call._

  ```bash
  zammad-cli escalate --json
  ```
- **`churn-risk`**  -  Score each organization's churn risk from open backlog, overdue tickets, unanswered pending, and negative-sentiment customer messages, listing the contributing factors.

  _Reach for this to get ahead of churn; it surfaces the factors so you can judge and act._

  ```bash
  zammad-cli churn-risk --json
  ```
- **`feedback-scan`**  -  Bucket ticket and article text into feature-request, pricing, compliance, and bug themes with the source ticket refs and snippets.

  _Reach for this to mine what customers keep asking for across features, pricing, and compliance._

  ```bash
  zammad-cli feedback-scan --bucket pricing --json
  ```

### Knowledge Base
- **`kb browse`**  -  Print the Knowledge Base as a category/answer tree parsed from the init bundle.

  _Reach for this to see the KB structure before reading or searching answers._

  ```bash
  zammad-cli kb browse
  ```
- **`kb search`**  -  Text search over KB answer titles and bodies from the live KB init bundle.

  _Reach for this to find the right KB answer by keyword._

  ```bash
  zammad-cli kb search "restore" --json
  ```
- **`kb get`**  -  Resolve a KB answer id from the init bundle to its full translated body.

  _Reach for this to read a KB answer's full content._

  ```bash
  zammad-cli kb get 42
  ```

### Ticket ergonomics
- **`ticket note`**  -  Add an internal or partner-visible note to a ticket in one line, with correct content-type defaults.

  _Reach for this to log work on a ticket without hand-building the article payload._

  ```bash
  zammad-cli ticket note 12345 --body "Investigated, awaiting logs" --internal --dry-run
  ```

## Command Reference

**articles**  -  Read and add ticket articles (notes and emails)

- `zammad-cli articles by-ticket`  -  List all articles for a ticket
- `zammad-cli articles create`  -  Add an article (note or email) to a ticket
- `zammad-cli articles get`  -  Get a single article by id

**groups**  -  Ticket routing groups

- `zammad-cli groups get`  -  Get a group by id
- `zammad-cli groups list`  -  List all groups

**kb**  -  Knowledge Base categories and answers

- `zammad-cli kb answer-create`  -  Create a KB answer. Pass translations_attributes as JSON, then publish or mark internal.
- `zammad-cli kb answer-delete`  -  Delete a KB answer (requires KB editor permission)
- `zammad-cli kb answer-internal`  -  Mark a KB answer internal (agent-only)
- `zammad-cli kb answer-publish`  -  Publish a KB answer (make it public)
- `zammad-cli kb category-create`  -  Create a KB category. Pass translations_attributes as JSON.
- `zammad-cli kb init`  -  Fetch the full Knowledge Base bundle (categories, answers, translations). Used by 'kb browse/search/get'.

**organizations**  -  Read and manage organizations (customers / partners)

- `zammad-cli organizations create`  -  Create an organization
- `zammad-cli organizations get`  -  Get an organization by id
- `zammad-cli organizations list`  -  List organizations (paginated)
- `zammad-cli organizations search`  -  Search organizations by name or custom field
- `zammad-cli organizations update`  -  Update an organization

**overviews**  -  Agent ticket overviews (saved views)

- `zammad-cli overviews get`  -  Get an overview (and its tickets) by id
- `zammad-cli overviews list`  -  List all overviews

**priorities**  -  Ticket priority reference (id -> name map)

- `zammad-cli priorities`  -  List ticket priorities

**states**  -  Ticket state reference (id -> name map)

- `zammad-cli states`  -  List ticket states

**tags**  -  Ticket tags

- `zammad-cli tags add`  -  Add a tag to an object
- `zammad-cli tags all`  -  List every tag defined in the instance (vocabulary)
- `zammad-cli tags list`  -  List tags on an object (defaults to a ticket)
- `zammad-cli tags remove`  -  Remove a tag from an object

**tickets**  -  Read, search, create, and change Zammad tickets

- `zammad-cli tickets create`  -  Create a ticket. Provide title, group, a customer, and an initial article.
- `zammad-cli tickets delete`  -  Delete a ticket permanently (requires admin / delete permission)
- `zammad-cli tickets get`  -  Get a ticket by id
- `zammad-cli tickets list`  -  List tickets (paginated)
- `zammad-cli tickets search`  -  Search tickets with Zammad query syntax (e.g. 'state:open owner_id:3')
- `zammad-cli tickets update`  -  Update a ticket (state, priority, owner, title, organization)

**users**  -  Read and manage users (agents and customers)

- `zammad-cli users create`  -  Create a user
- `zammad-cli users get`  -  Get a user by id
- `zammad-cli users list`  -  List users (paginated)
- `zammad-cli users me`  -  Get the authenticated user (auth self-check)
- `zammad-cli users search`  -  Search users by email, name, or query (e.g. organization_id:5)
- `zammad-cli users update`  -  Update a user


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
zammad-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Balance the queue at standup

```bash
zammad-cli agent-load --json --select agent,open,pending,backlog
```

One row per agent with open, pending, and total backlog counts.

### Find at-risk accounts

```bash
zammad-cli customer-health --at-risk --json --select organization,open,oldest_open_days,last_activity_days,signal
```

Organizations flagged at-risk with the health factors that triggered the flag, using --select to keep the payload small on a deeply aggregated response.

### Triage who to escalate

```bash
zammad-cli escalate --json
```

Ranked tickets with matched negative-signal snippets so you judge true sentiment.

### Mine pricing feedback

```bash
zammad-cli feedback-scan --bucket pricing --json
```

Tickets and article snippets bucketed as pricing feedback with source refs.

### Read the KB before answering

```bash
zammad-cli kb search "backup restore" --json
```

Keyword search over Knowledge Base answers (from the live KB init bundle).

## Auth Setup

Zammad uses a personal access token sent as `Authorization: Token token=<TOKEN>`. Create one in your Zammad instance under Profile → Token Access, then set `ZAMMAD_API_TOKEN`. Point the CLI at your instance with `ZAMMAD_URL` (e.g. https://support.example.com). The token's permissions cap what the CLI can do; a token without write scopes keeps every command read-only regardless of flags.

Run `zammad-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  zammad-cli articles get 12345 --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
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

- Use `--home <dir>` for one invocation, or set `ZAMMAD_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `ZAMMAD_CONFIG_DIR`, `ZAMMAD_DATA_DIR`, `ZAMMAD_STATE_DIR`, `ZAMMAD_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `ZAMMAD_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `zammad-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "zammad": {
        "command": "zammad-mcp",
        "env": {
          "ZAMMAD_HOME": "/srv/zammad"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `ZAMMAD_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `ZAMMAD_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, run:

```bash
zammad-cli recall "<user's question>" --agent
```

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
      "next_action": ["<trial command>", "zammad-cli learnings confirm 12"] }
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
       materially more, record the divergence via `zammad-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `zammad-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `zammad-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately:

```bash
zammad-cli teach --query "<user's question>" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
zammad-cli teach \
  --query "<user's question>" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
zammad-cli teach-playbook \
  --query "<user's question>" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`.

```bash
zammad-cli playbook amend \
  --query "<exact recall query string>" \
  --add-note "<your concrete correction>"
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

`zammad-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `ZAMMAD_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
zammad-cli feedback "the --since flag is inclusive but docs say exclusive"
zammad-cli feedback --stdin < notes.txt
zammad-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `ZAMMAD_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ZAMMAD_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
zammad-cli profile save briefing --json
zammad-cli --profile briefing articles get 12345
zammad-cli profile list --json
zammad-cli profile show briefing
zammad-cli profile delete briefing --yes
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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `zammad-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/project-management/zammad/cmd/zammad-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add zammad-mcp -- zammad-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which zammad-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   zammad-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `zammad-cli <command> --help`.
