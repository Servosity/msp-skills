---
name: quotewerks
description: "Use when the user asks to find, read, build, audit or reprice QuoteWerks Web quotes, orders or invoices, add a product to a quote by part number, re-quote an earlier order with current prices, or answer margin, win-rate, open-pipeline and distributor cost-drift questions across QuoteWerks documents. Typed writes preview first and need --confirm. Trigger phrases: `find the Acme quote in QuoteWerks`, `reprice the quote lines`, `which QuoteWerks quotes are stale`, `quotewerks margin by rep`, `re-quote an earlier order`, `use quotewerks`, `run quotewerks`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "QuoteWerks"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - quotewerks-cli
    install:
      - kind: script
        bins: [quotewerks-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.ps1
---

# QuoteWerks Web Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `quotewerks-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.ps1 | iex
   ```
3. Verify: `quotewerks-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `quotewerks-cli` and `quotewerks-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The QuoteWerks Web REST API is generic table access, so agents guess column names and quietly build bad quotes. This CLI ships typed commands for finding documents, adding and repricing lines from your product databases, and re-quoting; each previews by default and writes only with --confirm. A local mirror adds margin, win-rate, pipeline-aging and cost-drift reports no single API call answers. Column names come from the vendor's documentation and the REST API is in beta.

## When to Use This CLI

Use this CLI when an agent needs to look up, build, audit or reprice QuoteWerks Web quotes, orders or invoices, or answer margin, win-rate and pipeline questions across documents. Typed writes (docs create, items add, items reprice, requote) preview by default and write only with --confirm and a write-enabled key; --yes does not confirm them. The raw 'tables insert', 'tables update', 'tables delete' and 'import' commands write immediately, so run them with --dry-run first. margin, winrate, pipeline, cost-drift and changes read the local copy made by 'mirror' (not 'sync') and return empty results until it has run. Use --json rather than --agent for write previews, because --agent compacts away the request body.

## Anti-triggers

Do not use this CLI for:
- QuoteWerks Desktop data (on-premises database or the desktop COM API); this CLI only talks to QuoteWerks Web cloud tenants
- Sending quotes to customers or driving QuoteValet emails
- Bulk BI exports into Power BI or Tableau; use QuoteWerks Web's OData v4 feed
- Payments, purchasing or vendor RFQ workflows

## Unique Capabilities

These go beyond the raw REST API and the Zapier QuoteWerks app: typed commands with the real column names, previewed writes, and analysis of a local mirror.

### Typed quote tools
- **`docs search`**  -  Find quotes, orders and invoices by customer, number, status, rep, type or date without guessing QuoteWerks column names.

  _Reach for this before a raw 'tables query' on DocumentHeaders; it knows the real column names and catches silently ignored filters._

  ```bash
  quotewerks-cli docs search --customer "Acme" --type QUOTE --agent
  ```
- **`docs get`**  -  Get one quote, order or invoice plus every line item, with line types decoded and per-line margin.

  _Use this whenever you need the full contents of one document; it replaces a header fetch plus a hand-built item filter._

  ```bash
  quotewerks-cli docs get 1042 --agent
  ```
- **`items add`**  -  Add a line to a quote by part number, with price, cost, list and description filled in from your product databases (the lowest-cost match wins; an unmatched part is flagged, not priced).

  _Use this to add a product line safely; it previews by default and writes only with --confirm and a write-enabled API key._

  ```bash
  quotewerks-cli items add 1042 --part 20UES0X100 --qty 12 --json
  ```
- **`items reprice`**  -  See old versus new cost, price and margin for quote lines before changing them; lines with no known cost are skipped rather than priced at zero.

  _Use this when a distributor cost moved or a customer asks for a better price; nothing changes until --confirm (write-enabled API key)._

  ```bash
  quotewerks-cli items reprice 1042 --part 20UES0X100 --margin 18 --json
  ```
- **`docs create`**  -  Start a new quote from typed flags; it previews by default, and with --confirm (API key with write access) returns the new RecGUID and quote number.

  _Use this to start an empty quote; follow with 'items add' for lines._

  ```bash
  quotewerks-cli docs create --customer "Acme Dental" --name "Q4 laptop refresh" --json
  ```
- **`products search`**  -  Search parts across all of your QuoteWerks product databases at once.

  _Use this to find a part number, price or cost before quoting._

  ```bash
  quotewerks-cli products search "ThinkPad" --limit 20 --json
  ```
- **`requote`**  -  Copy an earlier quote or order into a new quote with today's product-database prices and new quantities.

  _Use this for 'same as last time' requests once you have the earlier document number (find it with docs search); it previews the new quote and writes only with --confirm._

  ```bash
  quotewerks-cli requote 1042 --qty 20UES0X100=12 --json
  ```

### Safe writes and audits
- **`docs lint`**  -  Catch price-below-cost, zero-cost, bad-quantity and low-margin lines on a quote before the customer sees it; add --strict to exit 1 when errors are found.

  _Run this on any quote an integration or agent built before it is sent._

  ```bash
  quotewerks-cli docs lint 1042 --min-margin 10 --json
  ```

### Pipeline and margin intelligence
- **`mirror`**  -  Copy quotes, line items and product databases into a local database; margin, winrate, pipeline, cost-drift and changes read this copy.

  _Run this before margin, winrate, pipeline, cost-drift or changes; they read the mirror._

  ```bash
  quotewerks-cli mirror --since 180d --json
  ```
- **`margin`**  -  See profit and margin percent by customer, rep, month or manufacturer from the local mirror (orders by default; run mirror first).

  _Use this for QBR and weekly sales-review margin questions._

  ```bash
  quotewerks-cli margin --by rep --since 90d --json
  ```
- **`pipeline`**  -  See open quote count and value by age bucket, or list the stale quotes to chase, from the local mirror (run mirror first).

  _Use this for the Monday pipeline review and follow-up list._

  ```bash
  quotewerks-cli pipeline --older-than 30d --json
  ```
- **`cost-drift`**  -  Find open-quote lines whose quoted cost no longer matches your product database, from the local mirror (run mirror first).

  _Run this after a distributor price update to see which quotes went thin._

  ```bash
  quotewerks-cli cost-drift --threshold 2 --json
  ```
- **`winrate`**  -  See won, lost and open quotes and the win rate by rep, customer or month from the local mirror; QuoteValet only records online acceptance, so pass --won-status for deals won offline.

  _Use this for sales-review and QBR win-rate questions._

  ```bash
  quotewerks-cli winrate --by rep --since 90d --json
  ```
- **`changes`**  -  See which documents were created, accepted, lost, re-statused, re-valued or removed between two mirror runs (the first mirror only records a baseline).

  _Use this for a 'what moved this week' pass over quotes._

  ```bash
  quotewerks-cli changes --since 7d --json
  ```

## Command Reference

**agent**  -  Manage agent

- `quotewerks-cli agent ask`  -  Ask the QuoteWerks Web AI agent a natural-language question about your data
- `quotewerks-cli agent report-draft`  -  One turn of report drafting, previewed as `run_as` with that user's document rights.
- `quotewerks-cli agent report-eligibility`  -  Which of the tenant's reports the assistant can open -- the exact check /load makes, run over all of them
- `quotewerks-cli agent report-load`  -  An existing report as the starting draft of a conversation -- previewed, no model call.
- `quotewerks-cli agent report-preview`  -  Re-run a draft -- typically with new prompt answers from the form. No model call.

**cloud_settings**  -  Manage cloud settings

- `quotewerks-cli cloud-settings get`  -  Get a single resource by ID.
- `quotewerks-cli cloud-settings list`  -  List resources with pagination, filtering, and sorting.
- `quotewerks-cli cloud-settings schema`  -  Get the schema (columns) for this resource.

**identity**  -  Manage identity

- `quotewerks-cli identity health`  -  Health check endpoint for the authentication service. Returns: 200 OK if the service is running
- `quotewerks-cli identity userinfo`  -  Show the tenant account number and user name for the credential in use (works with the X-API-Key header).

**reports**  -  Manage reports

- `quotewerks-cli reports run`  -  Executes a saved report and returns its rendered content.
- `quotewerks-cli reports schedulability`  -  Reports whether a definition can run without a person present, and why not if it cannot.

**tables**  -  Manage tables

- `quotewerks-cli tables delete`  -  Delete a row by primary key. Requires API key with AllowWrite permission.
- `quotewerks-cli tables insert`  -  Create a new row in a table. Requires API key with AllowWrite permission.
- `quotewerks-cli tables list`  -  List all available tables in the tenant database.
- `quotewerks-cli tables query`  -  Search a table with the filter document in the request body.
- `quotewerks-cli tables row`  -  Get a single row by primary key. Returns JSON:API format with optional relationship sideloading.
- `quotewerks-cli tables rows`  -  List rows from a table. Returns JSON:API format with pagination links and optional relationship sideloading.
- `quotewerks-cli tables schema`  -  Get the schema (columns) for a specific table, including write metadata.
- `quotewerks-cli tables update`  -  Update a row by primary key. Requires API key with AllowWrite permission.
- `quotewerks-cli tables values`  -  List the distinct values of one column in a table


**changes**


- `quotewerks-cli changes`  -  See which documents were created, accepted, lost, re-statused, re-valued or removed between two mirror runs (the first mirror only records a baseline).


**cost-drift**


- `quotewerks-cli cost-drift`  -  Find open-quote lines whose quoted cost no longer matches your product database, from the local mirror (run mirror first).


**docs**


- `quotewerks-cli docs search`  -  Find quotes, orders and invoices by customer, number, status, rep, type or date without guessing QuoteWerks column names.

- `quotewerks-cli docs get`  -  Get one quote, order or invoice plus every line item, with line types decoded and per-line margin.

- `quotewerks-cli docs create`  -  Start a new quote from typed flags; it previews by default, and with --confirm (API key with write access) returns the new RecGUID and quote number.

- `quotewerks-cli docs lint`  -  Catch price-below-cost, zero-cost, bad-quantity and low-margin lines on a quote before the customer sees it; add --strict to exit 1 when errors are found.


**items**


- `quotewerks-cli items list`  -  Find quote lines by document, part number or description
- `quotewerks-cli items add`  -  Add a line to a quote by part number, with price, cost, list and description filled in from your product databases (the lowest-cost match wins; an unmatched part is flagged, not priced).

- `quotewerks-cli items reprice`  -  See old versus new cost, price and margin for quote lines before changing them; lines with no known cost are skipped rather than priced at zero.


**margin**


- `quotewerks-cli margin`  -  See profit and margin percent by customer, rep, month or manufacturer from the local mirror (orders by default; run mirror first).


**mirror**


- `quotewerks-cli mirror`  -  Copy quotes, line items and product databases into a local database; margin, winrate, pipeline, cost-drift and changes read this copy.


**pipeline**


- `quotewerks-cli pipeline`  -  See open quote count and value by age bucket, or list the stale quotes to chase, from the local mirror (run mirror first).


**products**


- `quotewerks-cli products search`  -  Search parts across all of your QuoteWerks product databases at once.


**requote**


- `quotewerks-cli requote`  -  Copy an earlier quote or order into a new quote with today's product-database prices and new quantities.


**winrate**


- `quotewerks-cli winrate`  -  See won, lost and open quotes and the win rate by rep, customer or month from the local mirror; QuoteValet only records online acceptance, so pass --won-status for deals won offline.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
quotewerks-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Read one quote's lines compactly

```bash
quotewerks-cli docs get 1042 --agent --select header.DocNo,header.GrandTotal,lines.ManufacturerPartNumber,lines.UnitPrice
```

Narrows a large header and line payload to the fields an agent needs.

### Re-quote last order with new quantities

```bash
quotewerks-cli requote 1042 --qty 20UES0X100=12 --json
```

Previews a new quote copied from document 1042 with current product-database prices; add --confirm to create it.

### Reprice a line to an 18% margin

```bash
quotewerks-cli items reprice 1042 --part 20UES0X100 --margin 18 --json
```

Shows the before and after cost, price and margin; add --confirm to apply.

### Find quotes hurt by a distributor price change

```bash
quotewerks-cli cost-drift --threshold 2 --json
```

Lists open-quote lines whose quoted cost moved at least 2% from the product database (reads the local mirror; run mirror first).

### Audit an integration-built quote

```bash
quotewerks-cli docs lint 1042 --min-margin 10 --json
```

Flags price-below-cost, zero-cost and low-margin lines and exits nonzero on errors.

## Auth Setup

Create an API key in QuoteWerks Web under Setup > Options > Zapier and API (the API requires MasterRights) and export it as QUOTEWERKS_API_KEY, or save it with 'echo "$QUOTEWERKS_API_KEY" | quotewerks-cli auth set-token' (the key is read from stdin). It is sent as the X-API-Key header. Read commands work with any key; creating or changing documents needs a key with 'Allow write access' (AllowWrite), otherwise QuoteWerks returns 403. QuoteWerks Web (cloud) tenants only; the REST API is in beta.

Run `quotewerks-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  quotewerks-cli cloud-settings list --agent --select id
  ```
- **Previewable**  -  `--dry-run` shows the HTTP request without sending on generated endpoint commands (`tables`, `cloud-settings`, `agent`, `reports`); on the typed commands it only confirms the command would run, and the typed write commands already preview by default
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  typed writes (`docs create`, `items add`, `items reprice`, `requote`) preview by default and write only with `--confirm` (needs an API key with write access); `--yes` does not confirm them. The raw `tables insert`, `tables update`, `tables delete` and `import` commands write immediately, so run them with `--dry-run` first
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

- Use `--home <dir>` for one invocation, or set `QUOTEWERKS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `QUOTEWERKS_CONFIG_DIR`, `QUOTEWERKS_DATA_DIR`, `QUOTEWERKS_STATE_DIR`, `QUOTEWERKS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `QUOTEWERKS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `quotewerks-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "quotewerks": {
        "command": "quotewerks-mcp",
        "env": {
          "QUOTEWERKS_HOME": "/srv/quotewerks"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `QUOTEWERKS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `QUOTEWERKS_HOME`, or `doctor` will not find credentials left under the former root.

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
quotewerks-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "quotewerks-cli learnings confirm 12"] }
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
       materially more, record the divergence via `quotewerks-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `quotewerks-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `quotewerks-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
quotewerks-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
quotewerks-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
quotewerks-cli teach-playbook \
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
quotewerks-cli playbook amend \
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

`quotewerks-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `QUOTEWERKS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
quotewerks-cli feedback "the --since flag is inclusive but docs say exclusive"
quotewerks-cli feedback --stdin < notes.txt
quotewerks-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `QUOTEWERKS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `QUOTEWERKS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
quotewerks-cli profile save briefing --json
quotewerks-cli --profile briefing cloud-settings list
quotewerks-cli profile list --json
quotewerks-cli profile show briefing
quotewerks-cli profile delete briefing --yes
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
| 1 | `docs lint --strict` found error-severity findings |
| 6 | Partial failure: `requote --confirm` or `items reprice --confirm` wrote some rows and failed others; the error names what landed, so do not re-run `requote` |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `quotewerks-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add quotewerks-mcp --env QUOTEWERKS_API_KEY=<your-key> -- quotewerks-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which quotewerks-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   quotewerks-cli docs search --customer "Acme" --type QUOTE --agent
   ```
   For write previews use `--json` instead, because `--agent` compacts output.
4. If ambiguous, drill into subcommand help, for example `quotewerks-cli items reprice --help`.
