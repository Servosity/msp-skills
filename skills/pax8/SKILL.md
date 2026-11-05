---
name: pax8
description: "Every Pax8 Partner API endpoint, plus an offline store that reconciles billing, tracks MRR, and catches usage overages no Pax8 tool surfaces. Trigger phrases: `reconcile pax8 billing`, `what is my pax8 mrr`, `list pax8 companies`, `check pax8 usage overages`, `pax8 customer 360`, `use pax8`, `run pax8-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Pax8"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - pax8-cli
    install:
      - kind: script
        bins: [pax8-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.ps1
---

# Pax8 - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `pax8-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.ps1 | iex
   ```
3. Verify: `pax8-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `pax8-cli` and `pax8-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

pax8-cli mirrors the full Pax8 Partner API as typed, agent-native commands, then extends it with a local SQLite store. Reconcile invoices against subscriptions, compute MRR and margin, catch metered overages before they bill, and pull a true customer-360  -  the reports the portal makes you build by hand.

## When to Use This CLI

Use pax8-cli when an MSP needs to do anything with the Pax8 Partner API from the terminal or an agent: list/create/update companies and contacts, pull products and pricing, manage subscriptions and orders, fetch invoices and usage, and especially the cross-entity analytics (reconcile, mrr, overage, spend, customer-360) that the Pax8 portal cannot produce in one place. The six analytics commands (reconcile, mrr, overage, since, company show, spend) read the local synced store  -  run sync first; sync needs PAX8_CLIENT_ID/PAX8_CLIENT_SECRET.

## Anti-triggers

Do not use this CLI for:
- PSA/RMM tasks (tickets, alerts, patching)  -  Pax8 only covers marketplace billing/provisioning
- Vendor-portal administration (Microsoft 365 admin, Azure resources)  -  Pax8 resells products, it cannot administer them
- Payments or invoice disputes with Pax8 itself  -  portal/support workflow, not a Partner API surface
- Quoting/CPQ workflows  -  the Partner API has no quote endpoints

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`reconcile`**  -  Flag invoice lines that no longer match an active subscription, and active subscriptions that never got billed; --draft pre-checks the next unposted invoice.

  _Reach for this when an MSP needs to catch billing leakage the Pax8 portal can't surface in one place._

  ```bash
  pax8-cli reconcile --agent
  ```
- **`mrr`**  -  Compute monthly recurring revenue and margin from subscriptions and product pricing, trended across syncs, with a per-product breakdown.

  _Reach for this when an agent needs the revenue number the portal makes you build in a spreadsheet._

  ```bash
  pax8-cli mrr --agent --select totals.mrr,totals.margin
  ```
- **`overage`**  -  Aggregate usage-lines per subscription and surface overages before they land on the customer invoice.

  _Reach for this to catch metered overages proactively instead of after the invoice posts._

  ```bash
  pax8-cli overage --agent
  ```
- **`since`**  -  Show subscription changes in a window: synced history events plus subscriptions created or updated in it.

  _Reach for this to answer 'what changed in my book of business' without diffing exports by hand._

  ```bash
  pax8-cli since 7d --agent
  ```
- **`company show`**  -  One view of a company with its subscriptions, contacts, invoices, and usage.

  _Reach for this when an agent needs a full picture of one customer in a single call._

  ```bash
  pax8-cli company show <companyId> --agent --select subscriptions.productName,subscriptions.quantity
  ```
- **`spend`**  -  Roll up invoice-items by company to show what each customer is costing across invoices.

  _Reach for this to rank customers by spend without exporting every invoice._

  ```bash
  pax8-cli spend --agent
  ```

## Command Reference

**companies**  -  Companies represent the customers your organization serves through the Pax8 platform. Fetch a list of your company records, or a specific company by its companyId

- `pax8-cli companies create-company`  -  Creates a new company.
- `pax8-cli companies find`  -  Returns a paginated list of all your companies filtered by optional parameters
- `pax8-cli companies get`  -  Returns a single company record matching the ```companyId``` you specify
- `pax8-cli companies update-company`  -  Updates an existing Company. ATTENTION - at least one parameter has to be modified.

**invoices**  -  Manage invoices

- `pax8-cli invoices find-partner`  -  Fetch a paginated list of invoices. Default page is 0 and default size is 10. The maximum page size is 200
- `pax8-cli invoices find-partner-draft-items`  -  Fetch a paginated list of draft invoice items before they are finalized into invoices.
- `pax8-cli invoices get-partner`  -  Fetch a paginated list of invoices. Default page is 0 and default size is 10. The maximum page size is 200

**orders**  -  Orders describe your purchases of Pax8 products. Endpoints let you create new product orders and query past purchases

- `pax8-cli orders create`  -  Create a new order. Currently NOT supported for scheduled orders(orders with a future date).
- `pax8-cli orders find`  -  Returns a paginated list of orders. Currently NOT supported for scheduled orders(orders with a future date).
- `pax8-cli orders find-by-id`  -  Returns the Order record specified by OrderId. Currently NOT supported for scheduled orders

**products**  -  The Products resource lets you fetch all the information required to complete an Order -- product descriptions, pricing, provisioning details, and the dependencies associated with each product

- `pax8-cli products find-all`  -  Returns a paginated list of Pax8 products filtered by optional query parameters
- `pax8-cli products find-by-id`  -  Returns only the product record for the productId you specify

**subscriptions**  -  Subscriptions describe terms of service, including price and the billing start/end dates for a specified product/company combination. Resources let you update, cancel, and fetch details of existing subscriptions

- `pax8-cli subscriptions delete`  -  Cancels the Subscription specified by subscriptionId
- `pax8-cli subscriptions find`  -  Fetch a paginated list of subscriptions. Default page is 0 and default size is 10. The maximum page size is 200
- `pax8-cli subscriptions find-by-id`  -  Returns the Subscription record specified by the subscriptionId
- `pax8-cli subscriptions update`  -  Updates a subscription. Currently NOT supported for subscriptions with a future date.

**usage-summaries**  -  Manage usage summaries

- `pax8-cli usage-summaries <usageSummaryId>`  -  Fetch a paginated list of usage summaries. Default page is 0 and default size is 10. The maximum page size is 200


**company**


- `pax8-cli company show`  -  One view of a company with its subscriptions, contacts, invoices, and usage.


**mrr**


- `pax8-cli mrr`  -  Compute monthly recurring revenue and margin from subscriptions and product pricing, trended across syncs, with a per-product breakdown.


**overage**


- `pax8-cli overage`  -  Aggregate usage-lines per subscription and surface overages before they land on the customer invoice.


**reconcile**


- `pax8-cli reconcile`  -  Flag invoice lines that no longer match an active subscription, and active subscriptions that never got billed; --draft pre-checks the next unposted invoice.


**since**


- `pax8-cli since`  -  Show subscription changes in a window: synced history events plus subscriptions created or updated in it.


**spend**


- `pax8-cli spend`  -  Roll up invoice-items by company to show what each customer is costing across invoices.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
pax8-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Billing reconciliation for the current period

```bash
pax8-cli reconcile --agent
```

Surfaces invoice lines with no active subscription and active subscriptions that were never billed.

### MRR and margin, narrowed for an agent

```bash
pax8-cli mrr --agent --select totals.mrr,totals.margin,byProduct.productName
```

Returns just the revenue numbers an agent needs from a wide aggregate, using dotted select paths.

### Customer 360 in one call

```bash
pax8-cli company show <companyId> --agent --select subscriptions.productName,subscriptions.quantity
```

Joins a company to its subscriptions and invoices locally and narrows the deeply nested result.

### What changed this week

```bash
pax8-cli since 7d --agent
```

Reads synced subscription history plus subscriptions created or updated in the window: new, cancelled, and quantity-changed.

### Find a product by vendor

```bash
pax8-cli products find-all --vendor-name Microsoft --json
```

Lists the products one vendor sells through Pax8, straight from the Partner API.

## Auth Setup

Pax8 uses OAuth2 client-credentials. Set PAX8_CLIENT_ID and PAX8_CLIENT_SECRET (create an API key in the Pax8 portal under Integrations). The CLI exchanges them at https://token-manager.pax8.com/oauth/token with the required audience (override with PAX8_AUDIENCE, default api://p8p.client) and caches the bearer token until it expires. Run `pax8-cli doctor` to verify the token exchange; `pax8-cli auth login` mints and stores a token.

## Credential safety

- The client ID and secret go only to Pax8's token manager (`https://token-manager.pax8.com`), which receives nothing but the token request. A `PAX8_TOKEN_URL` on any other host is refused before anything is sent. The bearer token and every API request go only to the scheme, host and port of `PAX8_BASE_URL` (default `https://api.pax8.com/v1`). A redirect to any other host is refused, so a credential never reaches it.
- Any output, file, outbound request or MCP result that would repeat the client ID, the client secret or a token (from the environment, the store, or a token field in a response), whole or in an encoded form, is withheld whole, and "[credkit: withheld from <where>: it carried a credential this program holds]" takes its place. A record that carries one is not written to the local store. Responses are never cached on disk.
- Only `credentials.toml` (mode 0600) holds a credential. Set `PAX8_NO_CONFIG_WRITE=1` to keep even the minted token off disk.
- `--dry-run` sends nothing and mints no token. Over MCP, a tool that creates, updates, orders or cancels returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

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
  pax8-cli companies get mock-value --agent --select address,billOnBehalfOfEnabled,contacts
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

- Use `--home <dir>` for one invocation, or set `PAX8_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `PAX8_CONFIG_DIR`, `PAX8_DATA_DIR`, `PAX8_STATE_DIR`, `PAX8_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `PAX8_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `pax8-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "pax8": {
        "command": "pax8-mcp",
        "env": {
          "PAX8_HOME": "/srv/pax8"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `PAX8_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `PAX8_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `PAX8_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `PAX8_DATA_SOURCE` sets the `--data-source` default.

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
pax8-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "pax8-cli learnings confirm 12"] }
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
       materially more, record the divergence via `pax8-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `pax8-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `pax8-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
pax8-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
pax8-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
pax8-cli teach-playbook \
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
pax8-cli playbook amend \
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

`pax8-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `PAX8_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
pax8-cli feedback "the --since flag is inclusive but docs say exclusive"
pax8-cli feedback --stdin < notes.txt
pax8-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `PAX8_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PAX8_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
pax8-cli profile save briefing --json
pax8-cli --profile briefing companies get mock-value
pax8-cli profile list --json
pax8-cli profile show briefing
pax8-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `pax8-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add pax8-mcp -- pax8-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `pax8-mcp --transport http` requires `PAX8_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default (the bare root answers 404), and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `PAX8_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `PAX8_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which pax8-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   pax8-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `pax8-cli <command> --help`.
