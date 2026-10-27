---
name: cove
description: "Fleet-wide Cove backup health, billing usage and storage trends, with local history. Trigger phrases: `which backups failed last night`, `check cove backup status`, `stale cove devices`, `cove storage growth`, `cove billing usage report`, `use cove`, `run cove-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Cove Data Protection"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - cove-cli
    install:
      - kind: script
        bins: [cove-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.ps1
---

# Cove Data Protection - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `cove-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.ps1 | iex
   ```
3. Verify: `cove-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `cove-cli` and `cove-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Cove's console scopes to one customer at a time and forgets yesterday. cove-cli speaks the whole JSON-RPC API: one command sweeps every partner for failed or stale backups with column codes decoded to human names, `snapshot` keeps timestamped SQLite history so `storage growth` and `devices changes` can answer trend questions nothing else can, and `call` reaches all 251 documented methods.

## When to Use This CLI

Use cove-cli when a task touches N-able Cove Data Protection fleet state: which backups failed or went stale, per-customer health rollups, storage growth, month-end SKU/seat billing, partner/device/user/audit enumeration, or any of the 251 JSON-RPC methods via `call`. It is the right tool for cross-customer questions the backup.management console can only answer one partner at a time, and for trend questions that need the local snapshot history.

## Anti-triggers

Do not use this CLI for:
- Restoring files or browsing backed-up data  -  restores run through the Backup Manager client or console, and per-device session detail lives on storage-node Reporting Service endpoints this CLI does not cover
- Installing or controlling the Backup Manager agent on endpoints  -  use deployment tooling (RMM) for that
- N-able N-central or RMM administration  -  this CLI only speaks the Cove (backup.management) API
- Accounts that only have interactive 2FA logins  -  the JSON-RPC API needs an API-enabled service account

## Unique Capabilities

### Local history that compounds
- **`storage growth`**  -  See which devices and customers are growing their backup storage fastest, from timestamped local snapshots.

  _Reach for this when capacity planning or quota questions need a trend, not a point-in-time number._

  ```bash
  cove-cli storage growth --since 7d --agent
  ```
- **`devices changes`**  -  Devices whose backup status flipped between the two latest snapshots of the same partner tree: regressions and recoveries without re-sweeping the fleet.

  _Answers 'what broke since last week' and 'did Friday's failures clear' in one command._

  ```bash
  cove-cli devices changes --since 7d --json
  ```

### Fleet triage
- **`devices failures`**  -  Every device across all customers whose last backup session failed, aborted, errored, had no selection, never started or reported an unrecognised status, decoded to status names.

  _The MSP morning ritual: run this first to build the day's ticket queue._

  ```bash
  cove-cli devices failures --since 24h --agent
  ```
- **`devices stale`**  -  Devices with no successful backup in N days, ranked by staleness and grouped by customer.

  _Catches silent gaps: a device can report a fine last status while not having succeeded for days._

  ```bash
  cove-cli devices stale --days 3 --json
  ```
- **`fleet health`**  -  One-screen rollup: total devices, healthy, failed, stale, never-run and unknown status, with a per-customer breakdown.

  _The at-a-glance number for standups and QBR prep._

  ```bash
  cove-cli fleet health --by partner --json
  ```
- **`devices sources`**  -  Which data sources each device protects (Files, System State, SQL, Hyper-V, Microsoft 365), decoded fleet-wide, with a filter for devices missing one.

  _Reach for this when a restore plan assumes a data source is protected and you need to prove it across every customer._

  ```bash
  cove-cli devices sources --without "System State" --os server --agent
  ```
- **`fleet activity`**  -  Who did what in the Cove console: audit-trail actions grouped by user and operation across every sub-partner, with a breakdown by result.

  _Reach for this for a weekly review of console activity or after an incident, to see which accounts changed what._

  ```bash
  cove-cli fleet activity --since 7d --agent
  ```

### Billing
- **`billing usage`**  -  Per-device SKU, used storage, and M365 seat counts with column codes decoded  -  the month-end billing export in one command.

  _Turns Cove usage into invoice lines without the column-code legend open in another tab._

  ```bash
  cove-cli billing usage --csv
  ```
- **`billing changes`**  -  Devices whose plan changed since last month  -  current SKU vs Cove's built-in previous-month SKU column.

  _Run at month-end to catch plan upgrades and downgrades that affect invoices._

  ```bash
  cove-cli billing changes --json
  ```

## Command Reference

**audit**  -  Management console audit trail

- `cove-cli audit`  -  Enumerate audit actions in a time range (JSON-RPC EnumerateAuditActions)

**backup-jobs**  -  Backup/restore jobs

- `cove-cli backup-jobs`  -  Enumerate jobs for a partner (JSON-RPC EnumerateJobs)

**columns**  -  Statistics column metadata (pairs with the README column-code legend)

- `cove-cli columns`  -  Enumerate custom statistic columns visible to a partner (JSON-RPC EnumerateColumns)

**devices**  -  Backup devices (accounts) and their column-coded statistics

- `cove-cli devices get`  -  Get one device by id (JSON-RPC GetAccountInfoById)
- `cove-cli devices list`  -  Enumerate backup devices for a partner (JSON-RPC EnumerateAccounts)
- `cove-cli devices stats`  -  Query device statistics by column codes (JSON-RPC EnumerateAccountStatistics; see README column-code legend)

**labels**  -  Device labels

- `cove-cli labels`  -  Enumerate all device labels (JSON-RPC EnumerateAllLabels)

**locations**  -  Data-center locations

- `cove-cli locations`  -  Enumerate data-center locations (JSON-RPC EnumerateLocations)

**partners**  -  Customers and resellers in the partner hierarchy

- `cove-cli partners get`  -  Get one partner by id (JSON-RPC GetPartnerInfoById)
- `cove-cli partners list`  -  Enumerate partners under a parent (JSON-RPC EnumeratePartners)

**products**  -  Backup products available to a partner

- `cove-cli products`  -  Enumerate products (JSON-RPC EnumerateProducts)

**server**  -  Management service metadata

- `cove-cli server`  -  Get management server info (JSON-RPC GetServerInfo)

**storage**  -  Storage pools, statistics, and nodes

- `cove-cli storage list`  -  Enumerate storage pools for a partner (JSON-RPC EnumerateStorages)
- `cove-cli storage nodes`  -  Enumerate all storage nodes (JSON-RPC EnumerateAllStorageNodes)
- `cove-cli storage stats`  -  Storage usage statistics per location (JSON-RPC EnumerateStorageStatistics)

**users**  -  Console users

- `cove-cli users`  -  Enumerate users for partners (JSON-RPC EnumerateUsers)


**billing**


- `cove-cli billing usage`  -  Per-device SKU, used storage, and M365 seat counts with column codes decoded  -  the month-end billing export in one command.

- `cove-cli billing changes`  -  Devices whose plan changed since last month  -  current SKU vs Cove's built-in previous-month SKU column.


**devices**


- `cove-cli devices failures`  -  Every device across all customers whose last backup session failed, aborted, errored, or never started  -  decoded to status names.

- `cove-cli devices stale`  -  Devices with no successful backup in N days, ranked by staleness and grouped by customer.

- `cove-cli devices changes`  -  Devices whose backup status flipped between the two latest snapshots  -  regressions and recoveries without re-sweeping the fleet.

- `cove-cli devices sources`  -  Which data sources each device actually protects (Files and Folders, System State, MS SQL, Hyper-V, Microsoft 365 and more), decoded from Cove's active-sources column, with a fleet tally and a filter for devices missing a given source.


**fleet**


- `cove-cli fleet health`  -  One-screen rollup: total devices, healthy, failed, stale, never-run  -  with a per-customer breakdown.

- `cove-cli fleet activity`  -  Who did what in the Cove console over a window: audit-trail actions grouped by user and operation across every sub-partner, with non-successful results counted.


**storage**


- `cove-cli storage growth`  -  See which devices and customers are growing their backup storage fastest, from timestamped local snapshots.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
cove-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning failure sweep for the ticket queue

```bash
cove-cli devices failures --since 24h --agent --select items.device_name,items.customer,items.status_name
```

Cross-customer failed/aborted/not-started list narrowed to the three fields a triage ticket needs.

### Find silently stale devices

```bash
cove-cli devices stale --days 3 --json
```

Devices with no successful session in 3 days, ranked worst-first  -  catches gaps the last-status view hides.

### Month-end billing export

```bash
cove-cli billing usage --csv
```

Per-device SKU, used storage, and M365 seats as CSV for the invoice model.

### What broke since the last snapshot

```bash
cove-cli devices changes --since 7d --json
```

After two `cove-cli snapshot` runs some time apart: regressions and recoveries between them.

### Call any of the 251 JSON-RPC methods

```bash
cove-cli call GetServerInfo --json
```

Generic escape hatch with automatic visa injection and typed vendor error mapping.

## Auth Setup

Cove's JSON-RPC API authenticates with a partner-scoped login, not a bearer token or API key. Create a dedicated **API User** in the Cove Management Console (**Users > API Users**); it issues a login name and an API token (shown only once). Set `COVE_USERNAME` to the API user's login name, `COVE_PASSWORD` to the API token, and `COVE_PARTNER` to the customer/partner the API user was created for - for an API User `COVE_PARTNER` is **required**, not optional, and an empty partner is the usual cause of a `2100 "Unknown partner/username"` error. `COVE_PARTNER` must be the **full customer/partner string exactly as shown in the Cove console customer dropdown** - usually including the account email in parentheses, e.g. `Acme Corp (admin@example.com)`, **not** just the company name; Cove returns the same `2100 "Unknown partner/username or bad password"` error for a wrong `COVE_PARTNER` format as for a bad password. The API token is the *password*; it is not itself a visa and is never sent as a header (passing it to `--visa` fails by design). N-able removed the older per-user "API access" checkbox, and API Users cannot sign in to the console. Run `cove-cli auth login` once: it calls the `Login` method with these three values, receives a session token (the "visa"), and caches it locally with auto-refresh on expiry. Hand-built commands (failures, stale, health, sources, activity, billing, snapshot, call) inject the visa automatically. The raw generated endpoint commands (partners/devices/storage `list` and `get`, and the promoted single-endpoint commands) do **not** auto-inject it: pass `--visa "$(cove-cli auth token)"`, or prefer `cove-cli call <Method>`, which injects the visa for you (it sends only Get and Enumerate methods unless you pass `--yes`).

Run `cove-cli doctor` to verify setup.

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
  cove-cli audit --agent --select PartnerName,UserName,EntityName
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Read-only**  -  do not use this CLI for create, update, delete, publish, comment, upvote, invite, order, send, or other mutating requests

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

- Use `--home <dir>` for one invocation, or set `COVE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `COVE_CONFIG_DIR`, `COVE_DATA_DIR`, `COVE_STATE_DIR`, `COVE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `COVE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains durable local data such as `data.db`. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Run `cove-cli doctor --fail-on warn` to surface path warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "cove": {
        "command": "cove-mcp",
        "env": {
          "COVE_HOME": "/srv/cove"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `COVE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `COVE_HOME`, or `doctor` will not find files left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `COVE_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `COVE_DATA_SOURCE` sets the `--data-source` default.

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
cove-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "cove-cli learnings confirm 12"] }
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
       materially more, record the divergence via `cove-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `cove-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
cove-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
cove-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
cove-cli teach-playbook \
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
cove-cli playbook amend \
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

`cove-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `COVE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
cove-cli feedback "the --since flag is inclusive but docs say exclusive"
cove-cli feedback --stdin < notes.txt
cove-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `COVE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `COVE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
cove-cli profile save briefing --json
cove-cli --profile briefing audit
cove-cli profile list --json
cove-cli profile show briefing
cove-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 5 | API error (upstream issue) |
| 7 | Rate limited or blocked by a firewall or bot challenge (back off; do not retry immediately) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `cove-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add cove-mcp -- cove-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `cove-mcp --transport http` requires `COVE_MCP_HTTP_TOKEN` (callers send `Authorization: Bearer <token>`), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. `COVE_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which cove-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   cove-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `cove-cli <command> --help`.
