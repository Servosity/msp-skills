---
name: datto-saas
description: "Every Datto SaaS Protection (Backupify) client in one local database: seat audits, protection gaps, backup health ranking, exit inventories and plan-first bulk seat changes. Trigger phrases: `audit datto saas seats`, `which backupify seats are unprotected`, `rank datto saas protection backup health`, `exit inventory for a client leaving backupify`, `plan a bulk seat license change`, `use datto-saas`, `run datto-saas`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Datto SaaS Protection"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - datto-saas-cli
    install:
      - kind: script
        bins: [datto-saas-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.ps1
---

# Datto SaaS Protection - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `datto-saas-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.ps1 | iex
   ```
3. Verify: `datto-saas-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `datto-saas-cli` and `datto-saas-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The Partner Portal shows one client at a time. This CLI syncs every client's domains, seats and backup history into SQLite so you can reconcile billing with seats reconcile, find unprotected seats with seats gaps, rank backups with backup-health, and hand a departing client a full exit-inventory. Seat changes are planned locally with seats bulk-plan; seats bulk-change is the one command that sends a change.

## When to Use This CLI

Use this CLI when an MSP needs answers across many Datto SaaS Protection clients at once: seat and billing reconciliation, unprotected or newly added seats, backup coverage ranking, or a full inventory of a client that is leaving. Run datto-saas-cli sync first; the audit commands read the local store and an unsynced store returns empty results. It is also the safe way to plan License, Pause and Unlicense changes in batches of at most 100 seats before sending them with seats bulk-change.

## Anti-triggers

Do not use this CLI for:
- Restoring or exporting mailbox, drive or site data: the public API has no restore or export endpoints; use the SaaS Protection portal
- Creating or deleting clients, domains or subscriptions: the API has no endpoints for it
- Datto BCDR appliances, agents or screenshots: use the datto-bcdr CLI
- Datto RMM devices or monitoring: use the datto-rmm CLI
- Changing Seat Management 1.0 Google seats: the API cannot; use the portal

## Unique Capabilities

### Safe seat changes
- **`seats bulk-plan`**  -  Turn a seat selection into ready-to-run License, Pause or Unlicense batches, grouped by subscription and seat type and capped at 100 ids, without sending anything.

  _Reach for this before any seat change: it previews which seats would move, skips seats already in the target state, blocks clients with several subscriptions until --subscription picks one, and prints the seats bulk-change command per batch. Check blocked and skipped before trusting an empty plan._

  ```bash
  datto-saas-cli seats bulk-plan --action License --state Unprotected --customer 123456 --subscription Classic:Office365:654321 --agent
  ```

### Fleet seat audit
- **`seats reconcile`**  -  See, per client, where Datto's seatsUsed disagrees with billable and Active seats before you invoice.

  _Use it for monthly invoice prep or a cost-leak sweep; it reports signed deltas and flags instead of raw seat lists._

  ```bash
  datto-saas-cli seats reconcile --agent
  ```
- **`seats gaps`**  -  List seats across all clients that are Unprotected (Paused too with --include-paused), newest first, with each seat's remoteId for a License plan.

  _Use it to answer who joined a tenant and is not covered, across the whole fleet in one call._

  ```bash
  datto-saas-cli seats gaps --since 30d --agent
  ```
- **`seats roster`**  -  Every synced seat across all clients in one table, joined with its customer's domains, product types and subscription ids; --csv for a fleet-wide export.

  _Reach for this for a fleet-wide seat export or to hand a client's seat list to billing; use seats summary for counts._

  ```bash
  datto-saas-cli seats roster --customer 123456 --state Unprotected,Paused --agent
  ```
- **`seats summary`**  -  Seat counts per client and seat type, split by Active, Paused, Archived and Unprotected state and the billable flag.

  _Reach for this for plain counts by type and state; use seats reconcile to compare counts with Datto's seatsUsed._

  ```bash
  datto-saas-cli seats summary --agent
  ```

### Backup health
- **`backup-health`**  -  Rank clients worst first by backup coverage, then consecutive short backup windows and perfect-backup rate.

  _Use it for the daily or weekly backup triage instead of opening each client's report._

  ```bash
  datto-saas-cli backup-health --below 95 --agent
  ```

### Client offboarding
- **`exit-inventory`**  -  Build one manifest for a client leaving SaaS Protection: domains, subscription ids, retention, every seat with remoteId, storage, last backups and a blockers checklist.

  _Use it to plan a migration or offboarding from facts and to confirm nothing is still un-backed-up before cancellation. Use --data-source live for a real offboarding: the local fallback has no detailed backup stats._

  ```bash
  datto-saas-cli exit-inventory 123456 --agent
  ```

## Command Reference

**activity-log**  -  Datto platform activity log (shared reporting endpoint).

- `datto-saas-cli activity-log`  -  List Datto platform activity log entries, filtered by client, user, target or time window.

**applications**  -  Per-customer backup history by suite and application.

- `datto-saas-cli applications <saasCustomerId>`  -  Show one customer's backup report: used bytes, backup history per suite and app, and optional remote IDs per seat type.

**backup-stats**  -  Per-customer detailed backup statistics.

- `datto-saas-cli backup-stats <saasCustomerId>`  -  Show detailed backup statistics for one SaaS customer.

**domains**  -  Protected Microsoft 365 tenants and Google Workspace domains.

- `datto-saas-cli domains`  -  List every protected Microsoft 365 tenant and Google Workspace domain, with its saasCustomerId and subscription id.

**seats**  -  Protected seats (users, shared mailboxes, sites, team sites, teams, shared drives).

- `datto-saas-cli seats bulk-change <saasCustomerId> <externalSubscriptionId>`  -  License, pause, or unlicense up to 100 seats of one subscription in one call. This is the only write: it changes which seats are protected and billed. It sends immediately: there is no confirmation prompt and `--yes` is not required. Preview with `--dry-run`, which applies the same seat-type, action and 100-id checks as a real send, or plan batches with `seats bulk-plan`.
- `datto-saas-cli seats list <saasCustomerId>`  -  List the seats of one SaaS customer with seat type, state, billable flag, date added and remoteId.


**backup-health**


- `datto-saas-cli backup-health`  -  Rank clients worst first by backup coverage, then consecutive short backup windows and perfect-backup rate.


**exit-inventory**


- `datto-saas-cli exit-inventory <customer>`  -  Build one manifest for a client leaving SaaS Protection: domains, subscription ids, retention, every seat with remoteId, storage, last backups and a blockers checklist.


**sync**

- `datto-saas-cli sync`  -  Pull every domain, then each client's seats and backup history, into the local store. Run it before the local commands below.

**seats (local audit and planning)**  -  Run `datto-saas-cli sync` first: these commands read the local store, and an unsynced store returns empty results, which is not an all-clear.

- `datto-saas-cli seats bulk-plan`  -  Turn a seat selection into ready-to-run License, Pause or Unlicense batches, grouped by subscription and seat type and capped at 100 ids, without sending anything.

- `datto-saas-cli seats reconcile`  -  See, per client, where Datto's seatsUsed disagrees with billable and Active seats before you invoice.

- `datto-saas-cli seats gaps`  -  List seats across all clients that are Unprotected (Paused too with --include-paused), newest first, with each seat's remoteId for a License plan.

- `datto-saas-cli seats roster`  -  Every synced seat across all clients in one table, joined with its customer's domains, product types and subscription ids; --csv for a fleet-wide export.

- `datto-saas-cli seats summary`  -  Seat counts per client and seat type, split by Active, Paused, Archived and Unprotected state and the billable flag.




## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `DATTO_SAAS_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

- `datto-saas-cli domains`

The seat, backup-health and exit-inventory commands are not covered: the pre-read refresh only re-fetches the domains list, not the per-client seats and backup history those commands read. Run `datto-saas-cli sync` to refresh them; they print a stale-data hint on stderr. The local-only commands (`seats roster`, `seats summary`, `seats reconcile`, `seats gaps`, `seats bulk-plan`, `backup-health`) reject `--data-source live`.

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
datto-saas-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Reconcile seats before invoicing

```bash
datto-saas-cli seats reconcile --agent
```

Per client, compares summed domain seatsUsed with billable and Active seats and flags mismatches.

### Narrow a large seat roster

```bash
datto-saas-cli seats roster --agent --select saas_customer_name,domains,name,seat_type,seat_state,remote_id
```

The fleet roster is wide; select only the columns an agent needs.

### Exit inventory for a departing client

```bash
datto-saas-cli exit-inventory 123456 --agent
```

One manifest with domains, subscription ids, seats, storage and a blockers checklist.

### Plan unlicensing archived seats

```bash
datto-saas-cli seats bulk-plan --action Unlicense --state Archived --customer 123456 --subscription Classic:Office365:654321 --agent
```

Groups archived seats into batches of at most 100 and prints the seats bulk-change command for each; sends nothing. Check blocked and skipped before trusting an empty plan. Unlicense stops protecting the seat, so confirm how long the archived data is retained before running the batches.

## Auth Setup

Datto SaaS Protection uses the Datto REST API key pair from the Partner Portal (Admin, Integrations, API Keys). Set DATTO_SAAS_PUBLIC_KEY to the public key and DATTO_SAAS_SECRET_KEY to the secret key; the CLI sends them as HTTP Basic auth to api.datto.com. One partner key sees every client you manage.

Run `datto-saas-cli doctor` to verify setup.

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
  datto-saas-cli activity-log --agent --select category,id,message
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **No confirmation prompt**  -  `seats bulk-change` is the only write and sends immediately; `--yes` is not required. Preview with `--dry-run` or plan with `seats bulk-plan`
- **No create endpoints**  -  `--idempotent` only turns an HTTP 409 into success, so `seats bulk-change` refuses it

### Response envelope

Under `--agent`, commands wrap output in a provenance envelope (with plain `--json`, the local audit commands print the bare array or object):

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `DATTO_SAAS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `DATTO_SAAS_CONFIG_DIR`, `DATTO_SAAS_DATA_DIR`, `DATTO_SAAS_STATE_DIR`, `DATTO_SAAS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `DATTO_SAAS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `datto-saas-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "datto-saas": {
        "command": "datto-saas-mcp",
        "env": {
          "DATTO_SAAS_HOME": "/srv/datto-saas"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `DATTO_SAAS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `DATTO_SAAS_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `DATTO_SAAS_NO_LOCAL_DATA=1` disables the response cache and writing API data to the store (the learning loop's own records still go there unless you add `--no-learn`), turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `DATTO_SAAS_DATA_SOURCE` sets the `--data-source` default.

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
datto-saas-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "datto-saas-cli learnings confirm 12"] }
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
       materially more, record the divergence via `datto-saas-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `datto-saas-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., an "o365" teach satisfying a "Microsoft 365" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., one alias that maps to two seat types). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `datto-saas-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
datto-saas-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "unlicensed") satisfies future queries under another alias (e.g., "not protected", "Unprotected") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
datto-saas-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
datto-saas-cli teach-playbook \
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
datto-saas-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between syncs, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-client / per-seat / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`datto-saas-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `DATTO_SAAS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
datto-saas-cli feedback "the --since flag is inclusive but docs say exclusive"
datto-saas-cli feedback --stdin < notes.txt
datto-saas-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `DATTO_SAAS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `DATTO_SAAS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
datto-saas-cli profile save briefing --json
datto-saas-cli --profile briefing activity-log
datto-saas-cli profile list --json
datto-saas-cli profile show briefing
datto-saas-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Other error (for example `--data-source live` on a local-only command) |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `datto-saas-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add datto-saas-mcp --env DATTO_SAAS_PUBLIC_KEY=<public-key> --env DATTO_SAAS_SECRET_KEY=<secret-key> -- datto-saas-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `datto-saas-mcp --transport http` requires `DATTO_SAAS_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `DATTO_SAAS_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `DATTO_SAAS_MCP_READ_ONLY=1` lists only read-only tools: `datto-saas_execute`, `sync`, `export`, the exit-inventory recipe and the learn-store tools (`teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`, `playbook_amend`, `learnings_confirm`, `learnings_forget`, `learnings_reject`, `workflow_archive`) are not registered.

Over MCP the API endpoints are reached through `datto-saas_search`, `datto-saas_read` and `datto-saas_execute`. The one write, `seats.bulk-change`, goes through `datto-saas_execute`: it returns a preview and sends only when `confirm` is boolean `true`.

## Direct Use

1. Check if installed: `which datto-saas-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   datto-saas-cli seats gaps --since 30d --agent
   ```
4. If ambiguous, drill into subcommand help, for example `datto-saas-cli seats --help`.
